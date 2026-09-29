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

func TestMCPMergePreservesManualSettingsAndUsesAgentInterpolation(t *testing.T) {
	c := fixture(t)
	registry := `schema_version: 1
servers:
  - name: local
    transport: stdio
    command: example-server
    args: '["arg with space", "--safe"]'
    env: EXAMPLE_TOKEN
  - name: remote
    transport: http
    url: https://example.invalid/mcp
    token_env: EXAMPLE_TOKEN
`
	if err := os.WriteFile(filepath.Join(c.Harness, ".mcp-servers.yml"), []byte(registry), 0644); err != nil {
		t.Fatal(err)
	}
	for rel, body := range map[string]string{
		".mcp.json":          `{"mcpServers":{"personal":{"command":"personal-server"}}}`,
		".cursor/mcp.json":   `{"mcpServers":{"personal":{"command":"personal-server"}}}`,
		".codex/config.toml": "model = \"example\"\n[features]\nsearch = true\n",
	} {
		path := filepath.Join(c.Collection, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	render, err := c.RenderMCP()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.WriteMCP(render, false); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{".mcp.json", ".cursor/mcp.json"} {
		data, err := os.ReadFile(filepath.Join(c.Collection, rel))
		if err != nil {
			t.Fatal(err)
		}
		var root struct {
			MCPServers map[string]map[string]any `json:"mcpServers"`
		}
		if err := json.Unmarshal(data, &root); err != nil {
			t.Fatal(err)
		}
		if len(root.MCPServers) != 3 || root.MCPServers["personal"] == nil {
			t.Fatalf("%s lost manual server: %s", rel, data)
		}
		args := root.MCPServers["local"]["args"].([]any)
		if args[0] != "arg with space" {
			t.Fatalf("%s split one argument: %v", rel, args)
		}
		want := "${EXAMPLE_TOKEN}"
		if rel == ".cursor/mcp.json" {
			want = "${env:EXAMPLE_TOKEN}"
			if root.MCPServers["remote"]["type"] != nil {
				t.Fatalf("Cursor got Claude's remote type: %v", root.MCPServers["remote"])
			}
		}
		if root.MCPServers["local"]["env"].(map[string]any)["EXAMPLE_TOKEN"] != want {
			t.Fatalf("%s wrong env interpolation", rel)
		}
		if root.MCPServers["remote"]["headers"].(map[string]any)["Authorization"] != "Bearer "+want {
			t.Fatalf("%s wrong header interpolation", rel)
		}
	}
	codex, err := os.ReadFile(filepath.Join(c.Collection, ".codex/config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(codex), `model = "example"`) || !strings.Contains(string(codex), "search = true") || !strings.Contains(string(codex), `args = ["arg with space","--safe"]`) {
		t.Fatalf("Codex settings or args lost: %s", codex)
	}
	if changed, err := c.WriteMCP(render, true); err != nil || len(changed) != 0 {
		t.Fatalf("render is not stable: changed=%v err=%v", changed, err)
	}
}

func TestMCPRegistryRejectsUnknownSchemaAndInvalidTokenName(t *testing.T) {
	c := fixture(t)
	path := filepath.Join(c.Harness, ".mcp-servers.yml")
	for _, body := range []string{
		"schema_version: 2\nservers: []\n",
		"schema_version: 1\nservers:\n  - name: remote\n    transport: http\n    url: https://example.invalid\n    token_env: TOKEN_A TOKEN_B\n",
	} {
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := c.RenderMCP(); err == nil {
			t.Fatalf("accepted invalid registry: %s", body)
		}
	}
}
