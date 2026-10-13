package main

import (
	"strings"
	"unicode"

	"github.com/lcorneliussen/wtc-cli/internal/wtc"
)

// statusRuntimeGroup is one line of the detail table: a service with its
// tunnels, or a repository's resource hooks.
type statusRuntimeGroup struct {
	repo, name string
	members    []wtc.RuntimeItem
	local      string
	tunnels    []string
	notes      []string
}

func statusRuntimeGroups(snapshot wtc.StatusSnapshot) []statusRuntimeGroup {
	var groups []statusRuntimeGroup
	index := map[string]int{}
	for _, item := range snapshot.Runtime {
		key := item.Collection + "\x00" + item.Group
		at, ok := index[key]
		if !ok {
			at, index[key] = len(groups), len(groups)
			groups = append(groups, statusRuntimeGroup{repo: item.Dir, name: strings.TrimPrefix(item.Group, item.Dir+"/")})
		}
		g := &groups[at]
		g.members = append(g.members, item)
		if item.URL != "" {
			if item.Kind == "endpoint" {
				g.tunnels = append(g.tunnels, item.URL)
			} else if g.local == "" {
				g.local = item.URL
			}
		}
		for _, note := range []string{item.Reason, item.Error} {
			if note != "" {
				g.notes = append(g.notes, note)
			}
		}
	}
	return groups
}

// state is the member state that most needs attention, with its tone.
func (g statusRuntimeGroup) state() (string, string) {
	rank := map[string]int{statusToneDim: 1, statusToneSuccess: 2, statusToneWarning: 3, statusToneFailure: 4}
	state, tone := "", ""
	for _, item := range g.members {
		if _, t := statusRuntimeMark(item); rank[t] > rank[tone] {
			state, tone = statusRuntimeState(item), t
		}
	}
	return state, tone
}

type statusRuntimeLayout struct{ marks, repo, name, state, local, tunnel int }

func statusTUIRuntimeLayout(snapshot wtc.StatusSnapshot, width int) statusRuntimeLayout {
	l := statusRuntimeLayout{marks: 1, repo: statusTUIPRLayout(width).repo, name: 7, state: 11, local: 5}
	for _, g := range statusRuntimeGroups(snapshot) {
		l.marks = max(l.marks, len(g.members))
		l.name = max(l.name, len([]rune(g.name)))
		l.local = max(l.local, len([]rune(g.local)))
	}
	l.name, l.local = min(l.name, max(7, width/6)), min(l.local, max(5, width/3))
	l.tunnel = max(1, width-l.tunnelStart())
	return l
}

func (l statusRuntimeLayout) localStart() int {
	return l.marks + 1 + l.repo + 1 + l.name + 1 + l.state + 1
}
func (l statusRuntimeLayout) tunnelStart() int { return l.localStart() + l.local + 1 }

// urlAt is the URL drawn at a column of a group's line.
func (l statusRuntimeLayout) urlAt(g statusRuntimeGroup, x int) string {
	if x >= l.localStart() && x < l.localStart()+min(l.local, len([]rune(g.local))) {
		return g.local
	}
	start := l.tunnelStart()
	for _, tunnel := range g.tunnels {
		if x >= start && x < start+len([]rune(tunnel)) {
			return tunnel
		}
		start += len([]rune(tunnel)) + 2
	}
	return ""
}

// statusRuntimeState shortens the hook fact the way the repo summary does.
func statusRuntimeState(item wtc.RuntimeItem) string {
	if item.Kind == "resources" && item.State == "provision-hook-completed" {
		return "provisioned"
	}
	return item.State
}

// statusTUIRuntimeLines draws one line per service in the style of the PR
// list: the LOCAL marks of the service and its tunnels, repo, name, the state
// that most needs attention, the local URL, then the tunnel URLs.
func statusTUIRuntimeLines(snapshot wtc.StatusSnapshot, width int, styled bool) []string {
	if len(snapshot.Runtime) == 0 {
		return nil
	}
	l := statusTUIRuntimeLayout(snapshot, width)
	heading := "LOCAL"
	header := strings.Repeat(" ", l.marks) + " " + statusTUIFit("REPO", l.repo) + " " + statusTUIFit("SERVICE", l.name) + " " +
		statusTUIFit("STATE", l.state) + " " + statusTUIFit("URL", l.local) + " " + statusTUIFit("TUNNEL", l.tunnel)
	header = statusTUIFit(header, width)
	if styled {
		heading, header = statusTUIStyle(heading, statusToneHeading), statusTUIStyle(header, statusToneDim)
	}
	lines := []string{"", heading, header}
	for _, g := range statusRuntimeGroups(snapshot) {
		marks := statusRuntimeCell(g.members, l.marks, styled)
		state, tone := g.state()
		repo, name := statusTUIFit(statusTUISafe(g.repo), l.repo), statusTUIFit(statusTUISafe(g.name), l.name)
		stateCell := statusTUIFit(statusTUISafe(state), l.state)
		local := statusTUIFit(statusTUISafe(g.local), l.local)
		if styled {
			repo, name, stateCell = statusTUIStyle(repo, statusToneLabel), statusTUIStyle(name, statusToneLabel), statusTUIStyle(stateCell, tone)
			if statusTUIURL(g.local) != "" {
				local = statusTUIFitANSI(statusTUILinkTone(strings.TrimRight(local, " "), g.local, statusToneLink), l.local)
			}
		}
		var tunnel strings.Builder
		used := 0
		for i, raw := range g.tunnels {
			text := statusTUISafe(raw)
			if i > 0 {
				tunnel.WriteString("  ")
				used += 2
			}
			text = strings.TrimRight(statusTUIFit(text, min(len([]rune(text)), max(0, l.tunnel-used))), " ")
			used += len([]rune(text))
			if styled && statusTUIURL(raw) != "" {
				text = statusTUILinkTone(text, raw, statusToneLink)
			}
			tunnel.WriteString(text)
		}
		if notes := statusTUISafe(strings.Join(g.notes, " · ")); notes != "" && used+2 < l.tunnel {
			if used > 0 {
				notes = "  " + notes
			}
			notes = strings.TrimRight(statusTUIFit(notes, min(len([]rune(notes)), l.tunnel-used)), " ")
			if styled {
				notes = statusTUIStyle(notes, statusToneDim)
			}
			tunnel.WriteString(notes)
		}
		line := marks + " " + repo + " " + name + " " + stateCell + " " + local + " " + tunnel.String()
		if styled {
			line = statusTUIFitANSI(line, width)
		} else {
			line = statusTUIFit(line, width)
		}
		lines = append(lines, line)
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
