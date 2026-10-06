package main

import (
	"strings"
	"unicode"

	"github.com/lcorneliussen/wtc-cli/internal/wtc"
)

func statusTUIRuntimeLines(snapshot wtc.StatusSnapshot, width int, styled bool) []string {
	if len(snapshot.Runtime) == 0 {
		return nil
	}
	lines := []string{""}
	for _, line := range strings.Split(strings.TrimSuffix(wtc.RuntimeText(snapshot.Runtime), "\n"), "\n") {
		line = statusTUIFit(statusTUISafe(line), width)
		if styled {
			line = statusTUIStyle(line, statusToneDim)
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
