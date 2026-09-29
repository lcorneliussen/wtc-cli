package wtc

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func gitOutput(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func (c *Context) Repository(name string) (Repo, error) {
	for _, repo := range c.Registry.Repos {
		if repo.Name == name {
			return repo, nil
		}
	}
	return Repo{}, fmt.Errorf("repository %q is not in the registry", name)
}

// HarnessRepoName resolves the registry identity of harness/, which need not
// be named harness in a downstream registry.
func (c *Context) HarnessRepoName() (string, error) {
	if name := os.Getenv("WTC_HARNESS_REPO"); name != "" {
		if _, err := c.Repository(name); err != nil {
			return "", err
		}
		return name, nil
	}
	owner, err := gitOutput("-C", c.Harness, "rev-parse", "--git-common-dir")
	if err == nil {
		name := strings.TrimSuffix(filepath.Base(owner), ".git")
		if _, err := c.Repository(name); err == nil {
			return name, nil
		}
	}
	remote, err := gitOutput("-C", c.Harness, "remote", "get-url", "origin")
	if err == nil {
		for _, repo := range c.Registry.Repos {
			if repo.Remote == remote {
				return repo.Name, nil
			}
		}
	}
	return "", fmt.Errorf("cannot infer harness repository; set WTC_HARNESS_REPO")
}

func (c *Context) bareFor(name string) (string, error) {
	if _, err := c.Repository(name); err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(c.Harness, ".harness-repos"))
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, name+"=") {
			path := strings.TrimPrefix(line, name+"=")
			if path != "" {
				return path, nil
			}
		}
	}
	return filepath.Join(c.Workspace, ".bare", name+".git"), nil
}

func (c *Context) ensureBare(repo Repo) (string, error) {
	bare, err := c.bareFor(repo.Name)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(bare); os.IsNotExist(err) {
		if repo.Remote == "" {
			return "", fmt.Errorf("repository %q has no remote", repo.Name)
		}
		if _, err := gitOutput("clone", "--bare", repo.Remote, bare); err != nil {
			return "", err
		}
		if _, err := gitOutput("--git-dir="+bare, "config", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*"); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}
	if _, err := gitOutput("--git-dir="+bare, "fetch", "--prune", "origin"); err != nil {
		return "", err
	}
	return bare, nil
}

// AddWorktree checks out the current development tip without claiming a branch.
// An explicit branch is used for a PR head or the uncommon -b override.
func (c *Context) AddWorktree(name, directory, destination, branch string) error {
	repo, err := c.Repository(name)
	if err != nil {
		return err
	}
	if !validRepoName.MatchString(directory) {
		return fmt.Errorf("invalid worktree directory %q", directory)
	}
	bare, err := c.ensureBare(repo)
	if err != nil {
		return err
	}
	ref := repo.DefaultRef
	if ref == "" {
		ref = "origin/main"
	}
	if _, err := gitOutput("--git-dir="+bare, "rev-parse", "--verify", ref); err != nil {
		return fmt.Errorf("%s development tip %s is unavailable: %w", name, ref, err)
	}
	path := filepath.Join(destination, directory)
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("worktree path already exists: %s", path)
	} else if !os.IsNotExist(err) {
		return err
	}
	args := []string{"--git-dir=" + bare, "worktree", "add"}
	if branch == "" {
		args = append(args, "--detach", path, ref)
	} else {
		if _, err := gitOutput("check-ref-format", "--branch", branch); err != nil {
			return fmt.Errorf("invalid branch %q: %w", branch, err)
		}
		if _, err := gitOutput("--git-dir="+bare, "rev-parse", "--verify", "refs/heads/"+branch); err == nil {
			args = append(args, path, branch)
		} else if _, err := gitOutput("--git-dir="+bare, "rev-parse", "--verify", "refs/remotes/origin/"+branch); err == nil {
			args = append(args, "-b", branch, path, "origin/"+branch)
		} else {
			args = append(args, "-b", branch, path, ref)
		}
	}
	if _, err := gitOutput(args...); err != nil {
		return err
	}
	trustWorktreeMise(path)
	return nil
}

// AddPRWorktree fetches GitHub's immutable view of the PR head and verifies
// it against the forge response. A missing head must never turn into a new
// branch at the development tip.
func (c *Context) AddPRWorktree(name, directory, destination, branch, number, expectedSHA string) error {
	if !prNumber.MatchString(number) || !validRepoName.MatchString(directory) || branch == "" {
		return fmt.Errorf("invalid PR worktree identity")
	}
	decoded, err := hex.DecodeString(expectedSHA)
	if err != nil || (len(decoded) != 20 && len(decoded) != 32) {
		return fmt.Errorf("invalid PR head commit")
	}
	if _, err := gitOutput("check-ref-format", "--branch", branch); err != nil {
		return fmt.Errorf("invalid PR head branch %q: %w", branch, err)
	}
	repo, err := c.Repository(name)
	if err != nil {
		return err
	}
	bare, err := c.ensureBare(repo)
	if err != nil {
		return err
	}
	ref := "refs/remotes/wtc-pr/" + number
	if _, err := gitOutput("--git-dir="+bare, "fetch", "origin", "+refs/pull/"+number+"/head:"+ref); err != nil {
		return fmt.Errorf("fetch PR head #%s: %w", number, err)
	}
	head, err := gitOutput("--git-dir="+bare, "rev-parse", "--verify", ref)
	if err != nil || !strings.EqualFold(head, expectedSHA) {
		return fmt.Errorf("PR head changed while creating collection; expected %s, fetched %s", expectedSHA, head)
	}
	path := filepath.Join(destination, directory)
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("worktree path already exists: %s", path)
	} else if !os.IsNotExist(err) {
		return err
	}
	args := []string{"--git-dir=" + bare, "worktree", "add"}
	if localHead, err := gitOutput("--git-dir="+bare, "rev-parse", "--verify", "refs/heads/"+branch); err == nil {
		if !strings.EqualFold(localHead, expectedSHA) {
			return fmt.Errorf("local branch %s differs from PR head %s", branch, expectedSHA)
		}
		args = append(args, path, branch)
	} else {
		start := ref
		if originHead, err := gitOutput("--git-dir="+bare, "rev-parse", "--verify", "refs/remotes/origin/"+branch); err == nil && strings.EqualFold(originHead, expectedSHA) {
			start = "origin/" + branch
		}
		args = append(args, "-b", branch, path, start)
	}
	if _, err := gitOutput(args...); err != nil {
		return err
	}
	trustWorktreeMise(path)
	return nil
}

func trustWorktreeMise(path string) {
	if _, err := os.Stat(filepath.Join(path, "mise.toml")); err == nil {
		if mise, err := exec.LookPath("mise"); err == nil {
			cmd := exec.Command(mise, "trust", filepath.Join(path, "mise.toml"))
			cmd.Dir = path
			_ = cmd.Run() // Matches the shell bootstrap: a missing trust is reported by later use.
		}
	}
}

// RunRepoInit preserves the repository lifecycle hook contract. Failure is
// reported but does not discard an otherwise valid new worktree.
func RunRepoInit(worktree string) {
	if mise, err := exec.LookPath("mise"); err == nil {
		if _, err := os.Stat(filepath.Join(worktree, "mise.toml")); err == nil {
			cmd := exec.Command(mise, "tasks", "ls")
			cmd.Dir = worktree
			if output, err := cmd.Output(); err == nil {
				scanner := bufio.NewScanner(strings.NewReader(string(output)))
				for scanner.Scan() {
					if fields := strings.Fields(scanner.Text()); len(fields) > 0 && fields[0] == "harness:init" {
						runRepoCommand(worktree, mise, "run", "harness:init")
						return
					}
				}
			}
		}
	}
	path := filepath.Join(worktree, ".harness", "init.sh")
	if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && info.Mode()&0111 != 0 {
		runRepoCommand(worktree, path)
	}
}

func runRepoCommand(worktree, program string, args ...string) {
	cmd := exec.Command(program, args...)
	cmd.Dir = worktree
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "wtc: warning: init hook for %s failed: %v; setup may be incomplete\n", filepath.Base(worktree), err)
	}
}
