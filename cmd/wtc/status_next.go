package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/lcorneliussen/wtc-cli/internal/next"
	"github.com/lcorneliussen/wtc-cli/internal/wtc"
)

type statusNextDoneMsg struct {
	text string
	err  error
}

func statusNextRepos(snapshot wtc.StatusSnapshot) []wtc.StatusRepo {
	var repos []wtc.StatusRepo
	for _, r := range snapshot.Repos {
		if r.Next != nil {
			repos = append(repos, r)
		}
	}
	return repos
}

func statusNextLabel(s *next.Step) string {
	if label := map[string]string{"return-to-tip": "Return to tip", "catch-up": "Catch up", "new-branch": "Create a branch", "resolve": "Resolve conflicts", "merge": "Merge the base", "fix-checks": "Fix checks", "address-review": "Address review", "push": "Push", "merge-pr": "Open pull request"}[s.Step]; label != "" {
		return label
	}
	return s.Step
}

func statusNextLines(snapshot wtc.StatusSnapshot, width int, styled bool, selected int) []string {
	repos := statusNextRepos(snapshot)
	if len(repos) == 0 {
		return nil
	}
	lines := []string{"", "Next actions"}
	if styled {
		lines[1] = statusTUIStyle(lines[1], statusToneHeading)
	}
	for i, r := range repos {
		name := r.Dir
		if snapshot.ShowCollectionColumn {
			name = r.Collection + "/" + name
		}
		who := r.Next.By
		if r.Next.Approve != "" {
			who += "; " + r.Next.Approve + " approval"
		}
		prefix := "  "
		if selected == i {
			prefix = "› "
		}
		lines = append(lines, statusTUIFit(statusTUISafe(fmt.Sprintf("%s%s · %s · %s", prefix, name, statusNextLabel(r.Next), who)), width),
			statusTUIFit("    "+statusTUISafe(r.Next.Reason), width))
	}
	return lines
}

// A click selects a fact, not arbitrary argv. A refresh must still propose
// the same step in the same checkout before execution or an agent handoff.
func statusNextCurrent(wanted wtc.StatusRepo, snapshot wtc.StatusSnapshot) (wtc.StatusRepo, error) {
	for _, r := range snapshot.Repos {
		if r.Collection == wanted.Collection && r.Dir == wanted.Dir && r.Worktree == wanted.Worktree && r.Next != nil && reflect.DeepEqual(r.Next, wanted.Next) {
			return r, nil
		}
	}
	return wtc.StatusRepo{}, fmt.Errorf("the next step changed; refresh and choose again")
}

func statusNextCommand(r wtc.StatusRepo) ([]string, bool) {
	s := r.Next
	repo := r.Repo
	if repo == "" {
		repo = r.Dir
	}
	want := []string{"wtc", "catch-up", r.Collection, "--repos", repo}
	if s == nil || s.By != next.ByWTC || s.Approve != "" || !reflect.DeepEqual(s.Command, want) {
		return nil, false
	}
	// Leave any newly dirty or busy checkout with its owner.
	return append(want[1:], "--clean-only", "--idle-only"), true
}

func statusNextRun(c *wtc.Context, selected wtc.StatusRepo) tea.Cmd {
	return func() tea.Msg {
		failed := func(err error) tea.Msg { return statusNextDoneMsg{err: err} }
		if c == nil || selected.Collection == "" || selected.Collection == "." || selected.Collection == ".." || filepath.Base(selected.Collection) != selected.Collection {
			return failed(fmt.Errorf("cannot locate this collection"))
		}
		owner, err := wtc.OpenCollection(filepath.Join(c.Workspace, selected.Collection))
		if err != nil {
			return failed(err)
		}
		fresh, err := owner.StatusForgePreview()
		if err != nil {
			return failed(err)
		}
		r, err := statusNextCurrent(selected, fresh)
		if err != nil {
			return failed(err)
		}
		if argv, ok := statusNextCommand(r); ok {
			binary, err := os.Executable()
			if err != nil {
				return failed(err)
			}
			cmd := exec.Command(binary, argv...)
			cmd.Dir = owner.Collection
			out, err := cmd.CombinedOutput()
			return statusNextDoneMsg{text: strings.TrimSpace(string(out)), err: err}
		}
		if r.Next.By == next.ByPerson && r.Next.Step == "merge-pr" && r.PR != nil && r.PR.URL != "" {
			target := statusTUIURL(r.PR.URL)
			if target == "" {
				return failed(fmt.Errorf("the pull request has no web URL"))
			}
			opened := statusOpenURL(target)().(statusOpenedMsg)
			return statusNextDoneMsg{text: "Opened the pull request", err: opened.err}
		}
		skill := r.Next.Skill
		if r.Next.By == next.ByWTC && r.Next.Approve == next.ApproveAgent && r.Next.Step == "catch-up" {
			skill = "wtc-catch-up"
		}
		if skill == "" || r.Next.Approve == next.ApprovePerson || (r.Next.By != next.ByAgent && r.Next.Approve != next.ApproveAgent) {
			return failed(fmt.Errorf("this step needs its owner's action"))
		}
		prompt := fmt.Sprintf("/skill %s %s: %s. Work on this repository only; recheck its state before acting.", skill, r.Dir, r.Next.Reason)
		instruction := statusNextDoneMsg{text: "Ask the collection's agent: " + prompt}
		// A plain terminal provides the exact instruction. In herdr, submit
		// only to this collection's idle agent, never a running or blocked one.
		session := openSession(owner, "")
		workspaces, err := openWorkspaces(session)
		if err != nil {
			return instruction
		}
		for _, workspace := range workspaces {
			if workspace.Label != r.Collection {
				continue
			}
			panes, err := openPanes(session, workspace.ID)
			if err != nil {
				return failed(err)
			}
			pane := openPaneByLabel(panes, "agent")
			if pane.ID == "" || pane.Agent == "" || (pane.AgentStatus != "idle" && pane.AgentStatus != "done") {
				return instruction
			}
			_, err = openHerdr(session, "agent", "prompt", pane.ID, prompt)
			return statusNextDoneMsg{text: "Asked the collection's agent to " + strings.ToLower(statusNextLabel(r.Next)), err: err}
		}
		return instruction
	}
}
