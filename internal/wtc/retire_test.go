package wtc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRetireCollectionRunsTeardownAndKeepsRemoteRefs(t *testing.T) {
	c := newWorkspaceFixture(t)
	result, err := c.NewCollection(NewOptions{Slug: "finished", Repos: []string{"widget"}})
	if err != nil {
		t.Fatal(err)
	}
	target := result.Collection
	widget := filepath.Join(target, "widget")
	if err := os.Remove(filepath.Join(widget, "init-ran")); err != nil {
		t.Fatal(err)
	}
	fixtureFile(t, filepath.Join(widget, ".harness", "teardown.sh"), "#!/bin/sh\nprintf '%s' \"$WTC_COLLECTION\" > \"$(dirname \"$(dirname \"$PWD\")\")/teardown-ran\"\n", 0755)
	// The untracked hook itself would make the worktree dirty. Commit it only
	// in the local fixture and publish that ref to the local bare remote.
	fixtureGit(t, "-C", widget, "add", ".harness/teardown.sh")
	fixtureGit(t, "-C", widget, "-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "teardown fixture")
	head := fixtureGit(t, "-C", widget, "rev-parse", "HEAD")
	owner := filepath.Join(c.Workspace, ".bare", "widget.git")
	fixtureGit(t, "--git-dir="+owner, "update-ref", "refs/remotes/origin/teardown-fixture", head)
	retired, err := c.RetireCollection(RetireOptions{Name: "finished"})
	if err != nil {
		t.Fatal(err)
	}
	if !retired.FolderRemoved || len(retired.Removed) != 2 {
		t.Fatalf("unexpected retirement: %+v", retired)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target remains: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(c.Workspace, "teardown-ran")); err != nil || string(data) != "finished" {
		t.Fatalf("teardown hook did not run with collection env: %q (%v)", data, err)
	}
	if got := fixtureGit(t, "--git-dir="+owner, "rev-parse", "refs/remotes/origin/teardown-fixture"); got != head {
		t.Fatalf("remote ref changed: %s", got)
	}
}

func TestRetireCollectionPreflightAndLeftovers(t *testing.T) {
	c := newWorkspaceFixture(t)
	result, err := c.NewCollection(NewOptions{Slug: "pending", Repos: []string{"widget"}})
	if err != nil {
		t.Fatal(err)
	}
	target := result.Collection
	if _, err := c.RetireCollection(RetireOptions{Name: "pending"}); err == nil || !strings.Contains(err.Error(), "uncommitted") {
		t.Fatalf("dirty tree was not blocked: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "widget")); err != nil {
		t.Fatalf("preflight removed worktree: %v", err)
	}
	if err := os.Remove(filepath.Join(target, "widget", "init-ran")); err != nil {
		t.Fatal(err)
	}
	fixtureFile(t, filepath.Join(target, "keep.txt"), "unknown local state\n", 0644)
	retired, err := c.RetireCollection(RetireOptions{Name: "pending"})
	if err != nil {
		t.Fatal(err)
	}
	if retired.FolderRemoved || len(retired.Leftovers) != 1 || retired.Leftovers[0] != "keep.txt" {
		t.Fatalf("unknown file did not remain visible: %+v", retired)
	}
}

func TestRetireCollectionRejectsSelfAndEscapes(t *testing.T) {
	c := newWorkspaceFixture(t)
	for _, name := range []string{"main", "../source-widget", "missing"} {
		if _, err := c.RetireCollection(RetireOptions{Name: name, Force: true}); err == nil {
			t.Fatalf("accepted retirement of %q", name)
		}
	}
	if _, err := os.Stat(c.Harness); err != nil {
		t.Fatal(err)
	}
}

func TestRetireCollectionBlocksUnpushedCommitAndPreHookCanVeto(t *testing.T) {
	c := newWorkspaceFixture(t)
	result, err := c.NewCollection(NewOptions{Slug: "unpushed"})
	if err != nil {
		t.Fatal(err)
	}
	harness := filepath.Join(result.Collection, "harness")
	fixtureFile(t, filepath.Join(harness, "work.txt"), "local commit\n", 0644)
	fixtureGit(t, "-C", harness, "add", "work.txt")
	fixtureGit(t, "-C", harness, "-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "local fixture")
	if _, err := c.RetireCollection(RetireOptions{Name: "unpushed"}); err == nil || !strings.Contains(err.Error(), "commits absent") {
		t.Fatalf("unpushed commit was not blocked: %v", err)
	}
	fixtureFile(t, filepath.Join(c.Harness, "hooks", "wtc", "retire.pre.sh"), "#!/bin/sh\nexit 12\n", 0755)
	if _, err := c.RetireCollection(RetireOptions{Name: "unpushed", Force: true}); err == nil || !strings.Contains(err.Error(), "retire.pre") {
		t.Fatalf("pre-hook did not veto forced retirement: %v", err)
	}
	if _, err := os.Stat(harness); err != nil {
		t.Fatalf("pre-hook veto removed target: %v", err)
	}
}
