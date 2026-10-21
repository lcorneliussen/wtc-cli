package wtc

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// ReviewModeFile marks a collection created to review someone else's PR. In
// that mode wtc makes no commits and pushes nothing; it only moves the
// worktree to the author's latest head, and leaves the author's steps
// (resolving threads, undrafting) to the author.
const ReviewModeFile = ".wtc-review-mode"

// reviewPushPrefix is prepended to every push URL of a review-only worktree.
// The rewritten URL names a path under /dev/null, so a push fails locally,
// before any network access, with the reason in git's own error.
const reviewPushPrefix = "file:///dev/null/push-disabled-in-wtc-review-mode/"

type ReviewMode struct {
	SchemaVersion int    `json:"schema_version"`
	Repo          string `json:"repo"`
	Dir           string `json:"dir"`
	PR            string `json:"pr"`
	URL           string `json:"url,omitempty"`
	CreatedAt     string `json:"created_at"`
	// Heads lists every PR head this collection checked out, oldest first.
	// Only these may be left behind when the author rewrites the branch.
	Heads []string `json:"heads"`
}

// ReadReviewMode reports the review mode of a collection. A marker that
// cannot be parsed still makes the collection review-only.
func ReadReviewMode(collection string) (ReviewMode, bool) {
	data, err := os.ReadFile(filepath.Join(collection, ReviewModeFile))
	if err != nil {
		return ReviewMode{}, false
	}
	var mode ReviewMode
	_ = json.Unmarshal(data, &mode)
	return mode, true
}

func writeReviewMode(collection string, mode ReviewMode) error {
	mode.SchemaVersion = 1
	if mode.CreatedAt == "" {
		mode.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	data, err := json.MarshalIndent(mode, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(collection, ReviewModeFile), append(data, '\n'), 0644)
}

// reviewOnlyWorktree reports whether path is the PR worktree of a
// review-only collection.
func reviewOnlyWorktree(path string) bool {
	mode, ok := ReadReviewMode(filepath.Dir(path))
	return ok && (mode.Dir == "" || mode.Dir == filepath.Base(path))
}

// RefuseInReviewMode stops an author-only step in a review-only collection.
func (c *Context) RefuseInReviewMode(step string) error {
	mode, ok := ReadReviewMode(c.Collection)
	if !ok {
		return nil
	}
	pr := "this PR"
	if mode.Repo != "" && mode.PR != "" {
		pr = mode.Repo + "#" + mode.PR
	}
	return fmt.Errorf("%s belongs to the PR author; this collection only reviews %s", step, pr)
}

// lockReviewPush disables pushing from one worktree without touching the
// others that share its repository, in three layers of the worktree's own
// config:
//   - each remote gets a push URL; an explicit pushurl is used as is, so no
//     url.*.pushInsteadOf rule, which git picks by longest prefix, redirects it;
//   - an empty-prefix pushInsteadOf rewrites remotes added later and URLs given
//     on the command line, unless a longer rule elsewhere wins;
//   - core.hooksPath points at a pre-push hook that refuses, for those cases
//     too, unless the push skips hooks with --no-verify.
//
// Per-worktree config needs extensions.worktreeConfig. Enabling it writes to
// the shared owner: format version 1, the extension, and, for a bare owner,
// core.bare moved into the owner's own config.worktree, or every linked
// worktree would read it as bare. Remotes are checked before those writes.
func lockReviewPush(collection, worktree string) error {
	remotes, err := gitOutput("-C", worktree, "remote")
	if err != nil {
		return err
	}
	for _, remote := range strings.Fields(remotes) {
		// pushurl is multi-valued: one added here would be pushed to
		// alongside one set elsewhere.
		if urls, _ := gitOutput("-C", worktree, "config", "--get-all", "remote."+remote+".pushurl"); urls != "" {
			return fmt.Errorf("remote %s has remote.%s.pushurl set (%s), which a review-only worktree cannot override; remove it to review here", remote, remote, strings.ReplaceAll(urls, "\n", ", "))
		}
	}
	common, err := gitOutput("-C", worktree, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return err
	}
	shared := filepath.Join(common, "config")
	enabled, _ := gitOutput("--git-dir="+common, "config", "--file", shared, "--bool", "extensions.worktreeConfig")
	if enabled != "true" {
		bare, _ := gitOutput("--git-dir="+common, "config", "--file", shared, "--bool", "core.bare")
		if bare == "true" {
			if _, err := gitOutput("--git-dir="+common, "config", "--file", filepath.Join(common, "config.worktree"), "core.bare", "true"); err != nil {
				return err
			}
			if _, err := gitOutput("--git-dir="+common, "config", "--file", shared, "--unset", "core.bare"); err != nil {
				return err
			}
		}
		// Git reads repository extensions under format version 1, as
		// `git sparse-checkout` sets it when it enables this extension.
		if version, _ := gitOutput("--git-dir="+common, "config", "--file", shared, "core.repositoryformatversion"); version == "" || version == "0" {
			if _, err := gitOutput("--git-dir="+common, "config", "--file", shared, "core.repositoryformatversion", "1"); err != nil {
				return err
			}
		}
		if _, err := gitOutput("--git-dir="+common, "config", "--file", shared, "extensions.worktreeConfig", "true"); err != nil {
			return err
		}
	}
	for _, remote := range strings.Fields(remotes) {
		if _, err := gitOutput("-C", worktree, "config", "--worktree", "remote."+remote+".pushurl", reviewPushPrefix+remote); err != nil {
			return err
		}
	}
	if _, err := gitOutput("-C", worktree, "config", "--worktree", "url."+reviewPushPrefix+".pushInsteadOf", ""); err != nil {
		return err
	}
	hooks := filepath.Join(collection, ".wtc", "review-hooks")
	if err := os.MkdirAll(hooks, 0755); err != nil {
		return err
	}
	hook := "#!/bin/sh\necho 'push-disabled-in-wtc-review-mode: this collection only reviews the PR and pushes nothing' >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(hooks, "pre-push"), []byte(hook), 0755); err != nil {
		return err
	}
	if _, err := gitOutput("-C", worktree, "config", "--worktree", "core.hooksPath", hooks); err != nil {
		return err
	}
	return verifyReviewPushLock(worktree)
}

// verifyReviewPushLock checks the effective push URL of every remote.
func verifyReviewPushLock(worktree string) error {
	remotes, err := gitOutput("-C", worktree, "remote")
	if err != nil {
		return err
	}
	for _, remote := range strings.Fields(remotes) {
		urls, err := gitOutput("-C", worktree, "remote", "get-url", "--push", "--all", remote)
		if err != nil {
			return err
		}
		for _, url := range strings.Split(urls, "\n") {
			if !strings.HasPrefix(url, reviewPushPrefix) {
				return fmt.Errorf("remote %s still pushes to %s after the review-only lock", remote, url)
			}
		}
	}
	return nil
}

// followReviewHead moves a review-only worktree to the PR's current head, so
// each bundle reviews what the author last pushed. It never creates a merge
// commit: a new head is reached by fast-forward, or by reset when the author
// rewrote the branch. Commits that exist only here stop it, since review mode
// writes none. The base branch is fetched too, since catch-up is skipped.
func followReviewHead(collection, worktree, forge, pr, branch, base, expected string) error {
	ref := "refs/remotes/wtc-pr/" + pr
	source := "+refs/pull/" + pr + "/head:" + ref
	if forge == "bitbucket" {
		if branch == "" {
			return fmt.Errorf("PR source branch unknown")
		}
		source = "+refs/heads/" + branch + ":" + ref
	}
	fetch := []string{"-C", worktree, "fetch", "origin", source}
	if base != "" {
		fetch = append(fetch, "+refs/heads/"+base+":refs/remotes/origin/"+base)
	}
	if _, err := gitOutput(fetch...); err != nil {
		return err
	}
	head, err := gitOutput("-C", worktree, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return err
	}
	if expected != "" && !SameReviewSHA(head, expected) {
		return fmt.Errorf("fetched %s, but the PR head is %s", head[:7], expected[:min(7, len(expected))])
	}
	current, err := gitOutput("-C", worktree, "rev-parse", "HEAD")
	if err != nil || current == head {
		return err
	}
	mode, _ := ReadReviewMode(collection)
	if status, err := gitOutput("-C", worktree, "status", "--porcelain", "--untracked-files=no"); err != nil {
		return err
	} else if status != "" {
		return fmt.Errorf("worktree has local changes; review mode makes none, so discard them first")
	}
	if _, err := gitOutput("-C", worktree, "merge-base", "--is-ancestor", "HEAD", head); err == nil {
		_, err = gitOutput("-C", worktree, "merge", "--ff-only", "--quiet", head)
	} else if !slices.Contains(mode.Heads, current) {
		// The author rewrote the branch, and HEAD is not a head reviewed
		// here, so it holds commits made in this collection.
		return fmt.Errorf("HEAD %s is not a PR head this collection reviewed; review mode makes no commits", current[:7])
	} else {
		_, err = gitOutput("-C", worktree, "reset", "--keep", "--quiet", head)
	}
	if err != nil {
		return err
	}
	mode.Heads = append(mode.Heads, head)
	return writeReviewMode(collection, mode)
}
