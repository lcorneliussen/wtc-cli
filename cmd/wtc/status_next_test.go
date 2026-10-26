package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/lcorneliussen/wtc-cli/internal/next"
	"github.com/lcorneliussen/wtc-cli/internal/wtc"
)

func TestStatusNextActions(t *testing.T) {
	r := wtc.StatusRepo{Collection: "task", Dir: "api", Worktree: "/synthetic/task/api", Next: &next.Step{Step: "return-to-tip", By: next.ByWTC, Key: "k", Reason: "The pull request merged", Command: []string{"wtc", "catch-up", "task", "--repos", "api"}}}
	snapshot := wtc.StatusSnapshot{Collection: "task", Repos: []wtc.StatusRepo{r}}
	if text := statusTable(snapshot, true, 100); !strings.Contains(text, "Return to tip") || !strings.Contains(text, r.Next.Reason) {
		t.Fatal(text)
	}
	if _, err := statusNextCurrent(r, snapshot); err != nil {
		t.Fatal(err)
	}
	changed := r
	changed.Next = nil
	if _, err := statusNextCurrent(r, wtc.StatusSnapshot{Repos: []wtc.StatusRepo{changed}}); err == nil {
		t.Fatal("a stale action ran after the checkout became dirty")
	}
	argv, ok := statusNextCommand(r)
	if !ok || strings.Join(argv, " ") != "catch-up task --repos api --clean-only --idle-only" {
		t.Fatal(argv, ok)
	}
	registered := r
	registered.Repo = "service"
	s := *r.Next
	s.Command = []string{"wtc", "catch-up", "task", "--repos", "service"}
	registered.Next = &s
	if _, ok := statusNextCommand(registered); !ok {
		t.Fatal("registry names can differ from sibling directories")
	}
	for _, approval := range []string{next.ApproveAgent, next.ApprovePerson} {
		s := *r.Next
		s.Approve = approval
		changed.Next = &s
		if _, ok := statusNextCommand(changed); ok {
			t.Fatal("executed without required approval", approval)
		}
		if _, err := statusNextCurrent(r, wtc.StatusSnapshot{Repos: []wtc.StatusRepo{changed}}); err == nil {
			t.Fatal("an approval change was not rechecked")
		}
	}
	m := statusTUIModel{snapshot: snapshot, width: 100, height: 30}
	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'n', Text: "n"}))
	m = updated.(statusTUIModel)
	if !m.showNext || !strings.Contains(m.View().Content, "Enter run or ask agent") {
		t.Fatal(m.View().Content)
	}
	m.nextRunning = true
	if _, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})); cmd != nil {
		t.Fatal("a second execution started while the first was running")
	}
}
