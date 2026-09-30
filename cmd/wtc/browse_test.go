package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lcorneliussen/wtc-cli/internal/wtc"
)

func TestBrowseSocketMatchesShellContract(t *testing.T) {
	if got := browseChecksum("abc"); got != 1219131554 {
		t.Fatalf("POSIX cksum differs: %d", got)
	}
	if got := browseSocket("/work/example-wtc", "topic"); got != "/tmp/wtc-browse-example-wtc-topic.nvim" {
		t.Fatalf("short socket: %s", got)
	}
	name := strings.Repeat("long-collection-", 7)
	got := browseSocket("/work/example-wtc", name)
	if got != "/tmp/wtc-browse-"+("example-wtc-" + name)[:60]+"-1785337918.nvim" {
		t.Fatalf("long socket differs from shell spelling: %s", got)
	}
}

func TestBrowseHereLoadsBundledViewInCollection(t *testing.T) {
	root := t.TempDir()
	collection := filepath.Join(root, "topic")
	if err := os.Mkdir(collection, 0755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0755); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(root, "nvim.log")
	program := "#!/bin/sh\nif [ \"$1\" = --server ]; then exit 1; fi\nprintf 'cwd=%s\\n' \"$PWD\" > \"$BROWSE_TEST_LOG\"\nprintf 'arg=%s\\n' \"$@\" >> \"$BROWSE_TEST_LOG\"\n"
	if err := os.WriteFile(filepath.Join(bin, "nvim"), []byte(program), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("BROWSE_TEST_LOG", log)
	c := &wtc.Context{Collection: collection, Workspace: root}
	if err := browseHere(c); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, want := range []string{"cwd=" + collection, "arg=--listen", "arg=" + browseSocket(root, "topic"), "vim.g.wtc_browse_root", "dofile("} {
		if !strings.Contains(text, want) {
			t.Fatalf("Neovim launch missing %q: %s", want, text)
		}
	}
}

func TestBrowseViewAllowsHarnessOverlay(t *testing.T) {
	root := t.TempDir()
	harness := filepath.Join(root, "harness")
	path := filepath.Join(harness, "overlays", "browse", "wtc-browse.lua")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("vim.g.custom_browse = true\n"), 0644); err != nil {
		t.Fatal(err)
	}
	body, err := browseView(&wtc.Context{Harness: harness})
	if err != nil || string(body) != "vim.g.custom_browse = true\n" {
		t.Fatalf("harness browse overlay ignored: %q, %v", body, err)
	}
}

func TestBrowseAgentUsesTargetPaneWithoutTakingOverCaller(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0755); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(root, "herdr.log")
	program := `#!/bin/sh
printf '%s\n' "$*" >> "$BROWSE_TEST_LOG"
case "$*" in
  *'agent list') echo '{"result":{"agents":[{"pane_id":"caller-agent"}]}}' ;;
  *'workspace list') echo '{"result":{"workspaces":[{"label":"target","workspace_id":"target-ws"}]}}' ;;
  *'pane list --workspace target-ws') echo '{"result":{"panes":[{"label":"agent","pane_id":"target-agent","tab_id":"t1"},{"label":"browse","pane_id":"target-browse","tab_id":"t1"}]}}' ;;
  *'pane process-info --pane target-browse') echo '{"result":{"process_info":{"foreground_process_group_id":4,"foreground_processes":[{"pid":4,"name":"zsh","argv":["-zsh"]}]}}}' ;;
  *'pane run target-browse '*) echo '{"result":{}}' ;;
  *) exit 2 ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "herdr"), []byte(program), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("BROWSE_TEST_LOG", log)
	t.Setenv("HERDR_ENV", "1")
	t.Setenv("HERDR_SESSION", "caller-session")
	t.Setenv("HERDR_PANE_ID", "caller-agent")
	if agent, err := browseCallerIsAgent(); err != nil || !agent {
		t.Fatalf("registered herdr caller was not recognized as an agent: %v", err)
	}
	c := &wtc.Context{Collection: filepath.Join(root, "target"), Harness: filepath.Join(root, "target", "harness"), Workspace: root}
	if err := browseInPane(c, "test-session"); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, "pane run target-browse '"+filepath.Join(c.Harness, "tools", "wtc-browse.sh")+"' --here") || strings.Contains(text, "pane run target-agent") {
		t.Fatalf("browse was not routed to the target pane: %s", text)
	}
}

func TestBrowsePRTabUsesHerdrRootPaneResponse(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0755); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(root, "herdr.log")
	program := `#!/bin/sh
printf '%s\n' "$*" >> "$BROWSE_TEST_LOG"
case "$*" in
  *'tab list --workspace target-ws') echo '{"result":{"tabs":[]}}' ;;
  *'tab create --workspace target-ws '*) echo '{"result":{"root_pane":{"pane_id":"pr-pane"},"tab":{"tab_id":"pr-tab"}}}' ;;
  *'pane rename pr-pane pr') echo '{"result":{}}' ;;
  *'pane list --workspace target-ws') echo '{"result":{"panes":[{"label":"pr","pane_id":"pr-pane","tab_id":"pr-tab"}]}}' ;;
  *'pane process-info --pane pr-pane') echo '{"result":{"process_info":{"foreground_process_group_id":8,"foreground_processes":[{"pid":8,"name":"zsh","argv":["-zsh"]}]}}}' ;;
  *'pane run pr-pane gh dash') echo '{"result":{}}' ;;
  *) exit 2 ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "herdr"), []byte(program), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("BROWSE_TEST_LOG", log)
	browseEnsurePRTab(&wtc.Context{Collection: filepath.Join(root, "target")}, "test-session", "target-ws")
	body, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, "pane rename pr-pane pr") || !strings.Contains(text, "pane run pr-pane gh dash") {
		t.Fatalf("herdr's root pane was not used for the PR tab: %s", text)
	}
}
