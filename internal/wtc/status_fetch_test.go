package wtc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStatusRefreshRefsUsesSharedOwnerAgeGate(t *testing.T) {
	c := newWorkspaceFixture(t)
	common, err := statusCommonDir(c.Harness)
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(filepath.Join(c.Workspace, ".bare", "agent-harness.git"))
	if err != nil {
		t.Fatal(err)
	}
	if common != want {
		t.Fatalf("wrong shared owner: %s", common)
	}
	source := filepath.Join(c.Workspace, "source-agent-harness")
	fixtureFile(t, filepath.Join(source, "new"), "tip moved\n", 0644)
	fixtureGit(t, "-C", source, "add", "new")
	fixtureGit(t, "-C", source, "-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "new tip")
	old := time.Now().Add(-10 * time.Minute)
	if err := os.Chtimes(filepath.Join(common, "FETCH_HEAD"), old, old); err != nil {
		t.Fatal(err)
	}
	before, _, err := c.StatusLivePreview(true)
	if err != nil || before.StaleCount != 0 {
		t.Fatalf("--no-fetch changed refs: stale=%d %v", before.StaleCount, err)
	}
	report, err := c.StatusRefreshRefs(false)
	if err != nil || report.Attempted != 1 || report.Failed != 0 {
		t.Fatalf("wrong refresh: %+v %v", report, err)
	}
	after, _, err := c.StatusLivePreview(true)
	if err != nil || after.StaleCount != 1 {
		t.Fatalf("ref refresh did not expose stale worktree: stale=%d %v", after.StaleCount, err)
	}
	report, err = c.StatusRefreshRefs(false)
	if err != nil || report.Attempted != 0 {
		t.Fatalf("fresh shared owner fetched again: %+v %v", report, err)
	}
	report, err = c.StatusRefreshRefsWithAge(false, 0)
	if err != nil || report.Attempted != 1 || report.Failed != 0 {
		t.Fatalf("zero fetch age did not force a refresh: %+v %v", report, err)
	}
}

func TestStatusLiveSnapshotWritesOnlyScopedCaches(t *testing.T) {
	c := newWorkspaceFixture(t)
	if _, err := c.NewCollection(NewOptions{Slug: "other"}); err != nil {
		t.Fatal(err)
	}
	all, _, err := c.StatusLiveSnapshot(true, true)
	if err != nil {
		t.Fatal(err)
	}
	if !all.ShowCollectionColumn || all.Collection != "" || len(all.Repos) != 2 || len(all.PRs) != 0 {
		t.Fatalf("wrong workspace status scope: %+v", all)
	}
	allMD := all.Markdown()
	if !strings.Contains(allMD, "**main/harness**") || !strings.Contains(allMD, "**other/harness**") || strings.Contains(allMD, "## PRs") {
		t.Fatalf("workspace Markdown lost collection scope: %s", allMD)
	}
	for _, name := range []string{"main", "other"} {
		if _, err := os.Stat(filepath.Join(c.Workspace, name, ".wtc-status.json")); !os.IsNotExist(err) {
			t.Fatalf("workspace sweep wrote %s cache: %v", name, err)
		}
	}
	one, _, err := c.StatusLiveSnapshot(false, true)
	if err != nil || one.Collection != "main" {
		t.Fatalf("scoped status failed: %+v %v", one, err)
	}
	if _, err := os.Stat(filepath.Join(c.Collection, ".wtc-status.json")); err != nil {
		t.Fatalf("scoped cache missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(c.Workspace, "other", ".wtc-status.json")); !os.IsNotExist(err) {
		t.Fatalf("other collection cache written: %v", err)
	}
}
