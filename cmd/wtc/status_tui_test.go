package main

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/lcorneliussen/wtc-cli/internal/wtc"
	"github.com/mattn/go-runewidth"
)

func statusTestMessage(t *testing.T, command tea.Cmd) tea.Msg {
	t.Helper()
	messages := make(chan tea.Msg, 1)
	go func() { messages <- command() }()
	select {
	case message := <-messages:
		return message
	case <-time.After(3 * time.Second):
		t.Fatal("TUI refresh event did not arrive")
		return nil
	}
}

func TestStatusTUILinksColorsAndClickTargets(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("NO_COLOR", "")
	prURL := "https://github.com/example/widget/pull/7"
	buildURL := "https://example.invalid/build/42"
	passed := "SUCCESS"
	merged := "MERGED"
	model := statusTUIModel{width: 100, height: 24, snapshot: wtc.StatusSnapshot{Collection: "fixture",
		Repos: []wtc.StatusRepo{{Dir: "widget", BranchDisplay: "feature", Tree: "clean",
			PR:  &wtc.StatusPRFacts{Number: "7", URL: prURL, Checks: "SUCCESS"},
			Tip: &wtc.StatusBuild{Checks: &passed, URL: &buildURL}}},
		PRs: []wtc.StatusPRRow{{Repo: "widget", Number: "7", Title: "Change widget", URL: &prURL, Merge: &merged}}}}
	view := model.View().Content
	if strings.Count(view, ansi.SetHyperlink(prURL)) < 2 || !strings.Contains(view, ansi.SetHyperlink(buildURL)) ||
		!strings.Contains(view, "\x1b[4;38;5;81m") || !strings.Contains(view, "\x1b[1;38;5;180m") {
		t.Fatalf("TUI lost terminal links or visual hierarchy: %q", view)
	}
	for _, line := range strings.Split(view, "\n") {
		if ansi.StringWidth(line) > model.width {
			t.Fatalf("styled line exceeded terminal width: %q", line)
		}
	}
	if got := model.buildClickTarget(54, 3); got != prURL {
		t.Fatalf("repo PR cell click = %q", got)
	}
	if got := model.buildClickTarget(83, 3); got != buildURL {
		t.Fatalf("build cell click = %q", got)
	}
	if got := model.buildClickTarget(5, 6); got != prURL {
		t.Fatalf("PR list row click = %q", got)
	}
	t.Setenv("NO_COLOR", "1")
	view = model.View().Content
	if strings.Contains(view, "\x1b[4;38;5;81m") || !strings.Contains(view, ansi.SetHyperlink(prURL)) {
		t.Fatal("NO_COLOR suppressed links or retained styling")
	}
}

func TestStatusTUIRefreshStreamsCollectorProgressAndCompletion(t *testing.T) {
	model := statusTUIModel{snapshot: wtc.StatusSnapshot{Collection: "fixture"}, width: 80, height: 16}
	model.startRefresh()
	command := statusTUIRefreshWithCollector(&wtc.Context{}, false, func(c *wtc.Context) statusLoadedMsg {
		c.StatusProgress("Checking remote refs")
		return statusLoadedMsg{snapshot: wtc.StatusSnapshot{Collection: "fixture", Schema: 1}, at: time.Now()}
	})
	started := statusTestMessage(t, command)
	updated, wait := model.Update(started)
	model = updated.(statusTUIModel)
	progress := statusTestMessage(t, wait)
	updated, wait = model.Update(progress)
	model = updated.(statusTUIModel)
	if !strings.Contains(model.View().Content, "Checking remote refs") {
		t.Fatalf("collector progress did not reach the TUI: %s", model.View().Content)
	}
	loaded := statusTestMessage(t, wait)
	updated, _ = model.Update(loaded)
	model = updated.(statusTUIModel)
	if model.refreshing || model.snapshot.Schema != 1 {
		t.Fatalf("collector completion did not reach the TUI: %+v", model)
	}

	model.startRefresh()
	command = statusTUIRefreshWithCollector(&wtc.Context{}, false, func(*wtc.Context) statusLoadedMsg {
		return statusLoadedMsg{err: errors.New("fixture failure"), at: time.Now()}
	})
	started = statusTestMessage(t, command)
	updated, wait = model.Update(started)
	model = updated.(statusTUIModel)
	loaded = statusTestMessage(t, wait)
	updated, _ = model.Update(loaded)
	model = updated.(statusTUIModel)
	if model.refreshing || !strings.Contains(model.errorText, "fixture failure") ||
		!strings.Contains(strings.Join(model.progressLog, "\n"), "Refresh failed") {
		t.Fatalf("collector failure did not reach the TUI: %+v", model)
	}
}

func TestStatusTUIRepoRowsKeepSignalsWithinWidth(t *testing.T) {
	snapshot := wtc.StatusSnapshot{Collection: "fixture", Repos: []wtc.StatusRepo{{
		Dir: "widget", BranchDisplay: "topic", Tree: "±2", Ahead: 1, Behind: 3,
		PR: &wtc.StatusPRFacts{Number: "7", Checks: "SUCCESS", Merge: "BEHIND", Review: "waiting"},
	}}}
	lines := statusTUIRepoLines(snapshot, 60, false)
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
	if !strings.Contains(view, "wtc status · fixture") || !strings.Contains(view, "widget") || !strings.Contains(view, "archived (1)") || strings.Contains(view, "Old change") {
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

func TestStatusTUIRefreshLogUpdatesWhileCollectorRuns(t *testing.T) {
	model := statusTUIModel{snapshot: wtc.StatusSnapshot{Collection: "fixture"}, width: 80, height: 16}
	model.startRefresh()
	events := make(chan tea.Msg)
	updated, next := model.Update(statusProgressMsg{message: "Checked pull requests 2/4", at: model.startedAt.Add(2 * time.Second), events: events})
	model = updated.(statusTUIModel)
	if next == nil || !strings.Contains(model.View().Content, "Checked pull requests 2/4") ||
		model.View().MouseMode != tea.MouseModeCellMotion {
		t.Fatalf("refresh progress was not visible or clickable: %s", model.View().Content)
	}
	refreshAt := runewidth.StringWidth(strings.Split(model.headerLine(), "refreshing")[0])
	updated, _ = model.Update(tea.MouseClickMsg{X: refreshAt, Y: 0})
	model = updated.(statusTUIModel)
	if !model.showLog || !strings.Contains(model.View().Content, "2s  Checked pull requests 2/4") {
		t.Fatalf("refresh click did not open log: %s", model.View().Content)
	}
	updated, _ = model.Update(statusLoadedMsg{snapshot: wtc.StatusSnapshot{Collection: "fixture"}, at: model.startedAt.Add(3 * time.Second)})
	model = updated.(statusTUIModel)
	if model.refreshing || !strings.Contains(model.View().Content, "3s  Refresh finished") {
		t.Fatalf("refresh completion did not appear in log: %s", model.View().Content)
	}
}

func TestStatusTUIRefreshLogKeepsNewestEntryVisible(t *testing.T) {
	model := statusTUIModel{snapshot: wtc.StatusSnapshot{Collection: "fixture"}, width: 80, height: 12, showLog: true, showHelp: true}
	model.startRefresh()
	for i := 0; i < 20; i++ {
		model.progressLog = append(model.progressLog, "step")
		model.focusLogTail()
	}
	model.progressLog = append(model.progressLog, "latest progress")
	model.focusLogTail()
	if !strings.Contains(model.View().Content, "latest progress") {
		t.Fatalf("latest progress hidden while refreshing: %s", model.View().Content)
	}
	updated, _ := model.Update(statusLoadedMsg{snapshot: wtc.StatusSnapshot{Collection: "fixture"}, at: model.startedAt.Add(time.Second)})
	model = updated.(statusTUIModel)
	if !strings.Contains(model.View().Content, "Refresh finished") {
		t.Fatalf("completion hidden after refresh: %s", model.View().Content)
	}
}

func TestStatusTUIRefreshLogReportsFetchFallback(t *testing.T) {
	model := statusTUIModel{snapshot: wtc.StatusSnapshot{Collection: "fixture"}, width: 80, height: 12, showLog: true}
	model.startRefresh()
	updated, _ := model.Update(statusLoadedMsg{snapshot: wtc.StatusSnapshot{Collection: "fixture"},
		fetched: wtc.StatusFetchReport{Attempted: 2, Failed: 1}, at: model.startedAt.Add(time.Second)})
	model = updated.(statusTUIModel)
	if !strings.Contains(model.View().Content, "1 ref refresh failed; showing local refs") {
		t.Fatalf("failed fetch was hidden from refresh log: %s", model.View().Content)
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
