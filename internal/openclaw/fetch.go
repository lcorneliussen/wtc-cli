package openclaw

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// FetchBaseInterval is how long a clone's base branch counts as fresh. Status
// is asked for every half minute while a collection is on screen; fetching
// that often would only load the forge.
const FetchBaseInterval = 5 * time.Minute

// fetchCredentials makes git use the gh CLI's GitHub token, which the
// gateway's shell has and git does not read by itself. Nothing is written to
// the repository's config.
func fetchCredentials(origin Origin) []string {
	if origin.Forge != ForgeGitHub {
		return nil
	}
	if _, err := exec.LookPath("gh"); err != nil {
		return nil
	}
	return []string{"-c", "credential.helper=", "-c", "credential.helper=!gh auth git-credential"}
}

// FetchBases fetches the base branch of each checkout's clone, so that "behind"
// tells the truth without anyone having caught up first. Worktrees of one
// clone share it and are fetched once; a clone fetched within maxAge is left
// alone. It only moves remote-tracking refs, never a worktree, and a failed
// fetch leaves the last known base in place.
func FetchBases(paths []string, maxAge time.Duration) {
	type clone struct {
		dir      string
		origin   Origin
		branches []string
	}
	clones := map[string]*clone{}
	var order []string
	for _, p := range paths {
		common, err := git(p, "rev-parse", "--path-format=absolute", "--git-common-dir")
		if err != nil {
			continue
		}
		c := clones[common]
		if c == nil {
			c = &clone{dir: p, origin: Origin{Forge: ForgeUnknown}}
			if url, err := git(p, "remote", "get-url", "origin"); err == nil {
				c.origin = ParseOrigin(url)
			}
			clones[common] = c
			order = append(order, common)
		}
		// Worktrees of one clone may have different bases.
		if b := baseBranch(p); b != "" && !slices.Contains(c.branches, b) {
			c.branches = append(c.branches, b)
		}
	}
	var wg sync.WaitGroup
	for _, common := range order {
		c := clones[common]
		marker := filepath.Join(common, "wtc-base-fetched")
		if info, err := os.Stat(marker); err == nil && time.Since(info.ModTime()) < maxAge || len(c.branches) == 0 {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			args := append(fetchCredentials(c.origin), "fetch", "--quiet", "--no-tags", "--no-recurse-submodules", "origin")
			for _, b := range c.branches {
				args = append(args, "+refs/heads/"+b+":refs/remotes/origin/"+b)
			}
			cmd, err := gitCommand(ctx, c.dir, args...)
			if err != nil {
				return
			}
			cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
			if cmd.Run() == nil {
				_ = os.WriteFile(marker, nil, 0o644)
			}
		}()
	}
	wg.Wait()
}

// The branch a collection chose for a repository, when it is not origin's
// default: recorded per managed branch in the clone's git config, so every
// worktree of the clone knows its own. wtc add-repo --base records it.
const baseKey = "wtcbase"

// RecordBase notes branch as the base of the managed worktree name in the
// clone at dir: what it is compared with, caught up from and published to.
// An empty branch forgets the recorded base, so the repository's default
// applies.
func RecordBase(dir, name, branch string) error {
	key := "branch." + Branch(name) + "." + baseKey
	if branch == "" {
		if RecordedBase(dir, name) == "" {
			return nil
		}
		_, err := git(dir, "config", "--unset", key)
		return err
	}
	_, err := git(dir, "config", key, strings.TrimPrefix(branch, "origin/"))
	return err
}

// RecordedBase is the base recorded for managed worktree name in the
// repository at dir, or "".
func RecordedBase(dir, name string) string {
	return baseOfBranch(dir, Branch(name))
}

// checkedOutBase is the base recorded for the branch checked out at dir, or "".
func checkedOutBase(dir string) string {
	branch, _ := git(dir, "branch", "--show-current")
	if branch == "" {
		return ""
	}
	return baseOfBranch(dir, branch)
}

func baseOfBranch(dir, branch string) string {
	base, _ := git(dir, "config", "--get", "branch."+branch+"."+baseKey)
	return base
}

// baseBranch is the branch status measures against: the recorded base,
// otherwise origin's HEAD, else the first of main, master and develop that
// origin has.
func baseBranch(dir string) string {
	if b := checkedOutBase(dir); b != "" {
		return b
	}
	if head, err := git(dir, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"); err == nil && head != "" {
		return strings.TrimPrefix(head, "origin/")
	}
	for _, b := range []string{"main", "master", "develop"} {
		if gitOK(dir, "rev-parse", "--verify", "--quiet", "refs/remotes/origin/"+b) {
			return b
		}
	}
	return ""
}
