package wtc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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
	Owner         string         `yaml:"owner" json:"owner,omitempty"` // ad-hoc only: an existing checkout's Git dir
	Extra         map[string]any `yaml:",inline" json:"extra,omitempty"`
}
type Registry struct {
	SchemaVersion int               `yaml:"schema_version" json:"schema_version"`
	Repos         []Repo            `yaml:"repos" json:"repos"`
	Repositories  []Repo            `yaml:"repositories" json:"-"`
	Selected      []Repo            `yaml:"selected" json:"-"`
	NonDefault    map[string][]Repo `yaml:"non_default" json:"-"`
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
	Mise struct {
		Tools map[string]string `toml:"tools"`
	} `toml:"mise"`
	AgentEnv struct {
		PrependPaths []string `toml:"prepend_paths"`
	} `toml:"agent_env"`
	Secrets struct {
		ProdPaths []string `toml:"prod_paths"`
	} `toml:"secrets"`
	Runtime struct {
		Backend string `toml:"backend"`
		Binary  string `toml:"binary"`
	} `toml:"runtime"`
	// OpenClaw is set in a gateway's collection folder only.
	OpenClaw struct {
		// Harness names the gateway-configured harness of the collection,
		// HarnessRemote its repository.
		Harness       string `toml:"harness"`
		HarnessRemote string `toml:"harness_remote"`
	} `toml:"openclaw"`
}
type Context struct {
	Collection string   `json:"collection"`
	Harness    string   `json:"harness"`
	Workspace  string   `json:"workspace"`
	Registry   Registry `json:"registry"`
	Config     Config   `json:"-"`
	ConfigRoot string   `json:"config_root"`
	// AdHoc marks a collection without a harness repository. Its Harness is the
	// collection-local config directory and its registry is tracked nowhere.
	AdHoc bool `json:"ad_hoc,omitempty"`
	// StatusProgress receives human-readable collector steps. It is nil for
	// callers that only need the snapshot.
	StatusProgress  func(string) `json:"-"`
	previewPortBase *int
	// statusSweep marks a collection read by another collection's status
	// sweep, which reads its services itself with its own trusted binary.
	statusSweep bool
}

var validRepoName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)

func Discover(start string) (*Context, error) {
	abs, err := filepath.Abs(start)
	if err != nil {
		return nil, err
	}
	dir := abs
	for {
		if RetiredFolder(dir) {
			return nil, retiredFolderError(dir)
		}
		for _, marker := range []string{filepath.Join(AdHocConfigDir, adHocRegistryFile), filepath.Join("harness", ".harness-repos.yml")} {
			candidate := filepath.Join(dir, marker)
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return OpenCollection(dir)
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return nil, errors.New("not inside a worktree collection (harness/.harness-repos.yml not found); run `wtc init` to make this folder an ad-hoc collection")
}

func OpenCollection(dir string) (*Context, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if RetiredFolder(abs) {
		return nil, retiredFolderError(abs)
	}
	c := &Context{Collection: abs, Harness: filepath.Join(abs, "harness"), Workspace: filepath.Dir(abs)}
	// A collection's own config decides what it is, not the name of one of
	// its repositories: a gateway's folder may link a repository called
	// harness and stays ad hoc. wtc init never makes that config inside a
	// harness collection.
	if adHoc, err := os.Stat(filepath.Join(abs, AdHocConfigDir, adHocRegistryFile)); err == nil && !adHoc.IsDir() {
		c.Harness, c.AdHoc = filepath.Join(abs, AdHocConfigDir), true
	} else if info, err := os.Stat(filepath.Join(c.Harness, ".harness-repos.yml")); err != nil || info.IsDir() {
		return nil, fmt.Errorf("not a collection: %s", abs)
	}
	if err := c.load(); err != nil {
		return nil, err
	}
	return c, nil
}

// registryPath is the tracked registry of a harness or the local registry of
// an ad-hoc collection.
func (c *Context) registryPath() string {
	if c.AdHoc {
		return filepath.Join(c.Harness, adHocRegistryFile)
	}
	return filepath.Join(c.Harness, ".harness-repos.yml")
}

// ParseRegistry reads a .harness-repos.yml, in any of its layouts, into one
// list of repositories. It does not validate the entries.
func ParseRegistry(b []byte) (Registry, error) {
	var r Registry
	if err := yaml.Unmarshal(b, &r); err != nil {
		return r, fmt.Errorf("registry: %w", err)
	}
	if len(r.Repos) == 0 {
		r.Repos = r.Repositories
	}
	if len(r.Repos) == 0 {
		r.Repos = append(r.Repos, r.Selected...)
		categories := make([]string, 0, len(r.NonDefault))
		for name := range r.NonDefault {
			categories = append(categories, name)
		}
		sort.Strings(categories)
		for _, name := range categories {
			r.Repos = append(r.Repos, r.NonDefault[name]...)
		}
	}
	return r, nil
}

func (c *Context) load() error {
	b, err := os.ReadFile(c.registryPath())
	if err != nil {
		return err
	}
	if c.Registry, err = ParseRegistry(b); err != nil {
		return err
	}
	if !c.AdHoc {
		// Only a local registry may point at an owner; elsewhere the key stays
		// opaque metadata, as before ad-hoc collections existed.
		for i, r := range c.Registry.Repos {
			if r.Owner != "" {
				if r.Extra == nil {
					c.Registry.Repos[i].Extra = map[string]any{}
				}
				c.Registry.Repos[i].Extra["owner"], c.Registry.Repos[i].Owner = r.Owner, ""
			}
		}
	}
	if len(c.Registry.Repos) == 0 && !c.AdHoc {
		return errors.New("registry contains no repositories")
	}
	seen := map[string]bool{}
	portKeys := map[string]bool{}
	for _, r := range c.Registry.Repos {
		if !validRepoName.MatchString(r.Name) || seen[r.Name] {
			return fmt.Errorf("invalid or repeated repository name %q", r.Name)
		}
		seen[r.Name] = true
		if r.PortOffset != nil {
			if *r.PortOffset < 0 || *r.PortOffset >= 100 {
				return fmt.Errorf("port_offset outside collection block for %s", r.Name)
			}
			key := portKey(r.Name)
			if portKeys[key] {
				return fmt.Errorf("duplicate port variable %s", key)
			}
			portKeys[key] = true
		}
		// Preserve harness-specific metadata for commands that understand it.
		// Unrecognized fields do not make basic env/doctor operations unusable.
	}
	cfg := filepath.Join(c.Harness, "wtc.toml")
	if _, err := os.Stat(cfg); err == nil {
		if _, err := toml.DecodeFile(cfg, &c.Config); err != nil {
			return fmt.Errorf("config: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("config: %w", err)
	}
	c.ConfigRoot = os.Getenv("WTC_CONFIG_ROOT")
	if c.ConfigRoot == "" {
		data, err := os.ReadFile(filepath.Join(c.Collection, ".env.collection"))
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "WTC_CONFIG_ROOT=") {
				value := strings.TrimPrefix(line, "WTC_CONFIG_ROOT=")
				c.ConfigRoot = strings.Trim(value, "'\"")
				break
			}
		}
	}
	if c.ConfigRoot == "" && c.AdHoc {
		// An ad-hoc collection has no workspace; its secrets are per user.
		c.ConfigRoot, err = UserConfigRoot()
		if err != nil {
			return err
		}
	}
	if c.ConfigRoot == "" {
		// The control root is workspace-shared: it sits beside .bare/ and the
		// collections, so each workspace keeps its own secrets and gh identity.
		c.ConfigRoot = filepath.Join(c.Workspace, ".config")
	}
	return nil
}
