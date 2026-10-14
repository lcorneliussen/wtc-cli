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
		// One letter per task under LOCAL: w for the web service, / then t for its tunnel.
		header, row := ansi.Strip(lines[2]), ansi.Strip(lines[3])
		start := len([]rune(header[:max(0, strings.Index(header, "LOCAL"))]))
		if !strings.Contains(header, "LOCAL") || !strings.HasPrefix(string([]rune(row)[start:]), "w/t") {
			t.Fatalf("%d: marks missing under LOCAL: %q / %q", width, header, row)
		}
		if !strings.Contains(lines[3], "\x1b[32mw") {
			t.Fatalf("ready mark is not toned: %q", lines[3])
		}
		if item, ok := model.runtimeItemAt(start+2, 3); !ok || item.Path != "widget/web/tunnel" {
			t.Fatalf("wrong mark under pointer: %+v", item)
		}
		for _, x := range []int{start + 1, start + 3} {
			if _, ok := model.runtimeItemAt(x, 3); ok {
				t.Fatalf("separator or empty cell at %d resolved to a task", x)
			}
		}
		if got := model.buildClickTarget(start+2, 3); got != "https://widget.example.test" {
			t.Fatalf("tunnel mark did not open its URL: %q", got)
		}
		if got := model.buildClickTarget(start, 3); got != "" {
			t.Fatalf("service mark without a URL opened %q", got)
		}
		// The detail table has one line for the service and its tunnel.
		detail := -1
		for y, line := range lines {
			if plain := ansi.Strip(line); strings.HasPrefix(plain, "w/t widget") && strings.Contains(plain, " web ") && strings.Contains(plain, "ready") {
				detail = y
			}
			if plain := ansi.Strip(line); strings.Contains(plain, "web/tunnel") || strings.Contains(plain, "web/server") {
				t.Fatalf("service and tunnel are on separate lines: %q", plain)
			}
		}
		if detail < 0 || !strings.Contains(lines[detail], "\x1b]8;;https://widget.example.test") {
			t.Fatalf("%d: service line or its tunnel link missing: %q", width, lines)
		}
		if plain := ansi.Strip(lines[detail]); strings.Contains(plain, "https://") || !strings.Contains(plain, "widget.example.test") {
			t.Fatalf("URL not shortened to its host: %q", plain)
		}
		layout := statusTUIRuntimeLayout(snapshot, width)
		if got := model.buildClickTarget(layout.tunnelStart()+2, detail); got != "https://widget.example.test" {
			t.Fatalf("tunnel URL opened %q", got)
		}
		if got := model.buildClickTarget(layout.localStart(), detail); got != "" {
			t.Fatalf("empty local URL cell opened %q", got)
		}
		hover := statusRuntimeHover(snapshot.Runtime[1])
		model.hover = hover
		if view := ansi.Strip(model.View().Content); !strings.Contains(view, "widget/web/tunnel · endpoint · ready · https://widget.example.test") {
			t.Fatalf("hover text missing from footer: %q", hover)
		}
	}
}

func TestStatusRuntimeURLLabels(t *testing.T) {
	for raw, want := range map[string]string{
		"http://127.0.0.1:42431":                   ":42431",
		"http://localhost:3000/":                   ":3000",
		"https://workstation.example.ts.net:42433": "tailscale",
		"https://calm-river.trycloudflare.com":     "cloudflare",
		"https://1a2b.ngrok-free.app":              "ngrok",
		"https://abc-3000.euw.devtunnels.ms":       "devtunnel",
		"https://preview.example.test/path":        "preview.example.test",
	} {
		if got := statusRuntimeURLLabel(raw); got != want {
			t.Errorf("%s: got %q, want %q", raw, got, want)
		}
	}
}

func TestStatusRuntimeSeparatesProtectedAndPublicTunnels(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	item := func(name, kind, url string, public bool) wtc.RuntimeItem {
		return wtc.RuntimeItem{Collection: "fixture", Repo: "widget", Dir: "widget", Path: "widget/web/" + name, Group: "widget/web", Kind: kind, State: "ready", URL: url, Public: public}
	}
	snapshot := wtc.StatusSnapshot{Collection: "fixture", Repos: []wtc.StatusRepo{{Collection: "fixture", Repo: "widget", Dir: "widget", BranchDisplay: "feature", Tree: "clean"}},
		Runtime: []wtc.RuntimeItem{item("server", "service", "http://127.0.0.1:3000", false),
			item("tailnet", "endpoint", "https://workstation.example.ts.net:8443", false),
			item("preview", "endpoint", "https://calm-river.trycloudflare.com", true)}}
	model := statusTUIModel{width: 120, height: 40, snapshot: snapshot}
	lines := model.contentLines()
	header, row, y := "", "", -1
	for i, line := range lines {
		if plain := ansi.Strip(line); strings.HasPrefix(plain, "w/t/P widget") {
			header, row, y = ansi.Strip(lines[i-1]), plain, i
		}
	}
	if y < 0 {
		t.Fatalf("service line missing: %q", lines)
	}
	column := func(name string) int { return len([]rune(header[:strings.Index(header, name)])) }
	cell := func(name string) string { return strings.Fields(string([]rune(row)[column(name):]))[0] }
	if cell("URL") != ":3000" || cell("TUNNEL") != "tailscale" || cell("PUBLIC") != "cloudflare" {
		t.Fatalf("columns wrong: %q / %q", header, row)
	}
	for name, want := range map[string]string{"URL": "http://127.0.0.1:3000", "TUNNEL": "https://workstation.example.ts.net:8443", "PUBLIC": "https://calm-river.trycloudflare.com"} {
		if got := model.buildClickTarget(column(name), y); got != want {
			t.Errorf("%s opened %q", name, got)
		}
	}
	if !strings.Contains(lines[y], "\x1b[33mcloudflare") {
		t.Errorf("public link is not warning-toned: %q", lines[y])
	}
	if hover := statusRuntimeHover(snapshot.Runtime[2]); !strings.Contains(hover, "public endpoint") {
		t.Errorf("hover does not say public: %q", hover)
	}
	// Without a public endpoint the column is absent.
	snapshot.Runtime = snapshot.Runtime[:2]
	if plain := ansi.Strip(strings.Join(statusTUIModel{width: 120, height: 40, snapshot: snapshot}.contentLines(), "\n")); strings.Contains(plain, "PUBLIC") {
		t.Fatal("PUBLIC column shown without a public endpoint")
	}
}

func TestStatusRuntimeMarkDefaultsAndOverride(t *testing.T) {
	for want, item := range map[string]wtc.RuntimeItem{
		"w": {Path: "app/web/server", Group: "app/web", Kind: "service"},
		"d": {Path: "app/database", Group: "app/database", Kind: "service"},
		"t": {Path: "app/web/tunnel", Group: "app/web", Kind: "endpoint"},
		"T": {Path: "app/web/tunnel", Group: "app/web", Kind: "endpoint", Public: true},
		"r": {Path: "app/@resources", Group: "app/resources", Kind: "resources"},
		"x": {Path: "app/web/server", Group: "app/web", Kind: "service", Mark: "x"},
	} {
		if got, _ := statusRuntimeMark(item); got != want {
			t.Errorf("%+v: got %q, want %q", item, got, want)
		}
	}
}

func TestStatusRuntimeCellSeparatesServices(t *testing.T) {
	items := []wtc.RuntimeItem{
		{Collection: "c", Group: "widget/web", Path: "widget/web/server", Kind: "service", State: "ready"},
		{Collection: "c", Group: "widget/resources", Path: "widget/resources", Kind: "resources", State: "provision-hook-completed"},
		{Collection: "c", Group: "widget/web", Path: "widget/web/tunnel", Kind: "endpoint", State: "ready"},
	}
	if got := statusRuntimeCell(items, 6, false); got != "w/t r " {
		t.Fatalf("cell = %q, want a service joined to its tunnel and apart from resources", got)
	}
}
