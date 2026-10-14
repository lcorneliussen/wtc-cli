package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/lcorneliussen/wtc-cli/internal/openclaw"
	"github.com/spf13/cobra"
)

func currentHost() openclaw.HostKind { return openclaw.DetectHost(os.Getenv) }

// printPlan writes the plan to stdout as the command's whole output, and what
// to do with it to stderr, where it reaches the agent without getting in the
// way of whatever parses the JSON.
func printPlan(plan openclaw.Plan) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(plan); err != nil {
		return err
	}
	for _, in := range plan.Intents {
		who := map[openclaw.Actor]string{
			openclaw.ActorParent:      "in the collection's session",
			openclaw.ActorRepoSession: "in this repository's session",
			openclaw.ActorCurrent:     "in this session",
		}[in.Actor]
		if in.Tool != "" {
			fmt.Fprintf(os.Stderr, "wtc: nothing was done. Call the %s tool %s with this intent's args, unchanged.\n", in.Tool, who)
		} else {
			fmt.Fprintf(os.Stderr, "wtc: nothing was done. Carry out this %s intent %s with the skill for %s.\n", in.Action, who, in.Forge)
		}
	}
	for _, w := range plan.Warnings {
		fmt.Fprintln(os.Stderr, "wtc: warning:", w)
	}
	return nil
}

func addHostCommands(root *cobra.Command, asJSON *bool) {
	host := &cobra.Command{Use: "host", Short: "Print what wtc is running under: openclaw, herdr or terminal", Args: cobra.NoArgs}
	host.RunE = func(cmd *cobra.Command, args []string) error {
		h := string(currentHost())
		return emit(envelope{OK: true, Data: map[string]string{"host": h}, Summary: h}, *asJSON)
	}
	root.AddCommand(host)

	var noPullRequests bool
	checkouts := &cobra.Command{
		Use:   "checkouts [path ...]",
		Short: "Print the status of checkouts as JSON: branch, ahead, pending changes, pull request",
		Long: `Describes each checkout (default: the current directory) the way a gateway
plugin shows a collection whose repositories live in separate sessions:
branch, commits ahead of and behind the base, pending changes, whether the
branch was published, and its pull request. It reads and changes nothing.`,
	}
	checkouts.Flags().BoolVar(&noPullRequests, "no-pull-requests", false, "Do not ask the forge for pull requests")
	checkouts.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			args = []string{"."}
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(openclaw.CheckoutsStatus(args, !noPullRequests))
	}
	root.AddCommand(checkouts)

	var opt openclaw.PublishOptions
	var dest string
	publish := &cobra.Command{
		Use:   "publish",
		Short: "Under OpenClaw: check this session's worktree and print the intent that publishes it",
		Long: `Under OpenClaw a command in the agent's shell holds no git credential and
cannot reach the gateway. publish therefore pushes nothing. It checks that the
session's managed worktree is safe to publish and prints the intent for the
agent to carry out: the github_publish tool on GitHub, a push and a pull
request through a forge skill elsewhere.

It refuses when history that was already published has been rewritten, when a
merge or rebase is unfinished, when files that look like credentials would be
committed, and outside a gateway-managed worktree.`,
		Args: cobra.NoArgs,
	}
	publish.Flags().StringVar(&opt.Title, "title", "", "Pull request title")
	publish.Flags().StringVar(&opt.Body, "body", "", "Pull request description")
	publish.Flags().StringVar(&dest, "dest", "", "Branch the pull request goes into (default: origin's default branch)")
	publish.Flags().BoolVar(&opt.AllowSensitive, "allow-sensitive", false, "Publish uncommitted files even when their names look like credentials")
	publish.RunE = func(cmd *cobra.Command, args []string) error {
		if h := currentHost(); h != openclaw.HostOpenClaw {
			return fmt.Errorf("wtc publish hands a publication to an OpenClaw gateway, and this is a %s session. Push and open the pull request with the wtc-pr or wtc-draft-pr skill", h)
		}
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		checkout, err := openclaw.OpenCheckout(cwd, dest)
		if err != nil {
			return err
		}
		plan, err := checkout.PlanPublish(opt)
		if err != nil {
			return err
		}
		return printPlan(plan)
	}
	root.AddCommand(publish)
}

// addRepoIntents is add-repo under OpenClaw: one sessions_spawn intent per
// repository, for the collection's session to carry out. The gateway creates
// the worktrees, so that it owns them and can publish them.
func addRepoIntents(repos []string, worktree, base string) error {
	if worktree == "" {
		if cwd, err := os.Getwd(); err == nil {
			worktree = openclaw.CurrentWorktree(cwd)
		}
	}
	if worktree == "" {
		worktree = os.Getenv("WTC_COLLECTION")
	}
	if worktree == "" {
		return errors.New("which worktree name? Pass --worktree <issue-id>-<slug>: every repository of the collection gets a worktree of that name, on branch " + openclaw.BranchPrefix + "<name>")
	}
	var intents []openclaw.Intent
	for _, repo := range repos {
		in, err := openclaw.AddRepo(repo, repo, worktree, base, "Work on "+repo+" for "+worktree+". Wait for instructions from the collection's session.")
		if err != nil {
			return err
		}
		intents = append(intents, in)
	}
	plan := openclaw.NewPlan(intents...)
	plan.Warnings = append(plan.Warnings, "Each repository must be a project on the gateway under the name given here ("+strings.Join(repos, ", ")+").")
	return printPlan(plan)
}

// catchUpCheckout is catch-up under OpenClaw: merge the destination branch
// into this session's worktree. No rebase and no push.
func catchUpCheckout(dest string, asJSON bool) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	checkout, err := openclaw.OpenCheckout(cwd, dest)
	if err != nil {
		return err
	}
	result, err := checkout.CatchUp()
	if err != nil {
		return err
	}
	summary := fmt.Sprintf("%s %s", result.Branch, result.Note)
	if result.Merged {
		summary = fmt.Sprintf("merged origin/%s into %s; publish to send the merge (wtc publish)", result.Dest, result.Branch)
	}
	return emit(envelope{OK: true, Data: result, Summary: summary}, asJSON)
}
