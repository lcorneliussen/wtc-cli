package openclaw

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// PluginConfigKey is where the gateway keeps the wtc plugin's config.
const PluginConfigKey = "plugins.entries.wtc.config"

// Harness is a harness repository the gateway is configured with. A
// collection that uses it gets a managed worktree of it, like any of its
// repositories, and resolves repository names through its registry.
type Harness struct {
	Name   string `json:"name"`
	Remote string `json:"remote"`
	// Ref is the branch new worktrees of the harness start from.
	Ref string `json:"ref,omitempty"`
}

// GatewayConfig is the part of the plugin's config that wtc reads.
type GatewayConfig struct {
	Harnesses      []Harness `json:"harnesses,omitempty"`
	DefaultHarness string    `json:"defaultHarness,omitempty"`
}

// NoHarness is what --harness takes for a collection without one.
const NoHarness = "none"

var (
	harnessName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
	branchName  = regexp.MustCompile(`^[A-Za-z0-9._][A-Za-z0-9._/-]*$`)
)

// ParseGatewayConfig reads what `openclaw config get` prints for
// PluginConfigKey. Empty output is a gateway without harnesses.
func ParseGatewayConfig(out string) (GatewayConfig, error) {
	var cfg GatewayConfig
	start := strings.Index(out, "{")
	if start < 0 {
		return cfg, nil
	}
	if err := json.Unmarshal([]byte(out[start:]), &cfg); err != nil {
		return cfg, fmt.Errorf("%s: %w", PluginConfigKey, err)
	}
	seen := map[string]bool{}
	for i, h := range cfg.Harnesses {
		if !harnessName.MatchString(h.Name) || h.Name == NoHarness {
			return cfg, fmt.Errorf("%s: harness name %q: lowercase letters, digits and hyphens, and not %q", PluginConfigKey, h.Name, NoHarness)
		}
		if seen[h.Name] {
			return cfg, fmt.Errorf("%s: harness %s is configured twice", PluginConfigKey, h.Name)
		}
		seen[h.Name] = true
		if o := ParseOrigin(h.Remote); o.Forge == ForgeUnknown || o.Owner == "" || o.Repo == "" {
			return cfg, fmt.Errorf("%s: harness %s: remote %q is not a GitHub or Bitbucket repository", PluginConfigKey, h.Name, h.Remote)
		}
		if h.Ref == "" {
			cfg.Harnesses[i].Ref = "main"
		} else if !branchName.MatchString(h.Ref) || strings.Contains(h.Ref, "..") {
			return cfg, fmt.Errorf("%s: harness %s: ref %q is not a branch name", PluginConfigKey, h.Name, h.Ref)
		}
	}
	if cfg.DefaultHarness != "" && !seen[cfg.DefaultHarness] {
		return cfg, fmt.Errorf("%s: defaultHarness %s is not one of the configured harnesses", PluginConfigKey, cfg.DefaultHarness)
	}
	return cfg, nil
}

// Find is the configured harness called name, or nil.
func (c GatewayConfig) Find(name string) *Harness {
	for i := range c.Harnesses {
		if c.Harnesses[i].Name == name {
			return &c.Harnesses[i]
		}
	}
	return nil
}

func (c GatewayConfig) names() string {
	var names []string
	for _, h := range c.Harnesses {
		names = append(names, h.Name)
	}
	return strings.Join(names, ", ")
}

// ChooseHarness decides the harness of a collection, or nil for none. flag is
// --harness, recorded the harness the collection already has, and names the
// repositories add-repo was given by name; has says whether a harness's
// registry lists one. A collection keeps its harness. Otherwise the choice
// has to be clear: given, the default, the only one, or the only one that
// lists every name. When it is not, the agent asks the person.
func ChooseHarness(cfg GatewayConfig, flag, recorded string, names []string, has func(Harness, string) bool) (*Harness, error) {
	ask := fmt.Sprintf("Ask the person which harness this collection works in (%s), then pass --harness <name>, or --harness %s for a collection without one", cfg.names(), NoHarness)
	switch {
	case recorded != "":
		h := cfg.Find(recorded)
		if h == nil {
			return nil, fmt.Errorf("this collection uses harness %s, which the gateway no longer configures. Ask the person to configure it again under %s.harnesses", recorded, PluginConfigKey)
		}
		if flag != "" && flag != recorded {
			return nil, fmt.Errorf("this collection uses harness %s; a collection has one harness. Start another collection for work in %s", recorded, flag)
		}
		return h, nil
	case flag == NoHarness:
		return nil, nil
	case flag != "":
		h := cfg.Find(flag)
		if h == nil {
			if len(cfg.Harnesses) == 0 {
				return nil, fmt.Errorf("the gateway configures no harnesses, so there is no harness %s", flag)
			}
			return nil, fmt.Errorf("the gateway configures no harness %s. %s", flag, ask)
		}
		return h, nil
	case len(cfg.Harnesses) == 0:
		return nil, nil
	case cfg.DefaultHarness != "":
		return cfg.Find(cfg.DefaultHarness), nil
	case len(cfg.Harnesses) == 1:
		return &cfg.Harnesses[0], nil
	}
	var fits []*Harness
	if len(names) > 0 {
		for i, h := range cfg.Harnesses {
			all := true
			for _, n := range names {
				all = all && has(h, n)
			}
			if all {
				fits = append(fits, &cfg.Harnesses[i])
			}
		}
	}
	if len(fits) == 1 {
		return fits[0], nil
	}
	return nil, fmt.Errorf("which harness? %s", ask)
}

// FileAt is the content of path at ref in the repository at dir, read with
// the trusted git.
func FileAt(dir, ref, path string) ([]byte, error) {
	out, err := git(dir, "show", ref+":"+path)
	if err != nil {
		return nil, err
	}
	return []byte(out + "\n"), nil
}

// SameRepository reports whether two remotes name the same forge repository.
func SameRepository(a, b string) bool {
	oa, ob := ParseOrigin(a), ParseOrigin(b)
	return oa.Forge != ForgeUnknown && oa.Forge == ob.Forge && strings.EqualFold(oa.Owner, ob.Owner) && strings.EqualFold(oa.Repo, ob.Repo)
}

// CheckoutOf reports whether the checkout at dir is of the repository at
// remote, by its origin.
func CheckoutOf(dir, remote string) bool {
	url, err := git(dir, "remote", "get-url", "origin")
	return err == nil && SameRepository(url, remote)
}
