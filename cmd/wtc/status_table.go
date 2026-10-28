package main

import (
	"fmt"
	"strings"

	"github.com/lcorneliussen/wtc-cli/internal/branding"
	"github.com/lcorneliussen/wtc-cli/internal/wtc"
	"github.com/mattn/go-runewidth"
)

func statusTable(snapshot wtc.StatusSnapshot, reposOnly bool, width int) string {
	name := snapshot.Collection
	if name == "" {
		name = "(all)"
	}
	header := fmt.Sprintf(branding.Text("{{command}} status · %s · %s"), name, snapshot.GeneratedAt)
	if remaining := width - runewidth.StringWidth(header); remaining > 0 {
		header += statusTUIFit(" · "+wtc.StatusVersionLabel(snapshot.CLIVersion), remaining)
	}
	lines := []string{statusTUIFit(header, width), ""}
	lines = append(lines, statusTUIRepoLines(snapshot, width, false)...)
	lines = append(lines, statusTUIRuntimeLines(snapshot, width, false)...)
	if !reposOnly {
		lines = append(lines, statusTUIPRLines(snapshot, width, false, false)...)
	}
	lines = append(lines, statusTUIRetiredLines(snapshot, width, false)...)
	lines = append(lines, statusNextLines(snapshot, width, false, -1)...)
	if snapshot.StaleCount > 0 {
		lines = append(lines, "", statusTUIFit(fmt.Sprintf("%d worktree(s) behind the development tip", snapshot.StaleCount), width))
	}
	return strings.Join(lines, "\n") + "\n"
}
