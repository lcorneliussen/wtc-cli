package main

import (
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/lcorneliussen/wtc-cli/internal/wtc"
)

const (
	statusToneHeading = "1;38;5;180"
	statusToneLabel   = "1;38;5;252"
	statusToneDim     = "2"
	statusToneLink    = "4;38;5;81"
	statusToneSuccess = "38;5;114"
	statusToneWarning = "38;5;214"
	statusToneFailure = "38;5;203"
)

var statusTUILinkPattern = regexp.MustCompile(`(?i)\x1b]8;;https?://[^\x07\x1b]*(?:\x07|\x1b\\)(.*?)\x1b]8;;(?:\x07|\x1b\\)`)

func statusTUIHasVisibleLink(line string) bool {
	for _, match := range statusTUILinkPattern.FindAllStringSubmatch(line, -1) {
		if strings.Trim(ansi.Strip(match[1]), " \t…") != "" {
			return true
		}
	}
	return false
}

func statusTUIStyle(value, tone string) string {
	if value == "" || os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return value
	}
	return "\x1b[" + tone + "m" + value + "\x1b[0m"
}

func statusTUISafe(value string) string {
	return strings.Map(func(r rune) rune {
		if r < ' ' || r == 0x7f || r >= 0x80 && r <= 0x9f {
			return ' '
		}
		return r
	}, ansi.Strip(value))
}

func statusTUIURL(raw string) string {
	if raw == "" || strings.IndexFunc(raw, func(r rune) bool { return r < ' ' || r == 0x7f || r >= 0x80 && r <= 0x9f }) >= 0 {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
		return ""
	}
	return raw
}

func statusTUILink(label, rawURL string) string {
	target := statusTUIURL(rawURL)
	if target == "" {
		return label
	}
	return ansi.SetHyperlink(target) + statusTUIStyle(label, statusToneLink) + ansi.ResetHyperlink()
}

func statusTUIFitANSI(value string, width int) string {
	if width <= 0 {
		return ""
	}
	visible := ansi.StringWidth(value)
	if visible <= width {
		return value + strings.Repeat(" ", width-visible)
	}
	if width == 1 {
		return "…"
	}
	return ansi.Truncate(value, width, "…") + ansi.ResetHyperlink() + "\x1b[0m"
}

func statusTUILocalCell(cell string, row wtc.StatusRepo) string {
	switch {
	case row.Behind > 0 || row.Tree != "" && row.Tree != "clean":
		return statusTUIStyle(cell, statusToneWarning)
	case row.Ahead > 0:
		return statusTUIStyle(cell, statusToneSuccess)
	default:
		return statusTUIStyle(cell, statusToneDim)
	}
}

func statusTUIBuildLink(cell string, build *wtc.StatusBuild) string {
	if build == nil {
		return statusTUIStyle(cell, statusToneDim)
	}
	label := strings.TrimRight(cell, " ")
	padding := strings.TrimPrefix(cell, label)
	if build.URL != nil && statusTUIURL(*build.URL) != "" {
		return statusTUILink(label, *build.URL) + padding
	}
	if build.Checks != nil {
		switch *build.Checks {
		case "SUCCESS":
			return statusTUIStyle(label, statusToneSuccess) + padding
		case "FAILURE", "ERROR":
			return statusTUIStyle(label, statusToneFailure) + padding
		case "PENDING", "EXPECTED":
			return statusTUIStyle(label, statusToneWarning) + padding
		}
	}
	return statusTUIStyle(label, statusToneDim) + padding
}
