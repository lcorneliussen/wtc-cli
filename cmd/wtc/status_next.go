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

// Keep process boundaries replaceable in tests; state comparison and routing
// are the same as in the live view.
type statusNextRunner struct {
	readStatus func(*wtc.Context) (wtc.StatusSnapshot, error)
	busy       func(*wtc.Context) (string, error)
	command    func(*wtc.Context, []string) (string, error)
	openURL    func(string) error
	herdr      func(string, ...string) (openHerdrResponse, error)
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
	return statusNextRunWith(c, selected, statusNextRunner{
		readStatus: (*wtc.Context).StatusForgePreview,
		busy:       (*wtc.Context).AgentBusy,
		command: func(owner *wtc.Context, argv []string) (string, error) {
			binary, err := os.Executable()
			if err != nil {
				return "", err
			}
			cmd := exec.Command(binary, argv...)
			cmd.Dir = owner.Collection
			out, err := cmd.CombinedOutput()
			return strings.TrimSpace(string(out)), err
		},
		openURL: func(target string) error { return statusOpenURL(target)().(statusOpenedMsg).err },
		herdr:   openHerdr,
	})
}

func statusNextRunWith(c *wtc.Context, selected wtc.StatusRepo, run statusNextRunner) tea.Cmd {
	return func() tea.Msg {
		failed := func(err error) tea.Msg { return statusNextDoneMsg{err: err} }
		if c == nil || selected.Collection == "" || selected.Collection == "." || selected.Collection == ".." || filepath.Base(selected.Collection) != selected.Collection {
			return failed(fmt.Errorf("cannot locate this collection"))
		}
		owner, err := wtc.OpenCollection(filepath.Join(c.Workspace, selected.Collection))
		if err != nil {
			return failed(err)
		}
		fresh, err := run.readStatus(owner)
		if err != nil {
			return failed(err)
		}
		r, err := statusNextCurrent(selected, fresh)
		if err != nil {
			return failed(err)
		}
		if argv, ok := statusNextCommand(r); ok {
			state, err := run.busy(owner)
			if err != nil {
				return failed(err)
			}
			if state != "" {
				return failed(fmt.Errorf("the collection's agent is %s; leave the checkout with its owner", state))
			}
			out, err := run.command(owner, argv)
			return statusNextDoneMsg{text: out, err: err}
		}
		if r.Next.By == next.ByPerson && r.Next.Step == "merge-pr" && r.PR != nil && r.PR.URL != "" {
			target := statusTUIURL(r.PR.URL)
			if target == "" {
				return failed(fmt.Errorf("the pull request has no web URL"))
			}
			return statusNextDoneMsg{text: "Opened the pull request", err: run.openURL(target)}
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
		workspaces, err := run.herdr(session, "workspace", "list")
		if err != nil {
			return instruction
		}
		for _, workspace := range workspaces.Result.Workspaces {
			if workspace.Label != r.Collection {
				continue
			}
			panes, err := run.herdr(session, "pane", "list", "--workspace", workspace.ID)
			if err != nil {
				return failed(err)
			}
			pane := openPaneByLabel(panes.Result.Panes, "agent")
			if pane.ID == "" || pane.Agent == "" || (pane.AgentStatus != "idle" && pane.AgentStatus != "done") {
				return instruction
			}
			_, err = run.herdr(session, "agent", "prompt", pane.ID, prompt)
			return statusNextDoneMsg{text: "Asked the collection's agent to " + strings.ToLower(statusNextLabel(r.Next)), err: err}
		}
		return instruction
	}
}
