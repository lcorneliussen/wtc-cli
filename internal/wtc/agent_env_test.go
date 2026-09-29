package wtc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentToolchainPathOrdersSiblingPinsAndKeepsCache(t *testing.T) {
	c := fixture(t)
	root := t.TempDir()
	productBin := filepath.Join(root, "product-bin")
	harnessBin := filepath.Join(root, "harness-bin")
	collectionBin := filepath.Join(root, "collection-bin")
	for _, dir := range []string{productBin, harnessBin, collectionBin, filepath.Join(root, "bin")} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	for _, dir := range []string{filepath.Join(c.Collection, "widget"), c.Harness, c.Collection} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "mise.toml"), []byte("[tools]\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	shim := "#!/bin/sh\ncase \"$1\" in trust) exit 0;; esac\n" +
		"case \"$2\" in\n" +
		"*/widget) printf '%s\\n' '" + productBin + "';;\n" +
		"*/harness) printf '%s\\n' '" + productBin + "' '" + harnessBin + "';;\n" +
		"*) printf '%s\\n' '" + collectionBin + "';;\n" +
		"esac\n"
	if err := os.WriteFile(filepath.Join(root, "bin", "mise"), []byte(shim), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Join(root, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	got, err := c.AgentToolchainPath(true)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{productBin, harnessBin, collectionBin}, string(os.PathListSeparator))
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
	if path := readToolchainCache(filepath.Join(c.Collection, ".env.toolchain")); path != want {
		t.Fatalf("cache = %q", path)
	}
	t.Setenv("PATH", root) // no mise; a valid cache remains useful
	again, err := c.AgentToolchainPath(false)
	if err != nil || again != want {
		t.Fatalf("cached path = %q, %v", again, err)
	}
	if err := os.RemoveAll(productBin); err != nil {
		t.Fatal(err)
	}
	again, err = c.AgentToolchainPath(false)
	if err != nil || again != want {
		t.Fatalf("bare shell erased stale cache: %q, %v", again, err)
	}
}

func TestAgentEnvEvalAndWrap(t *testing.T) {
	eval := AgentEnvEval("/tmp/a'b:/tmp/c")
	if !strings.Contains(eval, "WTC_TOOLCHAIN_PATH='/tmp/a'\\''b:/tmp/c'") || !strings.Contains(eval, "export WTC_AGENT_ENV") {
		t.Fatalf("unsafe or incomplete eval: %s", eval)
	}
	raw := []byte(`{"toolInput":{"command":"ruby -v","other":1}}`)
	wrapped := AgentEnvWrap(raw, "/tmp/harness/tools/agent-env.sh")
	var result struct {
		HookSpecificOutput struct {
			HookEventName string `json:"hookEventName"`
			UpdatedInput  struct {
				Command string `json:"command"`
				Other   int    `json:"other"`
			} `json:"updatedInput"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(wrapped, &result); err != nil {
		t.Fatal(err)
	}
	if result.HookSpecificOutput.HookEventName != "PreToolUse" || result.HookSpecificOutput.UpdatedInput.Other != 1 || !strings.Contains(result.HookSpecificOutput.UpdatedInput.Command, "ruby -v") {
		t.Fatalf("unexpected hook output: %s", wrapped)
	}
	if len(AgentEnvWrap(wrapped, "/tmp/harness/tools/agent-env.sh")) != 0 {
		t.Fatal("wrapped command twice")
	}
	if len(AgentEnvWrap(raw, "/tmp/path with spaces/agent-env.sh")) != 0 || len(AgentEnvWrap([]byte("bad"), "/tmp/a")) != 0 {
		t.Fatal("unsafe or invalid hook input was rewritten")
	}
}
