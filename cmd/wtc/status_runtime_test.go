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
	snapshot := wtc.StatusSnapshot{Collection: "fixture", Repos: []wtc.StatusRepo{{Collection: "fixture", Repo: "widget", Dir: "widget", Runtime: "2 ready", BranchDisplay: "feature", Tree: "clean"}},
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
		if width < 60 {
			continue
		}
		// One letter per task under LOCAL; the tunnel is upper-case and opens its URL.
		header, row := ansi.Strip(lines[2]), ansi.Strip(lines[3])
		start := len([]rune(header[:max(0, strings.Index(header, "LOCAL"))]))
		if !strings.Contains(header, "LOCAL") || !strings.HasPrefix(string([]rune(row)[start:]), "sT") {
			t.Fatalf("%d: marks missing under LOCAL: %q / %q", width, header, row)
		}
		if !strings.Contains(lines[3], "\x1b[32ms") {
			t.Fatalf("ready mark is not toned: %q", lines[3])
		}
		if item, ok := model.runtimeItemAt(start+1, 3); !ok || item.Path != "widget/web/tunnel" {
			t.Fatalf("wrong mark under pointer: %+v", item)
		}
		if _, ok := model.runtimeItemAt(start+2, 3); ok {
			t.Fatal("empty cell resolved to a task")
		}
		if got := model.buildClickTarget(start+1, 3); got != "https://widget.example.test" {
			t.Fatalf("tunnel mark did not open its URL: %q", got)
		}
		if got := model.buildClickTarget(start, 3); got != "" {
			t.Fatalf("service mark without a URL opened %q", got)
		}
		hover := statusRuntimeHover(snapshot.Runtime[1])
		model.hover = hover
		if view := ansi.Strip(model.View().Content); !strings.Contains(view, "widget/web/tunnel · endpoint · ready · https://widget.example.test") {
			t.Fatalf("hover text missing from footer: %q", hover)
		}
	}
}
