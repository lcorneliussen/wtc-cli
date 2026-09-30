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
	return c.StatusRefreshRefsWithAge(all, 5*time.Minute)
}

func (c *Context) StatusRefreshRefsWithAge(all bool, maxAge time.Duration) (StatusFetchReport, error) {
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
		if err == nil && time.Since(info.ModTime()) < maxAge {
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
	return c.StatusLivePreviewWithFetchAge(noFetch, 5*time.Minute)
}

func (c *Context) StatusLivePreviewWithFetchAge(noFetch bool, maxAge time.Duration) (StatusSnapshot, StatusFetchReport, error) {
	var report StatusFetchReport
	if !noFetch {
		var err error
		report, err = c.StatusRefreshRefsWithAge(false, maxAge)
		if err != nil {
			return StatusSnapshot{}, report, err
		}
	}
	snapshot, err := c.StatusForgePreview()
	return snapshot, report, err
}

// StatusLiveSnapshot runs the complete public one-shot collector. A scoped
// result updates the three collection caches; an explicit workspace sweep
// stays read-only across collection boundaries and omits enlisted PR sections.
func (c *Context) StatusLiveSnapshot(all, noFetch bool) (StatusSnapshot, StatusFetchReport, error) {
	return c.StatusLiveSnapshotWithFetchAge(all, noFetch, 5*time.Minute)
}

func (c *Context) StatusLiveSnapshotWithFetchAge(all, noFetch bool, maxAge time.Duration) (StatusSnapshot, StatusFetchReport, error) {
	var report StatusFetchReport
	if !noFetch {
		var err error
		report, err = c.StatusRefreshRefsWithAge(all, maxAge)
		if err != nil {
			return StatusSnapshot{}, report, err
		}
	}
	if !all {
		snapshot, err := c.StatusForgePreview()
		if err != nil {
			return snapshot, report, err
		}
		return snapshot, report, c.WriteStatusSnapshot(snapshot)
	}
	collections, err := WorkspaceCollections(c.Workspace)
	if err != nil {
		return StatusSnapshot{}, report, err
	}
	snapshot := StatusSnapshot{Schema: 1, GeneratedAt: time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		ShowCollectionColumn: true, Repos: []StatusRepo{}, PRs: []StatusPRRow{}, Orphans: []StatusOrphan{}}
	for _, dir := range collections {
		collection, err := OpenCollection(dir)
		if err != nil {
			return snapshot, report, err
		}
		part, err := collection.StatusForgePreview()
		if err != nil {
			return snapshot, report, err
		}
		snapshot.Repos = append(snapshot.Repos, part.Repos...)
		snapshot.StaleCount += part.StaleCount
	}
	return snapshot, report, nil
}
