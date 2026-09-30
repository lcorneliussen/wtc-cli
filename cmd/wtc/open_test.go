package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lcorneliussen/wtc-cli/internal/wtc"
)

func TestOpenCreatesWideWorkspaceWithoutStartingDisabledProcesses(t *testing.T) {
	root := t.TempDir()
	collection := filepath.Join(root, "sample")
	harness := filepath.Join(collection, "harness")
	if err := os.MkdirAll(harness, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(harness, ".harness-repos.yml"), []byte("repos:\n  - name: example\n    remote: https://github.com/example/example.git\n    default_ref: origin/main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(collection, ".env.collection"), []byte("# generated\nWTC_COLLECTION='sample'\n"), 0644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	fixture := `#!/usr/bin/env python3
import json, os, sys
state_path = os.environ["OPEN_TEST_STATE"]
calls_path = os.environ["OPEN_TEST_CALLS"]
args = sys.argv[3:]
with open(calls_path, "a") as f: f.write(" ".join(args) + "\n")
try:
    state = json.load(open(state_path))
except FileNotFoundError:
    state = {"created": False, "panes": []}
def save():
    with open(state_path, "w") as f: json.dump(state, f)
def emit(result): print(json.dumps({"result": result}))
if args == ["workspace", "list"]:
    emit({"workspaces": [{"label": "sample", "workspace_id": "ws1"}] if state["created"] else []})
elif args[:2] == ["workspace", "create"]:
    state["created"] = True
    state["panes"] = [{"pane_id":"p1","tab_id":"t1","label":"1"}]
    state["tabs"] = [{"tab_id":"t1","label":"1"}]
    save(); emit({"workspace_id":"ws1", "pane_id":"p1"})
elif args[:2] == ["pane", "list"]:
    emit({"panes": state["panes"]})
elif args[:2] == ["tab", "list"]:
    emit({"tabs":state["tabs"]})
elif args[:2] == ["tab", "rename"]:
    for tab in state["tabs"]:
        if tab["tab_id"] == args[2]: tab["label"] = args[3]
    save(); emit({})
elif args[:2] == ["tab", "create"]:
    tab_id = "t" + str(len(state["tabs"])+1)
    new_id = "p" + str(len(state["panes"])+1)
    state["tabs"].append({"tab_id":tab_id,"label":args[args.index("--label")+1]})
    state["panes"].append({"pane_id":new_id,"tab_id":tab_id,"label":new_id})
    save(); emit({"pane_id":new_id})
elif args[:2] == ["pane", "split"]:
    new_id = "p" + str(len(state["panes"])+1)
    parent = next(p for p in state["panes"] if p["pane_id"] == args[2])
    state["panes"].append({"pane_id":new_id,"tab_id":parent["tab_id"],"label":new_id})
    save(); emit({"pane_id":new_id})
elif args[:2] == ["pane", "get"]:
    emit({"pane":next(p for p in state["panes"] if p["pane_id"] == args[2])})
elif args[:2] == ["pane", "rename"]:
    for pane in state["panes"]:
        if pane["pane_id"] == args[2]: pane["label"] = args[3]
    save(); emit({})
elif args[:2] == ["pane", "move"]:
    pane = next(p for p in state["panes"] if p["pane_id"] == args[2])
    if "--new-tab" in args:
        tab_id = "t" + str(len(state["tabs"])+1)
        state["tabs"].append({"tab_id":tab_id,"label":args[args.index("--label")+1]})
        pane["tab_id"] = tab_id
    else:
        pane["tab_id"] = args[args.index("--tab")+1]
    save(); emit({})
elif args[:2] == ["pane", "close"]:
    state["panes"] = [p for p in state["panes"] if p["pane_id"] != args[2]]
    save(); emit({})
elif args[:2] == ["pane", "process-info"]:
    emit({"process_info":{"foreground_process_group_id":1,"foreground_processes":[{"pid":1,"argv":["-zsh"]}]}})
elif args[:2] == ["api", "snapshot"]:
    emit({"snapshot":{"layouts":[{"area":{"width":180},"panes":[
        {"pane_id":pane["pane_id"],"rect":{"x":0 if pane["label"] in ("agent","shell") else 80}}
        for pane in state["panes"]]}]}})
else:
    print("unexpected command: " + " ".join(args), file=sys.stderr)
    sys.exit(2)
`
	if err := os.WriteFile(filepath.Join(bin, "herdr"), []byte(fixture), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("OPEN_TEST_STATE", filepath.Join(root, "state.json"))
	log := filepath.Join(root, "calls.log")
	t.Setenv("OPEN_TEST_CALLS", log)
	c, err := wtc.OpenCollection(collection)
	if err != nil {
		t.Fatal(err)
	}
	opt := openOptions{Session: "test", NoAgent: true, NoBrowse: true, NoStatus: true}
	preview := openCollection(c, "sample", openOptions{Session: "test", DryRun: true}, "wide", true, true)
	if preview.Error != "" || !strings.Contains(strings.Join(preview.Actions, " "), "would create") {
		t.Fatalf("wrong preview: %+v", preview)
	}
	if _, err := os.Stat(os.Getenv("OPEN_TEST_STATE")); !os.IsNotExist(err) {
		t.Fatalf("dry run created workspace: %v", err)
	}
	opened := openCollection(c, "sample", opt, "wide", true, true)
	if opened.Error != "" || opened.Workspace != "ws1" || !strings.Contains(strings.Join(opened.Actions, " "), "workspace created") {
		t.Fatalf("open failed: %+v", opened)
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	calls := string(data)
	if strings.Count(calls, "pane split ") != 3 || !strings.Contains(calls, "workspace create --cwd "+collection) || !strings.Contains(calls, "--env WTC_COLLECTION='sample'") {
		t.Fatalf("incorrect workspace setup:\n%s", calls)
	}
	if strings.Contains(calls, "agent start") || strings.Contains(calls, "pane run") {
		t.Fatalf("disabled processes were started:\n%s", calls)
	}
	again := openCollection(c, "sample", opt, "wide", true, true)
	if again.Error != "" || !strings.Contains(strings.Join(again.Actions, " "), "already open") && !strings.Contains(strings.Join(again.Actions, " "), "skipped") {
		t.Fatalf("rerun did not reuse the workspace: %+v", again)
	}
	data, err = os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), "workspace create ") != 1 || strings.Count(string(data), "pane split ") != 3 {
		t.Fatalf("rerun changed a complete layout:\n%s", data)
	}
	narrowOpt := opt
	narrowOpt.LayoutSet = true
	switched := openCollection(c, "sample", narrowOpt, "narrow", true, true)
	if switched.Error != "" || !strings.Contains(strings.Join(switched.Actions, " "), "layout switched") {
		t.Fatalf("wide-to-narrow switch failed: %+v", switched)
	}
	state, err := os.ReadFile(os.Getenv("OPEN_TEST_STATE"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(state), `"pane_id": "p1", "tab_id": "t1", "label": "agent"`) || !strings.Contains(string(state), `"pane_id": "p2", "tab_id": "t2", "label": "browse"`) {
		t.Fatalf("switch did not preserve the agent and move browse: %s", state)
	}
	if err := os.Remove(os.Getenv("OPEN_TEST_STATE")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(log); err != nil {
		t.Fatal(err)
	}
	narrow := openCollection(c, "sample", opt, "narrow", true, true)
	if narrow.Error != "" || narrow.Layout != "narrow" {
		t.Fatalf("narrow creation failed: %+v", narrow)
	}
	state, err = os.ReadFile(os.Getenv("OPEN_TEST_STATE"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(state), `"label": "tools"`) || !strings.Contains(string(state), `"pane_id": "p1", "tab_id": "t1", "label": "agent"`) || !strings.Contains(string(state), `"pane_id": "p4", "tab_id": "t2", "label": "shell"`) {
		t.Fatalf("narrow layout is incomplete: %s", state)
	}
}

func TestOpenAgentNamesFitHerdrLimit(t *testing.T) {
	for _, session := range []string{"wtc", "123", strings.Repeat("s", 30), strings.Repeat("s", 40)} {
		name := openAgentName(session, "123-long-collection")
		if len(name) > 32 || name[0] < 'a' || name[0] > 'z' {
			t.Fatalf("invalid agent name %q", name)
		}
	}
}

func TestOpenReadEnvironmentPreservesAssignments(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env.collection")
	content := "# generated\nWTC_COLLECTION='sample'\nTOKEN=abc=def\nSPACED=\"two words\"\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := openReadEnvironment(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"WTC_COLLECTION='sample'", "TOKEN=abc=def", `SPACED="two words"`}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("assignments changed: got %q, want %q", got, want)
	}
}

func TestOpenAgentStartsWithFirstPromptRecovery(t *testing.T) {
	root := t.TempDir()
	collection := filepath.Join(root, "sample")
	if err := os.MkdirAll(collection, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(collection, "HANDOFF.md"), []byte("start here\n"), 0644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(root, "calls.log")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$OPEN_TEST_CALLS"
case "$*" in
  *'agent prompt '*) echo agent_prompt_stalled >&2; exit 1 ;;
esac
printf '%s\n' '{"result":{}}'
`
	if err := os.WriteFile(filepath.Join(bin, "herdr"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("OPEN_TEST_CALLS", log)
	c := &wtc.Context{Collection: collection, ConfigRoot: root}
	if err := openStartAgent(c, openOptions{Session: "test"}, "pane-1"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	calls := string(data)
	for _, want := range []string{
		"agent start test--sample --kind claude --pane pane-1 -- --dangerously-skip-permissions --remote-control test--sample",
		"agent prompt pane-1 /wtc-start --wait --until working --timeout 20000",
		"agent send-keys pane-1 enter",
		"agent wait pane-1 --until working --timeout 10000",
	} {
		if !strings.Contains(calls, want) {
			t.Fatalf("missing %q in:\n%s", want, calls)
		}
	}
}
