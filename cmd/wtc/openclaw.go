package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/lcorneliussen/wtc-cli/internal/openclaw"
	"github.com/lcorneliussen/wtc-cli/internal/wtc"
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
		if in.Action == openclaw.ActionPublish {
			how := " with the skill for " + string(in.Forge)
			if in.Tool != "" {
				how = " with the " + in.Tool + " tool and its args, unchanged"
			}
			fmt.Fprintf(os.Stderr, "wtc: nothing was done. Publishing is the person's: tell them it is ready to push with ↑ in the collection card. Only if they asked you to publish, carry out this intent %s%s.\n", who, how)
		} else if in.Tool != "" {
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
// the worktrees, so that it owns them and can publish them. A collection with
// a harness resolves names through the harness's registry, and gets a
// worktree of the harness itself first.
func addRepoIntents(repos []string, worktree, base, harnessFlag string, separate bool) error {
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
	if !openclaw.ValidWorktreeName(worktree) {
		return fmt.Errorf("%q is not a valid OpenClaw worktree name (lowercase letters, digits and hyphens, at most 64)", worktree)
	}
	// A base is a branch of origin; the gateway takes it as a ref.
	branch := strings.TrimPrefix(base, "origin/")
	adds, err := harnessRepos(repos, worktree, branch, harnessFlag)
	if err != nil {
		return err
	}
	var intents []openclaw.Intent
	var projects []string
	for _, add := range adds {
		arg, branch := add.arg, add.branch
		base := ""
		if branch != "" {
			base = "origin/" + branch
		}
		repo, project, gitURL, err := openclaw.ProjectSource(arg)
		if err != nil {
			return err
		}
		task := "You own the " + repo + " worktree of collection " + worktree + ". The collection's session works in it; do nothing until it asks you to. When asked to publish, run wtc publish in your worktree and carry out its plan."
		if gitURL != "" {
			origin := openclaw.ParseOrigin(gitURL)
			if dir := openclaw.GatewayProjectDir(gatewayStateDir(), origin); dir != "" {
				project, gitURL = filepath.Base(dir), ""
				if branch != "" {
					// What the worktree is compared with, caught up from and
					// published to, not only where it starts.
					if err := openclaw.RecordBase(dir, worktree, branch); err != nil {
						return fmt.Errorf("recording %s as the base of %s: %w", branch, repo, err)
					}
				}
			} else if origin.Forge != openclaw.ForgeGitHub {
				// The gateway clones GitHub repositories only.
				return fmt.Errorf("%s is not on the gateway yet. Ask the person to add %s with Add repository in the Collection panel, then run this again", repo, arg)
			} else if separate {
				task = "Work on " + repo + " for " + worktree + ". Wait for instructions from the collection's session."
			} else {
				return fmt.Errorf("%s is not on the gateway yet. Ask the person to add %s with Add repository in the Collection panel, then run this again. Only if they want %s as a conversation of its own: --separate", repo, arg, repo)
			}
		}
		if gitURL == "" && branch != "" {
			if dir := openclaw.GatewayProjectDirByName(gatewayStateDir(), project); dir != "" {
				if err := openclaw.RecordBase(dir, worktree, branch); err != nil {
					return fmt.Errorf("recording %s as the base of %s: %w", branch, repo, err)
				}
			}
		}
		in, err := openclaw.AddRepo(repo, project, gitURL, worktree, base, task)
		if err != nil {
			return err
		}
		intents = append(intents, in)
		if project != "" && !add.resolved {
			projects = append(projects, project)
		}
	}
	plan := openclaw.NewPlan(intents...)
	if len(projects) > 0 {
		plan.Warnings = append(plan.Warnings, "Each of "+strings.Join(projects, ", ")+" must be a project on the gateway under that name. For a GitHub repository that is not, pass owner/name instead: the gateway clones it.")
	}
	return printPlan(plan)
}

// repoToAdd is one repository add-repo spawns a worktree of: arg as
// ProjectSource reads it, and the branch it is based on, or "" for the
// gateway's default. resolved marks a remote from a harness.
type repoToAdd struct {
	arg, branch string
	resolved    bool
}

// harnessRepos chooses the collection's harness and resolves the
// repositories through it: a name in its registry becomes that entry's
// remote, based on its default_ref unless --base says otherwise. When the
// collection has no worktree of the harness yet, it comes first.
func harnessRepos(repos []string, worktree, branch, flag string) ([]repoToAdd, error) {
	plain := func() []repoToAdd {
		var adds []repoToAdd
		for _, r := range repos {
			adds = append(adds, repoToAdd{arg: r, branch: branch})
		}
		return adds
	}
	cfg, err := gatewayConfig()
	if err != nil {
		return nil, err
	}
	folder := filepath.Join(gatewayStateDir(), "wtc-collections", worktree)
	recorded, _, err := wtc.GatewayHarness(folder)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, r := range repos {
		if _, _, gitURL, err := openclaw.ProjectSource(r); err == nil && gitURL == "" {
			names = append(names, r)
		}
	}
	registries := map[string]harnessRegistry{}
	registryOf := func(h openclaw.Harness) harnessRegistry {
		if r, ok := registries[h.Name]; ok {
			return r
		}
		r := readHarnessRegistry(h, worktree)
		registries[h.Name] = r
		return r
	}
	h, err := openclaw.ChooseHarness(cfg, flag, recorded, names, func(h openclaw.Harness, name string) bool {
		_, ok := registryOf(h).find(name)
		return ok
	})
	if err != nil || h == nil {
		return plain(), err
	}
	reg := registryOf(*h)
	if reg.err != nil {
		return nil, reg.err
	}
	if err := wtc.RecordGatewayHarness(folder, h.Name, h.Remote); err != nil {
		return nil, err
	}
	var adds []repoToAdd
	if reg.worktree == "" {
		adds = append(adds, repoToAdd{arg: h.Remote, branch: h.Ref, resolved: true})
	}
	for _, r := range repos {
		add := repoToAdd{arg: r, branch: branch}
		entry, ok := reg.find(r)
		if !ok {
			if _, _, gitURL, err := openclaw.ProjectSource(r); err == nil && gitURL != "" {
				entry, ok = reg.findRemote(gitURL)
			}
		}
		if ok {
			add.arg, add.resolved = entry.Remote, true
			if add.branch == "" {
				add.branch = strings.TrimPrefix(entry.DefaultRef, "origin/")
			}
		}
		if openclaw.SameRepository(add.arg, h.Remote) {
			// The harness is added once, above, or is there already.
			continue
		}
		adds = append(adds, add)
	}
	return adds, nil
}

// harnessRegistry is a harness's registry as a collection sees it: from its
// worktree of the harness when it has one, else from the gateway's clone of
// the harness at its configured ref.
type harnessRegistry struct {
	repos    []wtc.Repo
	worktree string
	err      error
}

func (r harnessRegistry) find(name string) (wtc.Repo, bool) {
	for _, repo := range r.repos {
		if repo.Name == name {
			return repo, true
		}
	}
	return wtc.Repo{}, false
}

func (r harnessRegistry) findRemote(url string) (wtc.Repo, bool) {
	for _, repo := range r.repos {
		if openclaw.SameRepository(repo.Remote, url) {
			return repo, true
		}
	}
	return wtc.Repo{}, false
}

func readHarnessRegistry(h openclaw.Harness, worktree string) harnessRegistry {
	var r harnessRegistry
	var data []byte
	all, _ := openclaw.ManagedCollections(gatewayStateDir())
	for _, dir := range all[worktree] {
		if openclaw.CheckoutOf(dir, h.Remote) {
			r.worktree = dir
			data, r.err = os.ReadFile(filepath.Join(dir, ".harness-repos.yml"))
			break
		}
	}
	if r.worktree == "" {
		dir := openclaw.GatewayProjectDir(gatewayStateDir(), openclaw.ParseOrigin(h.Remote))
		if dir == "" {
			r.err = fmt.Errorf("harness %s is not on the gateway yet. Ask the person to add %s with Add repository in the Collection panel, then run this again", h.Name, h.Remote)
			return r
		}
		if data, r.err = openclaw.FileAt(dir, "origin/"+h.Ref, ".harness-repos.yml"); r.err != nil {
			data, r.err = openclaw.FileAt(dir, h.Ref, ".harness-repos.yml")
		}
	}
	if r.err != nil {
		r.err = fmt.Errorf("harness %s has no .harness-repos.yml to read at %s, so it is not a harness wtc can use; check %s.harnesses: %w", h.Name, h.Ref, openclaw.PluginConfigKey, r.err)
		return r
	}
	reg, err := wtc.ParseRegistry(data)
	if err != nil {
		r.err = fmt.Errorf("harness %s: %w", h.Name, err)
		return r
	}
	r.repos = reg.Repos
	return r
}

// runOpenClaw runs the openclaw CLI; tests replace it.
var runOpenClaw = func(args ...string) (string, error) {
	if _, err := exec.LookPath("openclaw"); err != nil {
		return "", errNoOpenClaw
	}
	out, err := exec.Command("openclaw", args...).CombinedOutput()
	return string(out), err
}

var errNoOpenClaw = errors.New("openclaw not found")

// gatewayConfig is the wtc plugin's config on this gateway. Without the
// openclaw CLI, or with nothing set, there are no harnesses.
func gatewayConfig() (openclaw.GatewayConfig, error) {
	out, err := runOpenClaw("config", "get", openclaw.PluginConfigKey, "--json")
	if errors.Is(err, errNoOpenClaw) || (err != nil && strings.Contains(out, "unset")) {
		return openclaw.GatewayConfig{}, nil
	}
	if err != nil {
		return openclaw.GatewayConfig{}, fmt.Errorf("reading %s: %v: %s", openclaw.PluginConfigKey, err, strings.TrimSpace(out))
	}
	return openclaw.ParseGatewayConfig(out)
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

// gatewayStatus is status under OpenClaw. A collection there is no folder:
// it is the managed worktrees of one name, one per repository, each owned by
// a session of its own. Without a name it shows every collection.
func gatewayStatus(names []string, asJSON bool) error {
	all, err := openclaw.ManagedCollections(gatewayStateDir())
	if err != nil {
		return err
	}
	if len(names) == 0 {
		for name := range all {
			names = append(names, name)
		}
		sort.Strings(names)
	}
	result := map[string]gatewayCollection{}
	for _, name := range names {
		openclaw.FetchBases(all[name], openclaw.FetchBaseInterval)
		col := gatewayCollection{Collection: openclaw.CheckoutsStatus(all[name], true), Services: []wtc.RuntimeItem{}}
		col.Folder, col.Services, col.Error = linkGatewayFolder(name, col.Repos)
		col.Harness = gatewayHarness(col.Folder, all[name])
		result[name] = col
	}
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}
	if len(names) == 0 {
		fmt.Println("No collections: the gateway has no managed worktrees yet. wtc add-repo <repo> --worktree <name> starts one.")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for i, name := range names {
		if i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintf(w, "%s\n", name)
		repos := result[name].Repos
		if folder := result[name].Folder; folder != "" {
			fmt.Fprintf(w, "  folder %s\n", folder)
		}
		if len(repos) == 0 {
			fmt.Fprintln(w, "  (no repositories)")
			continue
		}
		fmt.Fprintln(w, "  REPO\tBRANCH\tAGAINST BASE\tTO PUBLISH\tPR")
		for _, r := range repos {
			if r.Error != "" {
				fmt.Fprintf(w, "  %s\t%s\t\t%s\t\n", filepath.Base(filepath.Dir(r.Path)), r.Branch, r.Error)
				continue
			}
			base := fmt.Sprintf("%d ahead, %d behind", r.Ahead, r.Behind)
			pending := "nothing"
			if n := r.Dirty + r.Untracked; n > 0 || r.Unpublished > 0 {
				pending = fmt.Sprintf("%d files, %d commits", n, r.Unpublished)
			}
			pr := "—"
			if p := r.PullRequest; p != nil {
				pr = fmt.Sprintf("#%d %s", p.Number, p.State)
				if p.Draft && p.State == "open" {
					pr = fmt.Sprintf("#%d draft", p.Number)
				}
			}
			fmt.Fprintf(w, "  %s\t%s\t%s\t%s\t%s\n", r.Repo, r.Branch, base, pending, pr)
		}
		for _, svc := range result[name].Services {
			fmt.Fprintf(w, "  service %s\t%s\t%s\t%s\t\n", svc.Path, svc.Kind, svc.State, svc.URL)
		}
		if e := result[name].Error; e != "" {
			fmt.Fprintf(w, "  folder: %s\n", e)
		}
	}
	return w.Flush()
}

// gatewayCollection is one collection as wtc status shows it under OpenClaw.
type gatewayCollection struct {
	openclaw.Collection
	// Folder holds the collection as one folder: a link per repository to its
	// managed worktree. Services run there.
	Folder string `json:"folder,omitempty"`
	// Harness is the collection's harness, whose AGENTS.md and
	// instructions apply to the work in it.
	Harness  *gatewayHarnessStatus `json:"harness,omitempty"`
	Services []wtc.RuntimeItem     `json:"services"`
	Error    string                `json:"error,omitempty"`
}

type gatewayHarnessStatus struct {
	Name   string `json:"name"`
	Remote string `json:"remote"`
	// Path is the collection's worktree of the harness, once it has one.
	Path string `json:"path,omitempty"`
}

// gatewayHarness is the harness recorded for the collection folder, with its
// worktree among the collection's, or nil.
func gatewayHarness(folder string, worktrees []string) *gatewayHarnessStatus {
	name, remote, err := wtc.GatewayHarness(folder)
	if err != nil || name == "" {
		return nil
	}
	h := &gatewayHarnessStatus{Name: name, Remote: remote}
	for _, dir := range worktrees {
		if openclaw.CheckoutOf(dir, remote) {
			h.Path = dir
			break
		}
	}
	return h
}

// linkGatewayFolder keeps the collection's folder in step with its worktrees
// and reads the services running for it.
func linkGatewayFolder(name string, repos []openclaw.CheckoutStatus) (string, []wtc.RuntimeItem, string) {
	var linked []wtc.LinkedRepo
	for _, r := range repos {
		if r.Error != "" || r.Repo == "" {
			continue
		}
		remote, err := exec.Command("git", "-C", r.Path, "remote", "get-url", "origin").Output()
		if err != nil {
			continue
		}
		base := r.Base
		if base == "" {
			base = "origin/main"
		}
		linked = append(linked, wtc.LinkedRepo{Name: r.Repo, Remote: strings.TrimSpace(string(remote)), DefaultRef: base, Path: r.Path})
	}
	folder := filepath.Join(gatewayStateDir(), "wtc-collections", name)
	c, err := wtc.LinkGatewayCollection(folder, linked)
	if err != nil {
		return folder, []wtc.RuntimeItem{}, err.Error()
	}
	services := c.RuntimeStatus()
	if services == nil {
		services = []wtc.RuntimeItem{}
	}
	return folder, services, ""
}

// refuseUnderGateway stops commands that make folder collections: under
// OpenClaw the gateway owns the worktrees, and a folder collection beside it
// is one it can neither see nor publish.
func refuseUnderGateway(command string) error {
	if currentHost() != openclaw.HostOpenClaw {
		return nil
	}
	return fmt.Errorf("wtc %s makes a folder collection, which the OpenClaw gateway cannot see or publish. Here a collection is managed worktrees of one name: wtc add-repo <owner/name> --worktree <name> starts one, and wtc status lists them (wtc docs instructions/openclaw.md)", command)
}

// gatewayStateDir is where the gateway keeps its projects and worktrees.
func gatewayStateDir() string {
	if dir := os.Getenv("OPENCLAW_STATE_DIR"); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".openclaw")
}
