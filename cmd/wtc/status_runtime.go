package main

import (
	"strings"

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
