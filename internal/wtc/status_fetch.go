package wtc

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type StatusFetchReport struct {
	Attempted int `json:"attempted"`
	Failed    int `json:"failed"`
}

func statusCommonDir(worktree string) (string, error) {
	common, err := catchUpGit(worktree, "rev-parse", "--git-common-dir")
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(common) {
		common = filepath.Join(worktree, common)
	}
	return filepath.Clean(common), nil
}

func (c *Context) StatusRefreshRefs(all bool) (StatusFetchReport, error) {
	var report StatusFetchReport
	inventory, targets, err := c.CatchUpInventory(CatchUpOptions{All: all, DryRun: true})
	if err != nil {
		return report, err
	}
	if inventory.ExitStatus != 0 {
		return report, fmt.Errorf("cannot read status worktree inventory")
	}
	seen := map[string]bool{}
	for _, target := range targets {
		common, err := statusCommonDir(target.path)
		if err != nil {
			report.Failed++
			continue
		}
		if seen[common] {
			continue
		}
		seen[common] = true
		info, err := os.Stat(filepath.Join(common, "FETCH_HEAD"))
		if err == nil && time.Since(info.ModTime()) < 5*time.Minute {
			continue
		}
		report.Attempted++
		if _, err := gitOutput("--git-dir="+common, "fetch", "--prune", "origin"); err != nil {
			report.Failed++
		}
	}
	return report, nil
}

// StatusLivePreview refreshes stale refs before gathering local and forge
// facts. A failed fetch is reported but does not erase the last known refs.
func (c *Context) StatusLivePreview(noFetch bool) (StatusSnapshot, StatusFetchReport, error) {
	var report StatusFetchReport
	if !noFetch {
		var err error
		report, err = c.StatusRefreshRefs(false)
		if err != nil {
			return StatusSnapshot{}, report, err
		}
	}
	snapshot, err := c.StatusForgePreview()
	return snapshot, report, err
}
