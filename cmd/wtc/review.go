package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/lcorneliussen/wtc-cli/internal/wtc"
	"github.com/spf13/cobra"
)

func addReviewCommands(root *cobra.Command, asJSON *bool) {
	var collection string
	review := &cobra.Command{Use: "review", Short: "Manage independent local PR reviews"}
	review.PersistentFlags().StringVar(&collection, "collection", "", "Collection directory (default: current)")
	context := func() (*wtc.Context, error) {
		if collection != "" {
			return wtc.OpenCollection(collection)
		}
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		return wtc.Discover(cwd)
	}
	var trusted bool
	status := &cobra.Command{Use: "status <repo> [pr-number]", Short: "Read the latest review comment and compare it with the PR head", Args: cobra.RangeArgs(1, 2)}
	status.Flags().BoolVar(&trusted, "trusted-local", false, "Require a local receipt from the review posting tool")
	status.RunE = func(cmd *cobra.Command, args []string) error {
		c, err := context()
		if err != nil {
			return err
		}
		number := ""
		if len(args) == 2 {
			number = args[1]
		}
		state, err := c.GetReviewStatus(args[0], number, trusted)
		if err != nil {
			return err
		}
		if *asJSON {
			return emit(envelope{OK: true, Data: state, Summary: state.State}, true)
		}
		if state.State == "none" {
			fmt.Println("none")
		} else {
			fmt.Printf("%s %s %d %d\n", state.State, state.Verdict, *state.Blockers, *state.Round)
		}
		return nil
	}
	review.AddCommand(status)
	var repo, authorization string
	ready := &cobra.Command{Use: "ready <pr-number>", Short: "Promote a draft PR after a current trusted passing review", Args: cobra.ExactArgs(1)}
	ready.Flags().StringVar(&repo, "repo", "", "Repository worktree name (default: current worktree)")
	ready.Flags().StringVar(&authorization, "user-authorized", "", "Verbatim user instruction overriding the review gate")
	ready.RunE = func(cmd *cobra.Command, args []string) error {
		c, err := context()
		if err != nil {
			return err
		}
		name := repo
		if name == "" {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			top, err := exec.Command("git", "-C", cwd, "rev-parse", "--show-toplevel").Output()
			if err != nil {
				return fmt.Errorf("run from a repository worktree or pass --repo")
			}
			name = filepath.Base(strings.TrimSpace(string(top)))
		}
		if err := wtc.ValidatePRIdentity(name, args[0]); err != nil {
			return err
		}
		worktree, slug, forge, err := c.ReviewRepo(name)
		if err != nil {
			return err
		}
		state, statusErr := c.GetReviewStatus(name, args[0], true)
		if authorization == "" {
			if statusErr != nil {
				return fmt.Errorf("cannot verify review gate: %w", statusErr)
			}
			if state.State != "current" || state.Verdict != "pass" && state.Verdict != "pass-with-notes" {
				return fmt.Errorf("review gate closed for %s #%s: %s %s", name, args[0], state.State, state.Verdict)
			}
		}
		var action *exec.Cmd
		if forge == "github" {
			action = exec.Command("gh", "pr", "ready", args[0], "--repo", slug)
		} else {
			action = exec.Command("bb", "pr", "ready", args[0])
			action.Dir = worktree
		}
		if !*asJSON {
			action.Stdout = os.Stdout
		}
		action.Stderr = os.Stderr
		if err := action.Run(); err != nil {
			return err
		}
		if *asJSON {
			return emit(envelope{OK: true, Data: map[string]any{"repo": name, "pr": args[0], "override": authorization != ""}, Summary: "PR marked ready"}, true)
		}
		return nil
	}
	review.AddCommand(ready)
	root.AddCommand(review)
}
