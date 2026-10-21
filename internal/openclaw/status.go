package openclaw

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// CollectionSchema names the JSON that describes a collection's checkouts. A
// gateway plugin renders it; the fields are what a person looks for before
// publishing or catching up.
const CollectionSchema = "wtc.collection/v1"

// Collection is the status of a set of checkouts that belong to one task.
type Collection struct {
	Schema string           `json:"schema"`
	Repos  []CheckoutStatus `json:"repos"`
}

// CheckoutStatus is one checkout, seen through plain git.
type CheckoutStatus struct {
	Path string `json:"path"`
	Repo string `json:"repo,omitempty"`
	// Owner is the repository's owner on its forge: a GitHub user or
	// organization, a Bitbucket workspace.
	Owner  string `json:"owner,omitempty"`
	Forge  Forge  `json:"forge,omitempty"`
	Branch string `json:"branch,omitempty"`
	// Base is the ref Ahead and Behind are counted against.
	Base       string `json:"base,omitempty"`
	Ahead      int    `json:"ahead"`
	Behind     int    `json:"behind"`
	Dirty      int    `json:"dirty"`
	Untracked  int    `json:"untracked"`
	Conflicted int    `json:"conflicted"`
	// Published says that the gateway has published this branch before.
	Published bool `json:"published"`
	// Unpublished counts the commits since then; with Dirty it is what the
	// next publication would send.
	Unpublished int          `json:"unpublished"`
	PullRequest *PullRequest `json:"pullRequest"`
	Error       string       `json:"error,omitempty"`
}

// PullRequest is the pull request of a checkout's branch.
type PullRequest struct {
	Number int    `json:"number"`
	URL    string `json:"url"`
	State  string `json:"state"`
	Draft  bool   `json:"draft"`
	Title  string `json:"title,omitempty"`
	// Checks is the CI of its head: success, failure, pending or none.
	Checks  string `json:"checks"`
	Passed  int    `json:"passed"`
	Failed  int    `json:"failed"`
	Running int    `json:"running"`
	// Mergeable is whether it merges into its base: mergeable, conflicting,
	// or unknown while GitHub is still working it out.
	Mergeable string `json:"mergeable,omitempty"`
	// Review is the review decision: approved, changes_requested,
	// review_required, or empty when the repository requires none.
	Review string `json:"review,omitempty"`
}

// ManagedCollections finds the gateway's collections: the managed worktrees
// under stateDir, grouped by worktree name, which is the collection's name.
// Each repository's worktree is in a directory of its own project.
func ManagedCollections(stateDir string) (map[string][]string, error) {
	dirs, err := filepath.Glob(filepath.Join(stateDir, "worktrees", "*", "*"))
	if err != nil {
		return nil, err
	}
	collections := map[string][]string{}
	for _, dir := range dirs {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
			continue
		}
		name := filepath.Base(dir)
		if ValidWorktreeName(name) {
			collections[name] = append(collections[name], dir)
		}
	}
	return collections, nil
}

// GatewayProject is the name of the gateway's own clone of a repository,
// which is its project ID, or "" when the gateway has none. projects.add
// clones GitHub repositories to stateDir/projects/<hash>/<name>; the wtc
// plugin clones others, such as Bitbucket ones, to stateDir/repos/<name> and
// registers them under that name. Checkouts registered from anywhere else are
// not found.
func GatewayProject(stateDir string, origin Origin) string {
	if dir := GatewayProjectDir(stateDir, origin); dir != "" {
		return filepath.Base(dir)
	}
	return ""
}

// GatewayProjectDirByName is the directory of the gateway's clone of the
// project named name, by the same rules as GatewayProject, or "".
func GatewayProjectDirByName(stateDir, name string) string {
	if name == "" || strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		return ""
	}
	dirs, _ := filepath.Glob(filepath.Join(stateDir, "projects", "*", name))
	dirs = append(dirs, filepath.Join(stateDir, "repos", name))
	for _, dir := range dirs {
		if gitOK(dir, "rev-parse", "--git-dir") {
			return dir
		}
	}
	return ""
}

// GatewayProjectDir is the directory of the gateway's clone that
// GatewayProject names, or "".
func GatewayProjectDir(stateDir string, origin Origin) string {
	dirs, _ := filepath.Glob(filepath.Join(stateDir, "projects", "*", origin.Repo))
	dirs = append(dirs, filepath.Join(stateDir, "repos", origin.Repo))
	for _, dir := range dirs {
		url, err := git(dir, "remote", "get-url", "origin")
		if err != nil {
			continue
		}
		o := ParseOrigin(url)
		if o.Forge == origin.Forge && strings.EqualFold(o.Owner, origin.Owner) && strings.EqualFold(o.Repo, origin.Repo) {
			return dir
		}
	}
	return ""
}

// CheckoutsStatus describes the checkouts at paths. With pullRequests it also asks
// GitHub, through gh and whatever login gh has, for each GitHub checkout's
// pull request; a checkout it cannot ask about just has none.
func CheckoutsStatus(paths []string, pullRequests bool) Collection {
	c := Collection{Schema: CollectionSchema, Repos: []CheckoutStatus{}}
	for _, p := range paths {
		c.Repos = append(c.Repos, checkoutStatus(p, pullRequests))
	}
	return c
}

func checkoutStatus(path string, pullRequests bool) CheckoutStatus {
	s := CheckoutStatus{Path: path}
	top, err := git(path, "rev-parse", "--show-toplevel")
	if err != nil {
		s.Error = "not a git checkout"
		if untrusted := (*UntrustedConfigError)(nil); errors.As(err, &untrusted) {
			s.Error = err.Error()
		}
		return s
	}
	s.Branch, _ = git(top, "branch", "--show-current")
	origin := Origin{Forge: ForgeUnknown}
	if url, err := git(top, "remote", "get-url", "origin"); err == nil {
		origin = ParseOrigin(url)
	}
	s.Forge, s.Owner, s.Repo = origin.Forge, origin.Owner, origin.Repo

	changes, err := (&Checkout{Dir: top}).pending()
	if err != nil {
		s.Error = err.Error()
		return s
	}
	for _, ch := range changes {
		s.Dirty++
		switch {
		case ch.status == "??":
			s.Untracked++
		case strings.Contains(ch.status, "U") || ch.status == "AA" || ch.status == "DD":
			s.Conflicted++
		}
	}

	head, _ := git(top, "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
	if b := checkedOutBase(top); b != "" {
		head = "origin/" + b
	}
	for _, candidate := range []string{head, "origin/main", "origin/master", "origin/develop"} {
		if candidate != "" && gitOK(top, "rev-parse", "--verify", "--quiet", candidate+"^{commit}") {
			s.Base = candidate
			break
		}
	}
	if s.Base != "" {
		if counts, err := git(top, "rev-list", "--left-right", "--count", s.Base+"...HEAD"); err == nil {
			if f := strings.Fields(counts); len(f) == 2 {
				s.Behind, _ = strconv.Atoi(f[0])
				s.Ahead, _ = strconv.Atoi(f[1])
			}
		}
	}
	s.Unpublished = s.Ahead
	if name, managed := strings.CutPrefix(s.Branch, BranchPrefix); managed {
		if published := (&Checkout{Dir: top, Worktree: name}).publishedHead(); published != "" {
			s.Published = true
			// Commits that came from the base, such as the pull request's
			// merge after a fast-forward catch-up, are not new work.
			args := []string{"rev-list", "--count", "HEAD", "^" + published}
			if s.Base != "" {
				args = append(args, "^"+s.Base)
			}
			if n, err := git(top, args...); err == nil {
				s.Unpublished, _ = strconv.Atoi(n)
			}
		}
	}
	if pullRequests && origin.Forge == ForgeGitHub && s.Branch != "" {
		s.PullRequest = gitHubPullRequest(top, origin, s.Branch)
	}
	return s
}

// gitHubPullRequest is the newest pull request from branch, or nil: without
// gh, without a login, or when there is none.
func gitHubPullRequest(dir string, origin Origin, branch string) *PullRequest {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", "pr", "list", "--repo", origin.Owner+"/"+origin.Repo, "--head", branch,
		"--state", "all", "--limit", "1", "--json", "number,url,state,isDraft,title,statusCheckRollup,mergeable,reviewDecision")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var found []struct {
		Number  int    `json:"number"`
		URL     string `json:"url"`
		State   string `json:"state"`
		IsDraft bool   `json:"isDraft"`
		Title   string `json:"title"`
		Merge   string `json:"mergeable"`
		Review  string `json:"reviewDecision"`
		Rollup  []struct {
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
			State      string `json:"state"`
		} `json:"statusCheckRollup"`
	}
	if json.Unmarshal(out, &found) != nil || len(found) == 0 {
		return nil
	}
	f := found[0]
	pr := &PullRequest{Number: f.Number, URL: f.URL, State: strings.ToLower(f.State), Draft: f.IsDraft, Title: f.Title, Checks: "none",
		Mergeable: strings.ToLower(f.Merge), Review: strings.ToLower(f.Review)}
	// Check runs report status and conclusion, commit statuses a state.
	for _, c := range f.Rollup {
		switch result := strings.ToUpper(c.Conclusion + c.State); {
		case c.Status != "" && c.Status != "COMPLETED", result == "PENDING", result == "EXPECTED":
			pr.Running++
		case result == "SUCCESS", result == "NEUTRAL", result == "SKIPPED":
			pr.Passed++
		default:
			pr.Failed++
		}
	}
	switch {
	case pr.Failed > 0:
		pr.Checks = "failure"
	case pr.Running > 0:
		pr.Checks = "pending"
	case pr.Passed > 0:
		pr.Checks = "success"
	}
	return pr
}
