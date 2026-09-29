package wtc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMCPRenderRespectsAgentsAndDoesNotWriteCredentials(t *testing.T) {
	c := fixture(t)
	t.Setenv("EXAMPLE_TOKEN", "private-value")
	registry := `schema_version: 1
servers:
  - name: local
    transport: stdio
    command: example-server
    args: --mode safe
    env: EXAMPLE_TOKEN
    agents: claude codex
  - name: remote
    transport: http
    url: https://example.invalid/mcp
    token_env: EXAMPLE_TOKEN
    agents: cursor codex
  - name: disabled
    command: other-server
    enabled: no
`
	if err := os.WriteFile(filepath.Join(c.Harness, ".mcp-servers.yml"), []byte(registry), 0644); err != nil {
		t.Fatal(err)
	}
	render, err := c.RenderMCP()
	if err != nil {
		t.Fatal(err)
	}
	if len(render.Missing) != 0 {
		t.Fatalf("reported supplied variable missing: %v", render.Missing)
	}
	for rel, data := range render.Files {
		if strings.Contains(string(data), "private-value") || strings.Contains(string(data), "disabled") {
			t.Fatalf("%s leaked credential or disabled server: %s", rel, data)
		}
	}
	var claude map[string]any
	if err := json.Unmarshal(render.Files[".mcp.json"], &claude); err != nil {
		t.Fatal(err)
	}
	servers := claude["mcpServers"].(map[string]any)
	if len(servers) != 1 || servers["local"] == nil {
		t.Fatalf("wrong Claude servers: %v", servers)
	}
	if strings.Contains(string(render.Files[".cursor/mcp.json"]), `"local"`) {
		t.Fatal("Cursor got Claude-only server")
	}
	if !strings.Contains(string(render.Files[".codex/config.toml"]), `bearer_token_env_var = "EXAMPLE_TOKEN"`) {
		t.Fatal("Codex lost bearer token variable name")
	}
	changed, err := c.WriteMCP(render, true)
	if err != nil || len(changed) != 3 {
		t.Fatalf("dry run: %v, %v", changed, err)
	}
	if _, err := os.Stat(filepath.Join(c.Collection, ".mcp.json")); !os.IsNotExist(err) {
		t.Fatal("dry run wrote MCP config")
	}
	changed, err = c.WriteMCP(render, false)
	if err != nil || len(changed) != 3 {
		t.Fatalf("write: %v, %v", changed, err)
	}
	changed, err = c.WriteMCP(render, true)
	if err != nil || len(changed) != 0 {
		t.Fatalf("second render changed files: %v, %v", changed, err)
	}
}

func TestMCPRefusesSymlinkedConfigDirectory(t *testing.T) {
	c := fixture(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(c.Collection, ".cursor")); err != nil {
		t.Fatal(err)
	}
	_, err := c.WriteMCP(MCPRender{Files: map[string][]byte{
		".mcp.json": []byte("{}\n"), ".cursor/mcp.json": []byte("{}\n"), ".codex/config.toml": []byte("# empty\n"),
	}}, false)
	if err == nil {
		t.Fatal("followed symlinked config directory")
	}
	if _, err := os.Stat(filepath.Join(outside, "mcp.json")); !os.IsNotExist(err) {
		t.Fatal("wrote outside collection")
	}
}
