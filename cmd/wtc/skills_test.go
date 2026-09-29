package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSkillsRenderAllContinuesPastInvalidCollection(t *testing.T) {
	workspace := t.TempDir()
	good := filepath.Join(workspace, "good")
	bad := filepath.Join(workspace, "bad")
	for _, collection := range []string{good, bad} {
		if err := os.MkdirAll(filepath.Join(collection, "harness"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(good, "harness", ".harness-repos.yml"), []byte("repos:\n  - name: widget\n    remote: https://example.invalid/widget.git\n    default_ref: origin/main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, "harness", ".harness-repos.yml"), []byte("repos: [invalid\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "run", ".", "skills", "render", "--collection", good, "--all", "--dry-run", "--json")
	out, err := cmd.Output()
	if err == nil {
		t.Fatal("--all hid a collection failure")
	}
	var result struct {
		OK   bool              `json:"ok"`
		Data []json.RawMessage `json:"data"`
		Meta struct {
			Failures []string `json:"failures"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("invalid JSON %q: %v", out, err)
	}
	if result.OK || len(result.Data) != 1 || len(result.Meta.Failures) != 1 {
		t.Fatalf("incomplete sweep result: %s", out)
	}
	if _, err := os.Lstat(filepath.Join(good, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatal("dry run mutated a valid collection")
	}
}

func TestSkillsRenderPreHookCanVetoWrites(t *testing.T) {
	collection := filepath.Join(t.TempDir(), "demo")
	harness := filepath.Join(collection, "harness")
	if err := os.MkdirAll(filepath.Join(harness, "hooks", "wtc"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(harness, ".harness-repos.yml"), []byte("repos:\n  - name: widget\n    remote: https://example.invalid/widget.git\n    default_ref: origin/main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(harness, "hooks", "wtc", "skills.render.pre.sh"), []byte("#!/bin/sh\nexit 7\n"), 0755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "run", ".", "skills", "render", "--collection", collection)
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("pre-hook did not veto render: %s", out)
	}
	if _, err := os.Lstat(filepath.Join(collection, ".agents")); !os.IsNotExist(err) {
		t.Fatal("render wrote agent links after pre-hook veto")
	}
}
