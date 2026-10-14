package openclaw

import (
	"context"
	"encoding/json"
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
	Path   string `json:"path"`
	Repo   string `json:"repo,omitempty"`
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
}

// CheckoutsStatus describes the checkouts at paths. With pullRequests it also asks
// GitHub, through gh and whatever login gh has, for each GitHub checkout's
// pull request; a checkout it cannot ask about just has none.
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
		return s
	}
	s.Branch, _ = git(top, "branch", "--show-current")
	origin := Origin{Forge: ForgeUnknown}
	if url, err := git(top, "remote", "get-url", "origin"); err == nil {
		origin = ParseOrigin(url)
	}
	s.Forge, s.Repo = origin.Forge, origin.Repo

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
			if n, err := git(top, "rev-list", "--count", published+"..HEAD"); err == nil {
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
		"--state", "all", "--limit", "1", "--json", "number,url,state,isDraft")
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
	}
	if json.Unmarshal(out, &found) != nil || len(found) == 0 {
		return nil
	}
	return &PullRequest{Number: found[0].Number, URL: found[0].URL, State: strings.ToLower(found[0].State), Draft: found[0].IsDraft}
}
