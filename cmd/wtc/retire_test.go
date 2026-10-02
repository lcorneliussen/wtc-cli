package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lcorneliussen/wtc-cli/internal/wtc"
)

func retireHandoffFixture(t *testing.T, sourceID string) (*wtc.Context, string) {
	t.Helper()
	root := t.TempDir()
	collection := filepath.Join(root, "finished")
	harness := filepath.Join(collection, "harness")
	if err := os.MkdirAll(harness, 0755); err != nil {
		t.Fatal(err)
	}
	registry := "repos:\n  - name: widget\n    remote: https://github.com/example/widget.git\n    default_ref: main\n"
	if err := os.WriteFile(filepath.Join(harness, ".harness-repos.yml"), []byte(registry), 0644); err != nil {
		t.Fatal(err)
	}
	c, err := wtc.OpenCollection(collection)
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(root, "herdr.log")
	fixture := `#!/usr/bin/env python3
import json, os, sys
args = sys.argv[3:]
with open(os.environ["RETIRE_TEST_LOG"], "a") as out: out.write(" ".join(args) + "\n")
def emit(value): print(json.dumps({"result": value}))
if args == ["workspace", "list"]:
    emit({"workspaces": [{"label":"finished", "workspace_id":os.environ["RETIRE_TEST_SOURCE_ID"]}] + ([{"label":"--cleanup--", "workspace_id":"cleanup-id"}] if os.environ.get("RETIRE_TEST_REUSE") else [])})
elif args[:2] == ["pane", "list"]:
    panes = [{"pane_id":"source:p1", "label":"agent", "agent":"codex", "agent_status":"working"}]
    if os.environ.get("RETIRE_TEST_EXTRA_AGENT"): panes.append({"pane_id":"source:p2", "label":"agent", "agent":"claude", "agent_status":"working"})
    emit({"panes":panes})
elif args[:2] == ["workspace", "create"]:
    emit({"workspace":{"workspace_id":"cleanup-id"}, "root_pane":{"pane_id":"cleanup:p1"}})
elif args[:2] == ["tab", "create"]:
    emit({"root_pane":{"pane_id":"cleanup:p2"}})
elif args[:2] == ["pane", "run"]:
    emit({})
else:
    sys.exit(2)
`
	if err := os.WriteFile(filepath.Join(bin, "herdr"), []byte(fixture), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("RETIRE_TEST_LOG", log)
	t.Setenv("RETIRE_TEST_SOURCE_ID", sourceID)
	t.Setenv("HERDR_ENV", "1")
	t.Setenv("HERDR_SESSION", "fixture")
	t.Setenv("HERDR_WORKSPACE_ID", "source-id")
	t.Setenv("HERDR_PANE_ID", "source:p1")
	return c, log
}

func TestSelfRetireHandsOffToCleanupWorkspace(t *testing.T) {
	c, log := retireHandoffFixture(t, "source-id")
	if err := handoffSelfRetire(c, "finished", false, true); err != nil {
		t.Fatal(err)
	}
	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	text := string(calls)
	if !strings.Contains(text, "workspace create --label --cleanup-- --cwd "+c.Workspace+" --no-focus") || !strings.Contains(text, "pane run cleanup:p1 ") || !strings.Contains(text, " retire-worker 'finished'") {
		t.Fatalf("cleanup handoff incomplete: %s", text)
	}
	if strings.Contains(text, "workspace close") {
		t.Fatalf("source workspace closed during handoff: %s", text)
	}
	if _, err := os.Stat(c.Collection); err != nil {
		t.Fatalf("handoff deleted the collection before worker ran: %v", err)
	}
}

func TestSelfRetireReusesCleanupWorkspaceAndRejectsWrongSource(t *testing.T) {
	c, log := retireHandoffFixture(t, "source-id")
	t.Setenv("RETIRE_TEST_REUSE", "1")
	if err := handoffSelfRetire(c, "finished", false, true); err != nil {
		t.Fatal(err)
	}
	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(calls), "tab create --workspace cleanup-id --label finished") || !strings.Contains(string(calls), "pane run cleanup:p2 ") {
		t.Fatalf("existing cleanup workspace was not reused: %s", calls)
	}
	t.Setenv("RETIRE_TEST_SOURCE_ID", "some-other-workspace")
	if err := handoffSelfRetire(c, "finished", false, true); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("wrong source workspace was accepted: %v", err)
	}
	updated, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(updated), "pane run ") != 1 {
		t.Fatalf("wrong source scheduled another deletion: %s", updated)
	}
}

func TestSelfRetireRefusesToCloseAnotherAgent(t *testing.T) {
	c, log := retireHandoffFixture(t, "source-id")
	t.Setenv("RETIRE_TEST_EXTRA_AGENT", "1")
	if err := handoffSelfRetire(c, "finished", false, true); err == nil || !strings.Contains(err.Error(), "another agent") {
		t.Fatalf("other agent was not protected: %v", err)
	}
	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(calls), "workspace create") || strings.Contains(string(calls), "pane run") {
		t.Fatalf("cleanup was scheduled despite another agent: %s", calls)
	}
}
