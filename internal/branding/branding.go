// Package branding identifies the invoked command without renaming on-disk or
// protocol identifiers during the compatibility window.
package branding

import (
	"os"
	"path/filepath"
	"strings"
)

// Name follows argv[0], including symlinks and dev-build handovers. Unknown
// executable names retain the released command's spelling.
func Name() string {
	return name(os.Args[0])
}

func name(argv0 string) string {
	switch filepath.Base(argv0) {
	case "taba", "taba.exe":
		return "taba"
	default:
		return "wtc"
	}
}

// Text expands an explicit command placeholder in authored human-facing text.
// Pass templates before interpolation so user values, paths, plugin IDs and
// machine payloads are never rewritten as display text.
func Text(template string) string {
	return strings.ReplaceAll(template, "{{command}}", Name())
}
