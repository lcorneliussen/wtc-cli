package main

import (
	"net/url"
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
	tunnels    []string // protected, such as a tailnet address
	public     []string // reachable from the internet
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
			if item.Kind == "endpoint" && item.Public {
				g.public = append(g.public, item.URL)
			} else if item.Kind == "endpoint" {
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

// statusRuntimeURLLabel is the short clickable name of a URL: the port for a
// loopback address, the provider for a known tunnel domain, else the host.
func statusRuntimeURLLabel(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return raw
	}
	host := u.Hostname()
	if host == "localhost" || strings.HasPrefix(host, "127.") || host == "::1" {
		if u.Port() != "" {
			return ":" + u.Port()
		}
		return host
	}
	for suffix, name := range map[string]string{".ts.net": "tailscale", ".trycloudflare.com": "cloudflare", ".ngrok-free.app": "ngrok", ".ngrok.app": "ngrok", ".ngrok.io": "ngrok", ".devtunnels.ms": "devtunnel"} {
		if strings.HasSuffix(host, suffix) {
			return name
		}
	}
	return host
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

type statusRuntimeLayout struct{ marks, repo, name, state, local, tunnel, public int }

func statusTUIRuntimeLayout(snapshot wtc.StatusSnapshot, width int) statusRuntimeLayout {
	l := statusRuntimeLayout{marks: 1, repo: statusTUIPRLayout(width).repo, name: 7, state: 11, local: 5}
	for _, g := range statusRuntimeGroups(snapshot) {
		l.marks = max(l.marks, len(g.members))
		l.name = max(l.name, len([]rune(g.name)))
		l.local = max(l.local, len([]rune(statusRuntimeURLLabel(g.local))))
	}
	l.name, l.local = min(l.name, max(7, width/6)), min(l.local, max(5, width/3))
	// TUNNEL holds protected endpoints; PUBLIC appears only when one exists.
	l.tunnel = len("TUNNEL")
	for _, g := range statusRuntimeGroups(snapshot) {
		l.tunnel = max(l.tunnel, statusRuntimeLinksWidth(g.tunnels))
		if len(g.public) > 0 {
			l.public = max(l.public, len("PUBLIC"), statusRuntimeLinksWidth(g.public))
		}
	}
	return l
}

func (l statusRuntimeLayout) localStart() int {
	return l.marks + 1 + l.repo + 1 + l.name + 1 + l.state + 1
}
func (l statusRuntimeLayout) tunnelStart() int { return l.localStart() + l.local + 1 }
func (l statusRuntimeLayout) publicStart() int { return l.tunnelStart() + l.tunnel + 1 }

func statusRuntimeLinksWidth(urls []string) int {
	width := 0
	for i, raw := range urls {
		if i > 0 {
			width += 2
		}
		width += len([]rune(statusRuntimeURLLabel(raw)))
	}
	return width
}

// urlAt is the URL drawn at a column of a group's line.
func (l statusRuntimeLayout) urlAt(g statusRuntimeGroup, x int) string {
	if x >= l.localStart() && x < l.localStart()+min(l.local, len([]rune(statusRuntimeURLLabel(g.local)))) {
		return g.local
	}
	for _, cell := range []struct {
		start int
		urls  []string
	}{{l.tunnelStart(), g.tunnels}, {l.publicStart(), g.public}} {
		start := cell.start
		for _, raw := range cell.urls {
			label := len([]rune(statusRuntimeURLLabel(raw)))
			if x >= start && x < start+label {
				return raw
			}
			start += label + 2
		}
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
// that most needs attention, then the local URL, protected tunnels and public
// tunnels as short links; hover and click give the full address.
func statusTUIRuntimeLines(snapshot wtc.StatusSnapshot, width int, styled bool) []string {
	if len(snapshot.Runtime) == 0 {
		return nil
	}
	l := statusTUIRuntimeLayout(snapshot, width)
	heading := "LOCAL"
	header := strings.Repeat(" ", l.marks) + " " + statusTUIFit("REPO", l.repo) + " " + statusTUIFit("SERVICE", l.name) + " " +
		statusTUIFit("STATE", l.state) + " " + statusTUIFit("URL", l.local) + " " + statusTUIFit("TUNNEL", l.tunnel)
	if l.public > 0 {
		header += " " + statusTUIFit("PUBLIC", l.public)
	}
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
		local := statusTUIFit(statusTUISafe(statusRuntimeURLLabel(g.local)), l.local)
		if styled {
			repo, name, stateCell = statusTUIStyle(repo, statusToneLabel), statusTUIStyle(name, statusToneLabel), statusTUIStyle(stateCell, tone)
			if statusTUIURL(g.local) != "" {
				local = statusTUIFitANSI(statusTUILinkTone(strings.TrimRight(local, " "), g.local, statusToneLink), l.local)
			}
		}
		// Public links are warning-toned: they are reachable from the internet.
		links := func(urls []string, width int, tone string) string {
			var b strings.Builder
			for i, raw := range urls {
				if i > 0 {
					b.WriteString("  ")
				}
				text := statusTUISafe(statusRuntimeURLLabel(raw))
				if styled && statusTUIURL(raw) != "" {
					text = statusTUILinkTone(text, raw, tone)
				}
				b.WriteString(text)
			}
			return b.String() + strings.Repeat(" ", max(0, width-statusRuntimeLinksWidth(urls)))
		}
		tail := links(g.tunnels, l.tunnel, statusToneLink)
		if l.public > 0 {
			tail += " " + links(g.public, l.public, statusToneWarning)
		}
		if notes := statusTUISafe(strings.Join(g.notes, " · ")); notes != "" {
			if styled {
				notes = statusTUIStyle(notes, statusToneDim)
			}
			tail += " " + notes
		}
		line := marks + " " + repo + " " + name + " " + stateCell + " " + local + " " + tail
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

// statusRuntimeMark is one letter for a task, toned by its state. A configured
// mark wins. Otherwise a service takes the first letter of its service name
// (w for web), a tunnel the first letter of its own name (t for tunnel), in
// upper case when it is public, and resource hooks are r.
func statusRuntimeMark(item wtc.RuntimeItem) (string, string) {
	first := func(name string) rune {
		for _, r := range strings.TrimPrefix(name[strings.LastIndex(name, "/")+1:], "@") {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				return unicode.ToLower(r)
			}
		}
		return '?'
	}
	letter := first(item.Group)
	if item.Kind == "endpoint" {
		letter = first(item.Path)
		if item.Public {
			letter = unicode.ToUpper(letter)
		}
	}
	if item.Mark != "" {
		letter = []rune(item.Mark)[0]
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
	if item.Public {
		parts[1] = "public " + item.Kind
	}
	for _, extra := range []string{item.Label, item.URL, item.Reason, item.Error} {
		if extra != "" {
			parts = append(parts, extra)
		}
	}
	return statusTUISafe(strings.Join(parts, " · "))
}
