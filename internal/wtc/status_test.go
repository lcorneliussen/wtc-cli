package wtc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStatusLocalSnapshotTracksDirtyBranchAndDevelopmentTip(t *testing.T) {
	c := newWorkspaceFixture(t)
	snapshot, err := c.StatusLocalSnapshot(false)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Collection != "main" || snapshot.ShowCollectionColumn || len(snapshot.Repos) == 0 || snapshot.StaleCount != 0 {
		t.Fatalf("unexpected initial snapshot: %+v", snapshot)
	}
	var harness StatusRepo
	for _, row := range snapshot.Repos {
		if row.Dir == "harness" {
			harness = row
		}
	}
	if harness.BranchKind != "detached" || harness.BranchDisplay != "⌂ main" || harness.Ahead != 0 || harness.Behind != 0 || harness.Tree != "clean" {
		t.Fatalf("wrong detached facts: %+v", harness)
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"pr":null`) || !strings.Contains(string(data), `"prs":[]`) {
		t.Fatalf("local snapshot does not preserve canonical shape: %s", data)
	}

	fixtureGit(t, "-C", c.Harness, "switch", "-qc", "topic")
	fixtureFile(t, filepath.Join(c.Harness, "committed"), "local work\n", 0644)
	fixtureGit(t, "-C", c.Harness, "add", "committed")
	fixtureGit(t, "-C", c.Harness, "-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "local")
	fixtureFile(t, filepath.Join(c.Harness, "untracked"), "dirty\n", 0644)
	source := filepath.Join(c.Workspace, "source-agent-harness")
	fixtureFile(t, filepath.Join(source, "README.md"), "new development tip\n", 0644)
	fixtureGit(t, "-C", source, "add", "README.md")
	fixtureGit(t, "-C", source, "-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "upstream")
	owner := filepath.Join(c.Workspace, ".bare", "agent-harness.git")
	fixtureGit(t, "--git-dir="+owner, "fetch", "-q", "origin", "+refs/heads/*:refs/remotes/origin/*")
	snapshot, err = c.StatusLocalSnapshot(false)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range snapshot.Repos {
		if row.Dir == "harness" {
			harness = row
		}
	}
	if harness.BranchKind != "branch" || harness.Branch != "topic" || harness.Ahead != 1 || harness.Behind != 1 || harness.Changed != 1 || harness.Tree != "±1" || snapshot.StaleCount != 1 {
		t.Fatalf("wrong branch facts: %+v; stale=%d", harness, snapshot.StaleCount)
	}
	if _, err := os.Stat(filepath.Join(c.Collection, ".wtc-status.json")); !os.IsNotExist(err) {
		t.Fatalf("local facts stage wrote status cache: %v", err)
	}
}

func TestStatusLocalSnapshotShowsUnresolvedMerge(t *testing.T) {
	c := newWorkspaceFixture(t)
	fixtureGit(t, "-C", c.Harness, "switch", "-qc", "topic")
	fixtureFile(t, filepath.Join(c.Harness, "README.md"), "local\n", 0644)
	fixtureGit(t, "-C", c.Harness, "add", "README.md")
	fixtureGit(t, "-C", c.Harness, "-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "local")
	source := filepath.Join(c.Workspace, "source-agent-harness")
	fixtureFile(t, filepath.Join(source, "README.md"), "upstream\n", 0644)
	fixtureGit(t, "-C", source, "add", "README.md")
	fixtureGit(t, "-C", source, "-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "upstream")
	owner := filepath.Join(c.Workspace, ".bare", "agent-harness.git")
	fixtureGit(t, "--git-dir="+owner, "fetch", "-q", "origin", "+refs/heads/*:refs/remotes/origin/*")
	fixtureGit(t, "-C", c.Harness, "config", "user.name", "fixture")
	fixtureGit(t, "-C", c.Harness, "config", "user.email", "fixture@example.invalid")
	if _, err := catchUpGit(c.Harness, "merge", "--no-edit", "origin/main"); err == nil {
		t.Fatal("fixture merge unexpectedly succeeded")
	}
	snapshot, err := c.StatusLocalSnapshot(false)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range snapshot.Repos {
		if row.Dir == "harness" {
			if !row.Conflict || row.Operation != "MERGE_HEAD" || !strings.Contains(snapshot.Markdown(), "unresolved conflicts") {
				t.Fatalf("unresolved merge hidden: %+v", row)
			}
			return
		}
	}
	t.Fatal("missing harness row")
}

func TestStatusLocalSnapshotAllRequiresExplicitSweep(t *testing.T) {
	c := newWorkspaceFixture(t)
	if _, err := c.NewCollection(NewOptions{Slug: "other"}); err != nil {
		t.Fatal(err)
	}
	one, err := c.StatusLocalSnapshot(false)
	if err != nil {
		t.Fatal(err)
	}
	all, err := c.StatusLocalSnapshot(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Repos) <= len(one.Repos) || !all.ShowCollectionColumn || all.Collection != "" {
		t.Fatalf("explicit sweep did not widen status: one=%+v all=%+v", one, all)
	}
}

func TestStatusLocalSnapshotRecordsSupportedForge(t *testing.T) {
	c := newWorkspaceFixture(t)
	for _, tc := range []struct{ remote, forge string }{
		{"git@github.com:example/widget.git", "github.com"},
		{"git@bitbucket.org:example/widget.git", "bitbucket.org"},
	} {
		fixtureGit(t, "-C", c.Harness, "remote", "set-url", "origin", tc.remote)
		snapshot, err := c.StatusLocalSnapshot(false)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range snapshot.Repos {
			if row.Dir == "harness" && (row.Slug != "example/widget" || row.Forge != tc.forge) {
				t.Fatalf("forge identity for %q: %+v", tc.remote, row)
			}
		}
	}
}

func TestStatusMarkdownKeepsRepoAndPRSignals(t *testing.T) {
	checks, review, merge := "SUCCESS", "noreviewers", "CLEAN"
	url := "https://github.com/example/widget/pull/7"
	snapshot := StatusSnapshot{Schema: 1, Collection: "fixture", GeneratedAt: "2026-09-30T00:00:00Z",
		StaleCount: 1,
		Repos: []StatusRepo{{Dir: "widget", BranchDisplay: "topic", Tree: "±2", Ahead: 1, Behind: 1,
			PR: &StatusPRFacts{Number: "7", Checks: checks, Merge: merge, Review: review}}},
		PRs: []StatusPRRow{{Repo: "widget", Number: "7", Checks: &checks, Merge: &merge, Review: &review,
			Title: "Add widget", URL: &url}},
		Orphans: []StatusOrphan{{Repo: "harness", Branch: "old", State: "MERGED"}}}
	md := snapshot.Markdown()
	for _, want := range []string{
		"# fixture", "**widget** (`topic`) — ±2; ↑1; ↓1; PR #7 ✓ ∅ no reviewers",
		"_1 worktree(s) behind remote — catch-up needed._",
		"[Add widget](https://github.com/example/widget/pull/7)",
		"## Orphans", "**harness** on `old` — PR MERGED; catch-up returns it to the tip",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("Markdown missing %q:\n%s", want, md)
		}
	}
	reposOnly := snapshot.ReposMarkdown()
	if strings.Contains(reposOnly, "## PRs") || strings.Contains(reposOnly, "## Orphans") || !strings.Contains(reposOnly, "**widget**") {
		t.Fatalf("repository-only Markdown retained PR section: %s", reposOnly)
	}
}

func TestStatusMarkdownShowsOpenPRsBeforeDrafts(t *testing.T) {
	snapshot := StatusSnapshot{Collection: "fixture", PRs: []StatusPRRow{
		{Repo: "widget", Number: "1", State: "DRAFT", Draft: true, Title: "Draft first"},
		{Repo: "widget", Number: "2", State: "OPEN", Title: "Open first"},
		{Repo: "widget", Number: "3", State: "DRAFT", Draft: true, Title: "Draft second"},
		{Repo: "widget", Number: "4", State: "OPEN", Title: "Open second"},
	}}
	md := snapshot.Markdown()
	previous := -1
	for _, title := range []string{"Open first", "Open second", "Draft first", "Draft second"} {
		at := strings.Index(md, title)
		if at <= previous {
			t.Fatalf("PR ordering lost near %q: %s", title, md)
		}
		previous = at
	}
}
