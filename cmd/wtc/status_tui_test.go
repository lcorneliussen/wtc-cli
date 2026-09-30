package main

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/lcorneliussen/wtc-cli/internal/wtc"
	"github.com/mattn/go-runewidth"
)

func TestStatusTUIRepoRowsKeepSignalsWithinWidth(t *testing.T) {
	snapshot := wtc.StatusSnapshot{Collection: "fixture", Repos: []wtc.StatusRepo{{
		Dir: "widget", BranchDisplay: "topic", Tree: "±2", Ahead: 1, Behind: 3,
		PR: &wtc.StatusPRFacts{Number: "7", Checks: "SUCCESS", Merge: "BEHIND", Review: "waiting"},
	}}}
	lines := statusTUIRepoLines(snapshot, 60)
	if len(lines) != 2 || !strings.Contains(lines[1], "#7 ✓ ↓ …") || !strings.Contains(lines[1], "±2 ↑1 ↓3") {
		t.Fatalf("repo signals missing: %q", lines)
	}
	if fitted := statusTUIFit("⌂ development-tip", 8); runewidth.StringWidth(fitted) != 8 || !strings.HasSuffix(fitted, "…") {
		t.Fatalf("wide branch clipping failed: %q", fitted)
	}
}

func TestStatusTUIModelShowsCachedRowsAndControls(t *testing.T) {
	merged := "MERGED"
	model := statusTUIModel{snapshot: wtc.StatusSnapshot{Collection: "fixture", Repos: []wtc.StatusRepo{{Dir: "widget", BranchDisplay: "⌂ main", Tree: "clean"}},
		PRs: []wtc.StatusPRRow{{Repo: "widget", Number: "7", Title: "Old change", Merge: &merged, Archived: true}}},
		width: 90, height: 20, focused: true, interval: 30 * time.Second, background: 5 * time.Minute,
		lastRefresh: time.Now().Add(-time.Minute), nextRefresh: time.Now().Add(time.Minute)}
	view := model.View().Content
	if !strings.Contains(view, "wtc status · fixture") || !strings.Contains(view, "widget") || !strings.Contains(view, "1 archived PR(s) hidden") || strings.Contains(view, "Old change") {
		t.Fatalf("cached view wrong: %s", view)
	}
	updated, _ := model.Update(tea.KeyPressMsg(tea.Key{Code: 'a', Text: "a"}))
	model = updated.(statusTUIModel)
	if !model.showArchived || !strings.Contains(model.View().Content, "Old change") {
		t.Fatal("archive toggle did not reveal merged PR")
	}
	updated, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: '?', Text: "?"}))
	model = updated.(statusTUIModel)
	if !model.showHelp || !strings.Contains(model.View().Content, "PgUp/PgDn") {
		t.Fatal("help toggle failed")
	}
	model.refreshing = true
	updated, _ = model.Update(statusLoadedMsg{snapshot: wtc.StatusSnapshot{Collection: "fixture"}, at: time.Now()})
	model = updated.(statusTUIModel)
	if model.refreshing || len(model.snapshot.Repos) != 0 || model.nextRefresh.IsZero() {
		t.Fatalf("refresh state did not advance: %+v", model)
	}
}

func TestStatusTUIClicksOnlyBuildCells(t *testing.T) {
	tipURL, prodURL := "https://example.invalid/build/7", "https://example.invalid/build/8"
	snapshot := wtc.StatusSnapshot{Collection: "fixture", Repos: []wtc.StatusRepo{{Dir: "widget", BranchDisplay: "topic",
		Tip: &wtc.StatusBuild{Branch: "main", URL: &tipURL}, Prod: &wtc.StatusBuild{Branch: "prod", URL: &prodURL}}}}
	m := statusTUIModel{snapshot: snapshot, width: 80, height: 20}
	if m.View().MouseMode != tea.MouseModeCellMotion {
		t.Fatal("build URLs did not enable mouse input")
	}
	if got := m.buildClickTarget(63, 3); got != tipURL {
		t.Fatalf("tip cell target: %q", got)
	}
	if got := m.buildClickTarget(72, 3); got != prodURL {
		t.Fatalf("prod cell target: %q", got)
	}
	for _, x := range []int{0, 20, 40, 62} {
		if got := m.buildClickTarget(x, 3); got != "" {
			t.Fatalf("non-build cell opened URL at %d: %q", x, got)
		}
	}
	m.scroll = 999
	if got := m.buildClickTarget(63, 3); got != tipURL {
		t.Fatalf("click map disagreed with clamped view scroll: %q", got)
	}
	m.noClick = true
	if m.View().MouseMode != tea.MouseModeNone {
		t.Fatal("--no-click did not disable mouse mode")
	}
}

func TestStatusOneShotTableCanHidePRSection(t *testing.T) {
	snapshot := wtc.StatusSnapshot{Collection: "fixture", GeneratedAt: "2026-09-30T00:00:00Z",
		Repos: []wtc.StatusRepo{{Dir: "widget", BranchDisplay: "topic", Tree: "clean"}},
		PRs:   []wtc.StatusPRRow{{Repo: "widget", Number: "7", Title: "Synthetic PR"}}}
	full := statusTable(snapshot, false, 80)
	repos := statusTable(snapshot, true, 80)
	if !strings.Contains(full, "Synthetic PR") || strings.Contains(repos, "Synthetic PR") || !strings.Contains(repos, "widget") {
		t.Fatalf("repository-only table wrong: full=%s repos=%s", full, repos)
	}
}
