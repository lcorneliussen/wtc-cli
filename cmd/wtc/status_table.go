package main

import (
	"fmt"
	"strings"

	"github.com/lcorneliussen/wtc-cli/internal/wtc"
)

func statusTable(snapshot wtc.StatusSnapshot, reposOnly bool, width int) string {
	name := snapshot.Collection
	if name == "" {
		name = "(all)"
	}
	lines := []string{statusTUIFit(fmt.Sprintf("wtc status · %s · %s", name, snapshot.GeneratedAt), width), ""}
	lines = append(lines, statusTUIRepoLines(snapshot, width, false)...)
	lines = append(lines, statusTUIRuntimeLines(snapshot, width, false)...)
	if !reposOnly {
		lines = append(lines, statusTUIPRLines(snapshot, width, false, false)...)
	}
	if snapshot.StaleCount > 0 {
		lines = append(lines, "", statusTUIFit(fmt.Sprintf("%d worktree(s) behind the development tip", snapshot.StaleCount), width))
	}
	return strings.Join(lines, "\n") + "\n"
}
