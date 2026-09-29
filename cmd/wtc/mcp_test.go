package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestMCPSweepUsesEachRegistryAndContinuesAfterFailure(t *testing.T) {
	workspace := t.TempDir()
	for name, registry := range map[string]string{
		"alpha":  "schema_version: 1\nservers:\n  - name: alpha-server\n    command: alpha-command\n    env: TARGET_TOKEN\n",
		"broken": "schema_version: 2\nservers: []\n",
	} {
		harness := filepath.Join(workspace, name, "harness")
		if err := os.MkdirAll(harness, 0755); err != nil {
			t.Fatal(err)
		}
		for file, body := range map[string]string{
			".harness-repos.yml": "repos:\n  - name: fixture\n    remote: https://example.invalid/fixture.git\n    default_ref: origin/main\n",
			".mcp-servers.yml":   registry,
		} {
			if err := os.WriteFile(filepath.Join(harness, file), []byte(body), 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
	// An older worktree has a harness but no MCP registry.
	if err := os.MkdirAll(filepath.Join(workspace, "older", "harness"), 0755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "wtc")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	run := func(dry bool) (map[string]any, error) {
		args := []string{"--json", "mcp", "render", "--all", "--collection", filepath.Join(workspace, "alpha")}
		if dry {
			args = append(args, "--dry-run")
		}
		cmd := exec.Command(bin, args...)
		out, err := cmd.Output()
		if len(out) == 0 {
			t.Fatalf("no JSON output: %v", err)
		}
		var payload map[string]any
		if jsonErr := json.Unmarshal(out, &payload); jsonErr != nil {
			t.Fatalf("JSON: %v\n%s", jsonErr, out)
		}
		return payload, err
	}
	for _, dry := range []bool{true, false} {
		payload, err := run(dry)
		if err == nil || payload["ok"] != false {
			t.Fatalf("failed target was not reported: dry=%v payload=%v err=%v", dry, payload, err)
		}
		data := payload["data"].(map[string]any)
		if data["failed"] != float64(1) || len(data["results"].([]any)) != 3 {
			t.Fatalf("unexpected sweep result: %v", data)
		}
		if data["missing_env_diagnostics"] != false {
			t.Fatalf("sweep claimed target credential diagnostics: %v", data)
		}
		byName := map[string]map[string]any{}
		for _, raw := range data["results"].([]any) {
			item := raw.(map[string]any)
			byName[filepath.Base(item["collection"].(string))] = item
		}
		if len(byName["alpha"]["changed"].([]any)) != 3 || byName["broken"]["error"] == nil || byName["older"]["absent"] != true {
			t.Fatalf("lost per-target result: %v", byName)
		}
		if byName["alpha"]["missing_env"] != nil {
			t.Fatalf("used invoking shell to diagnose target credentials: %v", byName["alpha"])
		}
		alphaConfig := filepath.Join(workspace, "alpha", ".mcp.json")
		config, readErr := os.ReadFile(alphaConfig)
		if dry {
			if !os.IsNotExist(readErr) {
				t.Fatalf("dry run wrote config: %s, %v", config, readErr)
			}
		} else if readErr != nil || !json.Valid(config) || !containsMCPServer(config, "alpha-server") {
			t.Fatalf("valid target was not rendered: %s, %v", config, readErr)
		}
		for _, name := range []string{"broken", "older"} {
			if _, err := os.Stat(filepath.Join(workspace, name, ".mcp.json")); !os.IsNotExist(err) {
				t.Fatalf("%s was written: %v", name, err)
			}
		}
	}
	if err := os.WriteFile(filepath.Join(workspace, "broken", "harness", ".mcp-servers.yml"), []byte("schema_version: 1\nservers: []\n"), 0644); err != nil {
		t.Fatal(err)
	}
	payload, err := run(false)
	if err != nil || payload["ok"] != true {
		t.Fatalf("recovered sweep failed: %v, %v", payload, err)
	}
}

func containsMCPServer(data []byte, name string) bool {
	var config struct {
		Servers map[string]json.RawMessage `json:"mcpServers"`
	}
	return json.Unmarshal(data, &config) == nil && config.Servers[name] != nil
}
