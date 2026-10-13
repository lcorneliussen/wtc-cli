package main

import (
	"strings"
	"unicode"

	"github.com/lcorneliussen/wtc-cli/internal/wtc"
)

type statusRuntimeLayout struct{ repo, task, kind, state, detail int }

func statusTUIRuntimeLayout(snapshot wtc.StatusSnapshot, width int) statusRuntimeLayout {
	l := statusRuntimeLayout{repo: statusTUIPRLayout(width).repo, task: 10, kind: 9, state: 11}
	for _, item := range snapshot.Runtime {
		l.task = max(l.task, len([]rune(statusRuntimeTask(item))))
	}
	l.task = min(l.task, max(10, width/4))
	if width < 60 {
		l.kind = 0
	}
	l.detail = max(1, width-l.detailStart())
	return l
}

func (l statusRuntimeLayout) detailStart() int {
	start := 2 + l.repo + 1 + l.task + 1 + l.state + 1
	if l.kind > 0 {
		start += l.kind + 1
	}
	return start
}

// statusRuntimeTask is a task's path inside its repository.
func statusRuntimeTask(item wtc.RuntimeItem) string {
	return strings.TrimPrefix(item.Path, item.Dir+"/")
}

// statusRuntimeState shortens the hook fact the way the repo summary does.
func statusRuntimeState(item wtc.RuntimeItem) string {
	if item.Kind == "resources" && item.State == "provision-hook-completed" {
		return "provisioned"
	}
	return item.State
}

// statusTUIRuntimeLines draws services, tunnels and resource hooks as a table
// in the style of the PR list: the LOCAL mark, repo, task, kind, state, then the URL
// or a note.
func statusTUIRuntimeLines(snapshot wtc.StatusSnapshot, width int, styled bool) []string {
	if len(snapshot.Runtime) == 0 {
		return nil
	}
	l := statusTUIRuntimeLayout(snapshot, width)
	heading := "LOCAL"
	header := "  " + statusTUIFit("REPO", l.repo) + " " + statusTUIFit("TASK", l.task) + " "
	if l.kind > 0 {
		header += statusTUIFit("KIND", l.kind) + " "
	}
	header += statusTUIFit("STATE", l.state) + " " + statusTUIFit("URL", l.detail)
	if styled {
		heading, header = statusTUIStyle(heading, statusToneHeading), statusTUIStyle(header, statusToneDim)
	}
	lines := []string{"", heading, header}
	for _, item := range snapshot.Runtime {
		mark, tone := statusRuntimeMark(item)
		repo := statusTUIFit(statusTUISafe(item.Dir), l.repo)
		task := statusTUIFit(statusTUISafe(statusRuntimeTask(item)), l.task)
		kind := statusTUIFit(statusTUISafe(item.Kind), l.kind)
		state := statusTUIFit(statusTUISafe(statusRuntimeState(item)), l.state)
		notes := []string{}
		for _, note := range []string{item.Label, item.Reason, item.Error} {
			if note != "" {
				notes = append(notes, statusTUISafe(note))
			}
		}
		note := strings.Join(notes, " · ")
		detail := note
		target := statusTUIURL(item.URL)
		if item.URL != "" {
			detail = statusTUISafe(item.URL)
			if note != "" {
				detail += "  " + note
			}
		}
		detail = statusTUIFit(detail, l.detail)
		if styled {
			mark, task, state = statusTUIStyle(mark, tone), statusTUIStyle(task, statusToneLabel), statusTUIStyle(state, tone)
			repo = statusTUIStyle(repo, statusToneLabel)
			kind = statusTUIStyle(kind, statusToneDim)
			if shown := statusTUIFit(statusTUISafe(item.URL), l.detail); target != "" && strings.TrimSpace(shown) != "" {
				rest := strings.TrimPrefix(detail, strings.TrimRight(shown, " "))
				detail = statusTUILinkTone(strings.TrimRight(shown, " "), item.URL, statusToneLink) + statusTUIStyle(rest, statusToneDim)
			} else {
				detail = statusTUIStyle(detail, statusToneDim)
			}
		}
		line := mark + " " + repo + " " + task + " "
		if l.kind > 0 {
			line += kind + " "
		}
		lines = append(lines, line+state+" "+detail)
	}
	return lines
}

// statusRuntimeItems lists the services, tunnels and resource hooks a repo row owns.
func statusRuntimeItems(snapshot wtc.StatusSnapshot, row wtc.StatusRepo) []wtc.RuntimeItem {
	var items []wtc.RuntimeItem
	for _, item := range snapshot.Runtime {
		if item.Repo == row.Repo && item.Collection == row.Collection {
			items = append(items, item)
		}
	}
	return items
}

// statusRuntimeMark is one letter for a task, toned by its state: the first
// letter of its name, upper-case for an endpoint such as a tunnel.
func statusRuntimeMark(item wtc.RuntimeItem) (string, string) {
	name := strings.TrimPrefix(item.Path[strings.LastIndex(item.Path, "/")+1:], "@")
	letter := '?'
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			letter = unicode.ToLower(r)
			break
		}
	}
	if item.Kind == "endpoint" {
		letter = unicode.ToUpper(letter)
	}
	tone := statusToneFailure
	switch item.State {
	case "ready", "provision-hook-completed", "done":
		tone = statusToneSuccess
	case "starting", "running", "provision-attempted":
		tone = statusToneWarning
	case "absent", "unconfigured", "stopped", "idle":
		tone = statusToneDim
	}
	return string(letter), tone
}

func statusRuntimeCell(items []wtc.RuntimeItem, width int, styled bool) string {
	var b strings.Builder
	for i, item := range items {
		if i >= width {
			break
		}
		letter, tone := statusRuntimeMark(item)
		if styled {
			letter = statusTUIStyle(letter, tone)
		}
		b.WriteString(letter)
	}
	return b.String() + strings.Repeat(" ", max(0, width-min(len(items), width)))
}

// statusRuntimeHover is the footer text for the mark under the pointer.
func statusRuntimeHover(item wtc.RuntimeItem) string {
	parts := []string{item.Path, item.Kind, item.State}
	for _, extra := range []string{item.Label, item.URL, item.Reason, item.Error} {
		if extra != "" {
			parts = append(parts, extra)
		}
	}
	return statusTUISafe(strings.Join(parts, " · "))
}
