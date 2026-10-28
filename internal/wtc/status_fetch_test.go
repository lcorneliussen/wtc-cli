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
	oldVersion := CLIVersion
	t.Cleanup(func() { CLIVersion = oldVersion })
	CLIVersion = "1.2.3"
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
	if all.CLIVersion != "1.2.3" {
		t.Fatalf("workspace live snapshot lost producer version: %q", all.CLIVersion)
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
	cached, cacheErr := c.ReadStatusSnapshot()
	if one.CLIVersion != "1.2.3" || cacheErr != nil || cached.CLIVersion != "1.2.3" {
		t.Fatalf("scoped live snapshot/cache lost producer: %q / %q (%v)", one.CLIVersion, cached.CLIVersion, cacheErr)
	}
	if _, err := os.Stat(filepath.Join(c.Collection, ".wtc-status.json")); err != nil {
		t.Fatalf("scoped cache missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(c.Workspace, "other", ".wtc-status.json")); !os.IsNotExist(err) {
		t.Fatalf("other collection cache written: %v", err)
	}
}

func TestStatusAllForwardsProgressIntoEachCollection(t *testing.T) {
	c := newWorkspaceFixture(t)
	if _, err := c.NewCollection(NewOptions{Slug: "other"}); err != nil {
		t.Fatal(err)
	}
	var steps []string
	c.StatusProgress = func(step string) { steps = append(steps, step) }
	if _, _, err := c.StatusLiveSnapshot(true, true); err != nil {
		t.Fatal(err)
	}
	worktrees := 0
	for _, step := range steps {
		if strings.HasPrefix(step, "Reading worktrees") {
			worktrees++
		}
	}
	if worktrees != 2 {
		t.Fatalf("workspace sweep lost per-collection progress: %v", steps)
	}
}

func TestStatusAllDoesNotRunOtherCollectionBuildHook(t *testing.T) {
	c := newWorkspaceFixture(t)
	if _, err := c.NewCollection(NewOptions{Slug: "other"}); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(c.Workspace, "other")
	hook := filepath.Join(other, "harness", "hooks", "wtc", "status.build.sh")
	if err := os.MkdirAll(filepath.Dir(hook), 0755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(other, "marker")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\ntouch '"+marker+"'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.StatusLiveSnapshot(true, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("workspace sweep ran another collection hook: %v", err)
	}
}

func TestStatusAllReadsOtherServicesWithItsOwnBinary(t *testing.T) {
	c := newWorkspaceFixture(t)
	if _, err := c.NewCollection(NewOptions{Slug: "other"}); err != nil {
		t.Fatal(err)
	}
	other, err := OpenCollection(filepath.Join(c.Workspace, "other"))
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(other.Collection, "marker")
	foreign := filepath.Join(other.Collection, "foreign-dekit")
	if err := os.WriteFile(foreign, []byte("#!/bin/sh\ntouch '"+marker+"'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	m := &runtimeManifest{Schema: 1, Root: other.runtimeRoot(), Binary: foreign,
		Tasks: []runtimeTask{{RuntimeItem: RuntimeItem{Collection: "other", Dir: "harness", Repo: "harness", Path: "harness/web/main", Kind: "service"},
			Log: filepath.Join(other.runtimeRoot(), "logs", "harness", "web", "main.log")}},
		Resources: []runtimeResource{{Dir: "harness", Repo: "harness", State: "provision-hook-completed"}}}
	if err := os.MkdirAll(m.Root, 0755); err != nil {
		t.Fatal(err)
	}
	if err := runtimeSaveManifest(m); err != nil {
		t.Fatal(err)
	}
	// This collection has no dekit: the sweep reports that, with the
	// manifest's services and resources, and never runs the foreign binary.
	c.Config.Runtime.Binary = filepath.Join(t.TempDir(), "missing-dekit")
	all, _, err := c.StatusLiveSnapshot(true, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("workspace sweep ran another collection's dekit: %v", err)
	}
	kinds := map[string]bool{}
	for _, item := range all.Runtime {
		if item.Collection == "other" {
			kinds[item.Kind] = item.State == "unknown" || item.Kind == "resources"
		}
	}
	if !kinds["service"] || !kinds["resources"] {
		t.Fatalf("sweep lost the other collection's services or resources: %+v", all.Runtime)
	}
}
