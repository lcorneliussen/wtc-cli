package main

import (
	"github.com/charmbracelet/x/ansi"
	"github.com/lcorneliussen/wtc-cli/internal/wtc"
	"strings"
	"testing"
)

func TestStatusRuntimeRowsPreservePRClicksAndWidth(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	url := "https://github.com/example/widget/pull/7"
	snapshot := wtc.StatusSnapshot{Collection: "fixture", Repos: []wtc.StatusRepo{{Dir: "widget", Runtime: "2 ready", BranchDisplay: "feature", Tree: "clean"}},
		Runtime: []wtc.RuntimeItem{{Collection: "fixture", Repo: "widget", Dir: "widget", Path: "widget/web/server", Group: "widget/web", Kind: "service", State: "ready"}, {Collection: "fixture", Repo: "widget", Dir: "widget", Path: "widget/web/tunnel", Group: "widget/web", Kind: "endpoint", State: "ready", URL: "https://widget.example.test"}},
		PRs:     []wtc.StatusPRRow{{Repo: "widget", Number: "7", Title: "Change widget", URL: &url}}}
	for _, width := range []int{40, 80, 120} {
		model := statusTUIModel{width: width, height: 60, snapshot: snapshot}
		lines := model.contentLines()
		found := false
		for y, line := range lines {
			plain := ansi.Strip(line)
			if ansi.StringWidth(line) > width {
				t.Fatalf("%d: line exceeded width: %q", width, plain)
			}
			if strings.HasPrefix(strings.TrimSpace(plain), "#7") {
				found = true
				if got := model.buildClickTarget(1, y); got != url {
					t.Fatalf("runtime rows shifted PR click: %q", got)
				}
			}
			if strings.Contains(plain, "endpoint") && model.buildClickTarget(1, y) != "" {
				t.Fatal("runtime row opened a PR")
			}
		}
		if !found {
			t.Fatal("PR missing")
		}
		if width == 120 && !strings.Contains(ansi.Strip(model.View().Content), "2 ready") {
			t.Fatal("repo runtime summary missing")
		}
	}
}
