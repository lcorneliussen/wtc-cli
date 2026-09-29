package wtc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

type Repo struct {
	Name          string         `yaml:"name" json:"name"`
	Remote        string         `yaml:"remote" json:"remote"`
	DefaultRef    string         `yaml:"default_ref" json:"default_ref"`
	PortOffset    *int           `yaml:"port_offset" json:"port_offset,omitempty"`
	Role          string         `yaml:"role" json:"role,omitempty"`
	IssuesPrefix  string         `yaml:"issues_prefix" json:"issues_prefix,omitempty"`
	ProductionRef string         `yaml:"production_ref" json:"production_ref,omitempty"`
	ReleaseRef    string         `yaml:"release_ref" json:"release_ref,omitempty"`
	ProdPaths     []string       `yaml:"prod_paths" json:"prod_paths,omitempty"`
	IDE           string         `yaml:"ide" json:"ide,omitempty"`
	MergeStrategy string         `yaml:"merge_strategy" json:"merge_strategy,omitempty"`
	Extra         map[string]any `yaml:",inline" json:"extra,omitempty"`
}
type Registry struct {
	SchemaVersion int    `yaml:"schema_version" json:"schema_version"`
	Repos         []Repo `yaml:"repos" json:"repos"`
	Repositories  []Repo `yaml:"repositories" json:"-"`
}
type Config struct {
	Harness struct {
		Name string `toml:"name"`
	} `toml:"harness"`
	Herdr struct {
		Session string `toml:"session"`
	} `toml:"herdr"`
	Compatibility struct {
		Requires string `toml:"requires"`
	} `toml:"compatibility"`
}
type Context struct {
	Collection string   `json:"collection"`
	Harness    string   `json:"harness"`
	Workspace  string   `json:"workspace"`
	Registry   Registry `json:"registry"`
	Config     Config   `json:"-"`
	ConfigRoot string   `json:"config_root"`
}

func Discover(start string) (*Context, error) {
	abs, err := filepath.Abs(start)
	if err != nil {
		return nil, err
	}
	dir := abs
	for {
		candidate := filepath.Join(dir, "harness", ".harness-repos.yml")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return OpenCollection(dir)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return nil, errors.New("not inside a worktree collection (harness/.harness-repos.yml not found)")
}

func OpenCollection(dir string) (*Context, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(filepath.Join(abs, "harness", ".harness-repos.yml"))
	if err != nil {
		return nil, fmt.Errorf("not a collection: %s", abs)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("not a collection: %s", abs)
	}
	c := &Context{Collection: abs, Harness: filepath.Join(abs, "harness"), Workspace: filepath.Dir(abs)}
	if err := c.load(); err != nil {
		return nil, err
	}
	return c, nil
}
func (c *Context) load() error {
	b, err := os.ReadFile(filepath.Join(c.Harness, ".harness-repos.yml"))
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(b, &c.Registry); err != nil {
		return fmt.Errorf("registry: %w", err)
	}
	if len(c.Registry.Repos) == 0 {
		c.Registry.Repos = c.Registry.Repositories
	}
	if len(c.Registry.Repos) == 0 {
		return errors.New("registry contains no repositories")
	}
	seen := map[string]bool{}
	for _, r := range c.Registry.Repos {
		if r.Name == "" || seen[r.Name] {
			return fmt.Errorf("invalid or repeated repository name %q", r.Name)
		}
		seen[r.Name] = true
		for key := range r.Extra {
			if !strings.HasPrefix(key, "x-") {
				return fmt.Errorf("unsupported registry field %q in %s", key, r.Name)
			}
		}
	}
	cfg := filepath.Join(c.Harness, "wtc.toml")
	if _, err := os.Stat(cfg); err == nil {
		if _, err := toml.DecodeFile(cfg, &c.Config); err != nil {
			return fmt.Errorf("config: %w", err)
		}
	}
	c.ConfigRoot = os.Getenv("WTC_CONFIG_ROOT")
	if c.ConfigRoot == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		c.ConfigRoot = filepath.Join(home, ".config", "wtc")
	}
	return nil
}
