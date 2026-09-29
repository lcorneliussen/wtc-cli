package wtc

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type RetireOptions struct {
	Name  string
	Force bool
}

type RetireResult struct {
	Collection      string   `json:"collection"`
	Removed         []string `json:"removed"`
	Leftovers       []string `json:"leftovers,omitempty"`
	Warnings        []string `json:"warnings,omitempty"`
	WorkspaceClosed bool     `json:"workspace_closed"`
	FolderRemoved   bool     `json:"folder_removed"`
}

type retireWorktree struct {
	name, path, owner string
}

// RetireCollection first inspects every worktree, then removes only the named
// collection. Remote branches and bare owners are never changed.
func (c *Context) RetireCollection(opt RetireOptions) (RetireResult, error) {
	result := RetireResult{Removed: []string{}}
	if !collectionNamePattern.MatchString(opt.Name) {
		return result, fmt.Errorf("invalid collection name %q", opt.Name)
	}
	if opt.Name == filepath.Base(c.Collection) {
		return result, fmt.Errorf("cannot retire the collection running this command")
	}
	target := filepath.Join(c.Workspace, opt.Name)
	if info, err := os.Lstat(target); err != nil {
		return result, err
	} else if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return result, fmt.Errorf("collection path must be a directory: %s", target)
	}
	if _, err := OpenCollection(target); err != nil {
		return result, err
	}
	result.Collection = target
	worktrees, problems, err := inspectRetireWorktrees(target)
	if err != nil {
		return result, err
	}
	if len(problems) > 0 && !opt.Force {
		return result, fmt.Errorf("retirement blocked: %s (use --force only after confirming the work is disposable)", strings.Join(problems, "; "))
	}
	if len(problems) > 0 {
		result.Warnings = append(result.Warnings, problems...)
	}
	if _, err := os.Stat(filepath.Join(target, "HANDOFF.md")); err == nil {
		result.Warnings = append(result.Warnings, "unconsumed HANDOFF.md is being removed")
	}
	if _, err := os.Stat(filepath.Join(target, ".harness-backups")); err == nil {
		result.Warnings = append(result.Warnings, ".harness-backups remains in the collection folder")
	}
	values := map[string]string{"target": target, "force": strconv.FormatBool(opt.Force)}
	if err := c.RunHook("retire.pre", values); err != nil {
		return result, err
	}
	for _, wt := range worktrees {
		RunRepoTeardown(wt.path)
		args := []string{"--git-dir=" + wt.owner, "worktree", "remove"}
		if opt.Force {
			args = append(args, "--force")
		}
		args = append(args, wt.path)
		if _, err := gitOutput(args...); err != nil {
			return result, fmt.Errorf("remove %s: %w", wt.name, err)
		}
		if _, err := gitOutput("--git-dir="+wt.owner, "worktree", "prune"); err != nil {
			return result, fmt.Errorf("prune %s: %w", wt.name, err)
		}
		result.Removed = append(result.Removed, wt.name)
	}
	closed, warning := c.closeRetiredWorkspace(opt.Name)
	result.WorkspaceClosed = closed
	if warning != "" {
		result.Warnings = append(result.Warnings, warning)
	}
	stopRetiredStatusWatchers(target)
	if err := removeRetiredGeneratedFiles(target); err != nil {
		return result, err
	}
	if err := os.Remove(target); err == nil {
		result.FolderRemoved = true
	} else {
		entries, readErr := os.ReadDir(target)
		if readErr != nil {
			return result, fmt.Errorf("remove collection folder: %w", err)
		}
		for _, entry := range entries {
			result.Leftovers = append(result.Leftovers, entry.Name())
		}
	}
	if err := c.RunHook("retire.post", values); err != nil {
		return result, err
	}
	return result, nil
}

func inspectRetireWorktrees(target string) ([]retireWorktree, []string, error) {
	entries, err := os.ReadDir(target)
	if err != nil {
		return nil, nil, err
	}
	worktrees := []retireWorktree{}
	problems := []string{}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		path := filepath.Join(target, entry.Name())
		if _, err := os.Lstat(filepath.Join(path, ".git")); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return nil, nil, err
		}
		top, err := gitOutput("-C", path, "rev-parse", "--show-toplevel")
		resolvedTop, topErr := filepath.EvalSymlinks(top)
		resolvedPath, pathErr := filepath.EvalSymlinks(path)
		if err != nil || topErr != nil || pathErr != nil || resolvedTop != resolvedPath {
			return nil, nil, fmt.Errorf("%s is not an independent Git worktree", path)
		}
		owner, err := gitOutput("-C", path, "rev-parse", "--path-format=absolute", "--git-common-dir")
		if err != nil {
			return nil, nil, err
		}
		if info, err := os.Stat(owner); err != nil || !info.IsDir() {
			return nil, nil, fmt.Errorf("cannot find Git owner for %s", path)
		}
		worktrees = append(worktrees, retireWorktree{name: entry.Name(), path: path, owner: owner})
		status, err := gitOutput("-C", path, "status", "--porcelain")
		if err != nil {
			return nil, nil, err
		}
		if status != "" {
			problems = append(problems, entry.Name()+" has uncommitted changes")
		}
		unpushed, err := gitOutput("-C", path, "log", "--oneline", "HEAD", "--not", "--remotes")
		if err != nil {
			return nil, nil, err
		}
		if unpushed != "" {
			problems = append(problems, entry.Name()+" has commits absent from remote refs")
		}
	}
	// Teardown product repositories before their harness, which may supply
	// collection-level configuration used by product hooks.
	sort.Slice(worktrees, func(i, j int) bool {
		if worktrees[i].name == "harness" {
			return false
		}
		if worktrees[j].name == "harness" {
			return true
		}
		return worktrees[i].name < worktrees[j].name
	})
	return worktrees, problems, nil
}

func (c *Context) closeRetiredWorkspace(name string) (bool, string) {
	herdr, err := exec.LookPath("herdr")
	if err != nil {
		return false, ""
	}
	session := c.Config.Herdr.Session
	if session == "" {
		session = strings.TrimSuffix(strings.TrimSuffix(filepath.Base(c.Workspace), "-harness"), "-wtc")
	}
	output, err := exec.Command(herdr, "--session", session, "workspace", "list").Output()
	if err != nil {
		return false, "" // No running session is normal.
	}
	var listing struct {
		Result struct {
			Workspaces []struct {
				Label string `json:"label"`
				ID    string `json:"workspace_id"`
			} `json:"workspaces"`
		} `json:"result"`
	}
	if err := json.Unmarshal(output, &listing); err != nil {
		return false, fmt.Sprintf("could not read herdr workspaces: %v", err)
	}
	for _, workspace := range listing.Result.Workspaces {
		if workspace.Label != name {
			continue
		}
		if output, err := exec.Command(herdr, "--session", session, "workspace", "close", workspace.ID).CombinedOutput(); err != nil {
			return false, fmt.Sprintf("could not close herdr workspace %s: %v: %s", workspace.ID, err, strings.TrimSpace(string(output)))
		}
		return true, ""
	}
	return false, ""
}

func stopRetiredStatusWatchers(target string) {
	output, err := exec.Command("ps", "ax", "-o", "pid=,args=").Output()
	if err != nil {
		return
	}
	tui := filepath.Join(target, "harness", "tools", "wtc-status-tui.sh")
	oneshoot := filepath.Join(target, "harness", "tools", "wtc-status.sh")
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		args := strings.Join(fields[1:], " ")
		if !strings.Contains(args, tui) && !(strings.Contains(args, oneshoot) && strings.Contains(args, "--tui")) {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err == nil && pid != os.Getpid() {
			if process, err := os.FindProcess(pid); err == nil {
				_ = process.Kill()
			}
		}
	}
}

func removeRetiredGeneratedFiles(target string) error {
	files := []string{
		"HANDOFF.md", ".env.collection", ".env.collection.local", "mise.toml", ".DS_Store",
		"AGENTS.md", "WTC-SCOPE.md", ".mcp.json", ".envrc", ".env.toolchain",
		".wtc-prs", ".last-wtc-status.yml", ".wtc-status.json", ".wtc-status.md",
	}
	for _, name := range files {
		if err := os.Remove(filepath.Join(target, name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	for _, name := range []string{".claude", ".agents", ".cursor", ".codex", ".grok"} {
		if err := os.RemoveAll(filepath.Join(target, name)); err != nil {
			return err
		}
	}
	wtcDir := filepath.Join(target, ".wtc")
	if info, err := os.Lstat(wtcDir); err == nil && info.IsDir() {
		if err := os.RemoveAll(filepath.Join(wtcDir, "skills")); err != nil {
			return err
		}
		_ = os.Remove(wtcDir) // Other .wtc files are left for inspection.
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
