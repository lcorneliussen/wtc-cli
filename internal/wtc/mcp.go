package wtc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

type MCPServer struct {
	Name      string `yaml:"name"`
	Transport string `yaml:"transport"`
	Command   string `yaml:"command"`
	Args      string `yaml:"args"`
	URL       string `yaml:"url"`
	Env       string `yaml:"env"`
	TokenEnv  string `yaml:"token_env"`
	Agents    string `yaml:"agents"`
	Enabled   *bool  `yaml:"enabled"`
}

type mcpRegistry struct {
	SchemaVersion int         `yaml:"schema_version"`
	Servers       []MCPServer `yaml:"servers"`
}

type MCPRender struct {
	Files   map[string][]byte `json:"-"`
	Missing []string          `json:"missing_env,omitempty"`
}

var envIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func (c *Context) RenderMCP() (MCPRender, error) {
	path := filepath.Join(c.Harness, ".mcp-servers.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		return MCPRender{}, err
	}
	var registry mcpRegistry
	if err := yaml.Unmarshal(data, &registry); err != nil {
		return MCPRender{}, fmt.Errorf("MCP registry: %w", err)
	}
	if registry.SchemaVersion != 1 {
		return MCPRender{}, fmt.Errorf("unsupported MCP registry schema_version %d", registry.SchemaVersion)
	}
	seen := map[string]bool{}
	missing := map[string]bool{}
	for _, s := range registry.Servers {
		if s.Name == "" || seen[s.Name] || strings.ContainsAny(s.Name, "\r\n\x00") {
			return MCPRender{}, fmt.Errorf("invalid or duplicate MCP server name %q", s.Name)
		}
		seen[s.Name] = true
		if s.Transport != "" && s.Transport != "stdio" && s.Transport != "http" {
			return MCPRender{}, fmt.Errorf("%s: unsupported transport %q", s.Name, s.Transport)
		}
		if s.Transport == "http" && s.URL == "" || s.Transport != "http" && s.Command == "" {
			return MCPRender{}, fmt.Errorf("%s: missing URL or command", s.Name)
		}
		if s.TokenEnv != "" && !envIdentifier.MatchString(s.TokenEnv) {
			return MCPRender{}, fmt.Errorf("%s: invalid token_env %q", s.Name, s.TokenEnv)
		}
		if _, err := mcpArgs(s.Args); err != nil {
			return MCPRender{}, fmt.Errorf("%s: %w", s.Name, err)
		}
		for _, v := range append(strings.Fields(s.Env), s.TokenEnv) {
			if v == "" {
				continue
			}
			if !envIdentifier.MatchString(v) {
				return MCPRender{}, fmt.Errorf("%s: invalid env variable %q", s.Name, v)
			}
			if s.Enabled == nil || *s.Enabled {
				if os.Getenv(v) == "" {
					missing[v] = true
				}
			}
		}
		for _, agent := range strings.Fields(s.Agents) {
			if agent != "claude" && agent != "cursor" && agent != "codex" {
				return MCPRender{}, fmt.Errorf("%s: unknown agent %q", s.Name, agent)
			}
		}
	}
	files := map[string][]byte{}
	for _, agent := range []string{"claude", "cursor"} {
		servers := map[string]any{}
		for _, s := range registry.Servers {
			if !mcpEnabledFor(s, agent) {
				continue
			}
			servers[s.Name] = mcpJSONServer(s, agent)
		}
		body, err := json.MarshalIndent(map[string]any{"_generated": "wtc mcp render — generated servers follow harness/.mcp-servers.yml", "_wtcGeneratedServers": sortedMCPNames(servers), "mcpServers": servers}, "", "  ")
		if err != nil {
			return MCPRender{}, err
		}
		body = append(body, '\n')
		if agent == "claude" {
			files[".mcp.json"] = body
		} else {
			files[".cursor/mcp.json"] = body
		}
	}
	var toml bytes.Buffer
	toml.WriteString("# BEGIN WTC MCP SERVERS\n# Generated from harness/.mcp-servers.yml.\n")
	for _, s := range registry.Servers {
		if !mcpEnabledFor(s, "codex") {
			continue
		}
		fmt.Fprintf(&toml, "\n[mcp_servers.%q]\n", s.Name)
		if s.Transport == "http" {
			fmt.Fprintf(&toml, "url = %q\n", s.URL)
			if s.TokenEnv != "" {
				fmt.Fprintf(&toml, "bearer_token_env_var = %q\n", s.TokenEnv)
			}
		} else {
			fmt.Fprintf(&toml, "command = %q\n", s.Command)
			if s.Args != "" {
				args, _ := mcpArgs(s.Args)
				fmt.Fprintf(&toml, "args = %s\n", jsonArray(args))
			}
			if s.Env != "" {
				fmt.Fprintf(&toml, "env_vars = %s\n", jsonArray(strings.Fields(s.Env)))
			}
		}
	}
	toml.WriteString("\n# END WTC MCP SERVERS\n")
	files[".codex/config.toml"] = toml.Bytes()
	var missingNames []string
	for name := range missing {
		missingNames = append(missingNames, name)
	}
	sort.Strings(missingNames)
	return MCPRender{Files: files, Missing: missingNames}, nil
}

func mcpEnabledFor(s MCPServer, agent string) bool {
	if s.Enabled != nil && !*s.Enabled {
		return false
	}
	if s.Agents == "" {
		return true
	}
	for _, name := range strings.Fields(s.Agents) {
		if name == agent {
			return true
		}
	}
	return false
}

func mcpJSONServer(s MCPServer, agent string) map[string]any {
	item := map[string]any{}
	if s.Transport == "http" {
		if agent == "claude" {
			item["type"] = "http"
		}
		item["url"] = s.URL
		if s.TokenEnv != "" {
			placeholder := "${" + s.TokenEnv + "}"
			if agent == "cursor" {
				placeholder = "${env:" + s.TokenEnv + "}"
			}
			item["headers"] = map[string]string{"Authorization": "Bearer " + placeholder}
		}
	} else {
		if agent == "cursor" {
			item["type"] = "stdio"
		}
		item["command"] = s.Command
		if s.Args != "" {
			args, _ := mcpArgs(s.Args)
			item["args"] = args
		}
		if s.Env != "" {
			env := map[string]string{}
			for _, name := range strings.Fields(s.Env) {
				if agent == "cursor" {
					env[name] = "${env:" + name + "}"
				} else {
					env[name] = "${" + name + "}"
				}
			}
			item["env"] = env
		}
	}
	return item
}

func mcpArgs(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "[") {
		var args []string
		if err := json.Unmarshal([]byte(raw), &args); err != nil {
			return nil, fmt.Errorf("invalid JSON args array: %w", err)
		}
		return args, nil
	}
	return strings.Fields(raw), nil
}

func sortedMCPNames(servers map[string]any) []string {
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func jsonArray(items []string) string {
	data, _ := json.Marshal(items)
	return string(data)
}

func mergeMCPJSON(old, generated []byte) ([]byte, error) {
	if len(bytes.TrimSpace(old)) == 0 {
		return generated, nil
	}
	var existing map[string]json.RawMessage
	if err := json.Unmarshal(old, &existing); err != nil {
		return nil, fmt.Errorf("existing MCP JSON: %w", err)
	}
	var generatedRoot map[string]json.RawMessage
	if err := json.Unmarshal(generated, &generatedRoot); err != nil {
		return nil, err
	}
	var priorGenerated string
	_ = json.Unmarshal(existing["_generated"], &priorGenerated)
	if strings.HasPrefix(priorGenerated, "wtc mcp render") && existing["_wtcGeneratedServers"] == nil {
		return generated, nil // Previous CLI versions owned the whole file.
	}
	var currentServers, generatedServers map[string]json.RawMessage
	if len(existing["mcpServers"]) > 0 {
		if err := json.Unmarshal(existing["mcpServers"], &currentServers); err != nil {
			return nil, fmt.Errorf("existing mcpServers: %w", err)
		}
	}
	if err := json.Unmarshal(generatedRoot["mcpServers"], &generatedServers); err != nil {
		return nil, err
	}
	if currentServers == nil {
		currentServers = map[string]json.RawMessage{}
	}
	var priorNames []string
	if len(existing["_wtcGeneratedServers"]) > 0 {
		if err := json.Unmarshal(existing["_wtcGeneratedServers"], &priorNames); err != nil {
			return nil, fmt.Errorf("existing generated server list: %w", err)
		}
	}
	for _, name := range priorNames {
		delete(currentServers, name)
	}
	for name, body := range generatedServers {
		currentServers[name] = body
	}
	serverBody, _ := json.Marshal(currentServers)
	existing["mcpServers"] = serverBody
	existing["_wtcGeneratedServers"] = generatedRoot["_wtcGeneratedServers"]
	if strings.HasPrefix(priorGenerated, "wtc mcp render") {
		existing["_generated"] = generatedRoot["_generated"]
	}
	merged, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(merged, '\n'), nil
}

func mergeMCPTOML(old, generated []byte) ([]byte, error) {
	const begin = "# BEGIN WTC MCP SERVERS"
	const end = "# END WTC MCP SERVERS"
	base := string(old)
	if start := strings.Index(base, begin); start >= 0 {
		stop := strings.Index(base[start:], end)
		if stop < 0 {
			return nil, fmt.Errorf("existing Codex config has an unterminated generated MCP block")
		}
		stop += start + len(end)
		if stop < len(base) && base[stop] == '\n' {
			stop++
		}
		base = base[:start] + base[stop:]
	} else if strings.HasPrefix(base, "# Generated by wtc mcp render") {
		// The first renderer owned whole files. Preserve any unrelated settings
		// someone added to one, while removing its old MCP tables.
		var kept []string
		skip := false
		for _, line := range strings.Split(base, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "[mcp_servers.") {
				skip = true
			} else if strings.HasPrefix(trimmed, "[") {
				skip = false
			}
			if !skip && !strings.HasPrefix(trimmed, "# Generated by wtc mcp render") && !strings.HasPrefix(trimmed, "# Change harness/.mcp-servers.yml") {
				kept = append(kept, line)
			}
		}
		base = strings.TrimSpace(strings.Join(kept, "\n"))
	}
	base = strings.TrimRight(base, "\n")
	if base != "" {
		base += "\n\n"
	}
	merged := []byte(base + string(generated))
	var parsed map[string]any
	if _, err := toml.Decode(string(merged), &parsed); err != nil {
		return nil, fmt.Errorf("merged Codex config: %w", err)
	}
	return merged, nil
}

// WriteMCP reports changed generated paths. A dry run leaves all files alone.
func (c *Context) WriteMCP(render MCPRender, dryRun bool) ([]string, error) {
	paths := []string{".mcp.json", ".cursor/mcp.json", ".codex/config.toml"}
	var changed []string
	contents := map[string][]byte{}
	for _, rel := range paths {
		target := filepath.Join(c.Collection, rel)
		if filepath.Dir(target) != c.Collection {
			parent, err := os.Lstat(filepath.Dir(target))
			if err == nil && !parent.IsDir() {
				return nil, fmt.Errorf("refusing non-directory MCP config parent: %s", filepath.Dir(target))
			}
			if err != nil && !os.IsNotExist(err) {
				return nil, err
			}
		}
		info, err := os.Lstat(target)
		if err == nil && !info.Mode().IsRegular() {
			return nil, fmt.Errorf("refusing non-regular generated MCP file: %s", target)
		}
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		old, err := os.ReadFile(target)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		body := render.Files[rel]
		if rel == ".codex/config.toml" {
			body, err = mergeMCPTOML(old, body)
		} else {
			body, err = mergeMCPJSON(old, body)
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", rel, err)
		}
		contents[rel] = body
		if !bytes.Equal(old, body) {
			changed = append(changed, rel)
		}
	}
	if dryRun {
		return changed, nil
	}
	staged := map[string]string{}
	defer func() {
		for _, path := range staged {
			_ = os.Remove(path)
		}
	}()
	for _, rel := range changed {
		target := filepath.Join(c.Collection, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return changed, err
		}
		tmp, err := os.CreateTemp(filepath.Dir(target), ".wtc-mcp-")
		if err != nil {
			return changed, err
		}
		staged[rel] = tmp.Name()
		if _, err := tmp.Write(contents[rel]); err != nil {
			tmp.Close()
			return changed, err
		}
		if err := tmp.Chmod(0644); err != nil {
			tmp.Close()
			return changed, err
		}
		if err := tmp.Close(); err != nil {
			return changed, err
		}
	}
	for _, rel := range changed {
		if err := os.Rename(staged[rel], filepath.Join(c.Collection, rel)); err != nil {
			return changed, err
		}
		delete(staged, rel)
	}
	return changed, nil
}
