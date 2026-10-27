package main

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"errors"
	"github.com/lcorneliussen/wtc-cli/internal/next"
	"github.com/lcorneliussen/wtc-cli/internal/wtc"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func batchFixture(t *testing.T) (*wtc.Context, []wtc.StatusRepo) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "task")
	if err := os.MkdirAll(filepath.Join(root, "harness"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "harness", ".harness-repos.yml"), []byte("repos:\n  - name: api\n    remote: https://github.com/example/api.git\n    default_ref: origin/main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	c, err := wtc.OpenCollection(root)
	if err != nil {
		t.Fatal(err)
	}
	var rows []wtc.StatusRepo
	for _, name := range []string{"api", "web", "docs"} {
		choices := next.ChoicesFor(next.Facts{Collection: "task", Repo: name, Host: next.HostLocal, Branch: "topic", Head: "h1", PR: &next.PR{Number: 7, State: "open", Checks: "success", Mergeable: "mergeable"}})
		rows = append(rows, wtc.StatusRepo{Collection: "task", Dir: name, Repo: name, Worktree: filepath.Join(root, name), Next: choices.Primary(), Choices: &choices})
	}
	return c, rows
}

func TestStatusNextBatchCombinesHandoffsAndKeepsIndependentFailure(t *testing.T) {
	c, rows := batchFixture(t)
	prompts := 0
	reads := 0
	run := statusNextRunner{
		readStatus: func(*wtc.Context) (wtc.StatusSnapshot, error) {
			reads++
			return wtc.StatusSnapshot{Repos: rows[1:]}, nil
		},
		herdr: func(_ string, args ...string) (openHerdrResponse, error) {
			var r openHerdrResponse
			switch args[0] {
			case "workspace":
				r.Result.Workspaces = []openWorkspaceInfo{{ID: "w", Label: "task"}}
			case "pane":
				r.Result.Panes = []openPaneInfo{{ID: "p", Label: "agent", Agent: "codex", AgentStatus: "idle"}}
			case "agent":
				prompts++
				if !strings.Contains(args[3], "web:") || !strings.Contains(args[3], "docs:") || strings.Contains(args[3], "api:") {
					t.Fatal(args[3])
				}
			default:
				t.Fatal(args)
			}
			return r, nil
		},
	}
	result := statusNextBatch(context.Background(), c, rows, run)().(statusNextDoneMsg)
	if len(result.pending) != 2 {
		t.Fatal("accepted targets need duplicate suppression", result)
	}
	if prompts != 1 || reads != 5 || !strings.Contains(result.text, "api: failed or stale") || !strings.Contains(result.text, "completion") {
		t.Fatal(prompts, reads, result)
	}
}

func TestStatusNextBatchCancellationStopsQueuedWork(t *testing.T) {
	c, rows := batchFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	reads := 0
	run := statusNextRunner{readStatus: func(*wtc.Context) (wtc.StatusSnapshot, error) {
		reads++
		cancel()
		return wtc.StatusSnapshot{Repos: rows}, nil
	}, herdr: func(string, ...string) (openHerdrResponse, error) {
		t.Fatal("cancelled handoff ran")
		return openHerdrResponse{}, nil
	}}
	result := statusNextBatch(ctx, c, rows, run)().(statusNextDoneMsg)
	if reads != 1 || !strings.Contains(result.text, "cancelled before") {
		t.Fatal(reads, result)
	}
}

func TestStatusNextAlternativeRequiresExactFreshChoice(t *testing.T) {
	_, rows := batchFixture(t)
	r := rows[0]
	r = statusNextCycle(r)
	if r.Next.Step != "review" {
		t.Fatal(r.Next)
	}
	got, err := statusNextCurrent(r, wtc.StatusSnapshot{Repos: rows})
	if err != nil || got.Next.Step != "review" {
		t.Fatal(got, err)
	}
	changed := rows[0]
	choices := next.ChoicesFor(next.Facts{Collection: "task", Repo: "api", Head: "h2", Dirty: 1})
	changed.Choices = &choices
	if _, err := statusNextCurrent(r, wtc.StatusSnapshot{Repos: []wtc.StatusRepo{changed}}); err == nil {
		t.Fatal("stale alternative accepted")
	}
}

func TestStatusNextSelectionPreviewAndRemovedTargets(t *testing.T) {
	c, rows := batchFixture(t)
	m := statusTUIModel{context: c, snapshot: wtc.StatusSnapshot{Repos: rows}, showNext: true, width: 100, height: 30}
	key := func(code rune, text string) tea.Cmd {
		model, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: code, Text: text}))
		m = model.(statusTUIModel)
		return cmd
	}
	if cmd := key('s', "s"); cmd != nil || len(m.nextSelected) != 3 {
		t.Fatal(m.nextSelected)
	}
	if cmd := key(tea.KeyEnter, ""); cmd != nil || len(m.nextPreview) != 3 || m.nextRunning {
		t.Fatal("preview dispatched")
	}
	if !strings.Contains(m.nextNotice, "Enter confirms") {
		t.Fatal(m.nextNotice)
	}
	m.snapshot.Repos = m.snapshot.Repos[1:]
	targets := statusNextTargets(m.snapshot, m.nextSelected)
	if len(targets) != 3 || targets[2].Dir != "api" {
		t.Fatal("removed target silently lost", targets)
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.nextCancel = cancel
	m.nextRunning = true
	key('q', "q")
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatal("quit left queued actions running")
	}
}

func TestStatusNextPendingHandoffRequiresExplicitRetry(t *testing.T) {
	_, rows := batchFixture(t)
	m := statusTUIModel{snapshot: wtc.StatusSnapshot{Repos: rows[:1]}, showNext: true, nextPending: map[string]bool{statusNextPendingKey(rows[0]): true}}
	model, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = model.(statusTUIModel)
	if cmd != nil || !strings.Contains(m.nextNotice, "handoff is pending") {
		t.Fatal("duplicate dispatched", m.nextNotice)
	}
	model, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 's', Text: "s"}))
	m = model.(statusTUIModel)
	if len(m.nextSelected) != 0 {
		t.Fatal("pending recommendation selected")
	}
	model, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'x', Text: "x"}))
	m = model.(statusTUIModel)
	if len(m.nextPending) != 0 {
		t.Fatal("explicit retry did not reset pending marker")
	}
}
