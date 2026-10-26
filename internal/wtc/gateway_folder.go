package wtc

import (
	"fmt"
	"os"
	"path/filepath"
)

// LinkedRepo is a repository of a collection whose worktree someone else
// owns: under OpenClaw, a worktree the gateway manages.
type LinkedRepo struct {
	Name       string
	Remote     string
	DefaultRef string
	Path       string
}

const gatewayRuntimeConfig = `# Written by wtc for a collection whose worktrees an OpenClaw gateway owns.
# Services run on this gateway host with wtc svc.
[runtime]
backend = "dekit"
`

// LinkGatewayCollection keeps dir an ad-hoc collection whose repositories are
// symlinks to worktrees owned elsewhere, so that one folder holds the whole
// collection and wtc svc can run its services. It creates the folder on first
// use, adds repositories it has not seen and repoints moved links. It never
// removes a repository, and it never touches anything that is not a link.
func LinkGatewayCollection(dir string, repos []LinkedRepo) (*Context, error) {
	if _, err := os.Stat(filepath.Join(dir, AdHocConfigDir, adHocRegistryFile)); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
			return nil, err
		}
		if _, err := InitAdHoc(dir); err != nil {
			return nil, err
		}
	}
	config, err := configInput(filepath.Join(dir, AdHocConfigDir))
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(config); os.IsNotExist(err) {
		if err := os.WriteFile(config, []byte(gatewayRuntimeConfig), 0o644); err != nil {
			return nil, err
		}
	}
	c, err := OpenCollection(dir)
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	for _, r := range c.Registry.Repos {
		known[r.Name] = true
	}
	for _, r := range repos {
		if !known[r.Name] {
			if err := appendRegistryEntry(filepath.Join(dir, AdHocConfigDir, adHocRegistryFile), Repo{Name: r.Name, Remote: r.Remote, DefaultRef: r.DefaultRef}); err != nil {
				return nil, err
			}
			known[r.Name] = true
		}
		link := filepath.Join(dir, r.Name)
		if target, err := os.Readlink(link); err == nil {
			if target == r.Path {
				continue
			}
			if err := os.Remove(link); err != nil {
				return nil, err
			}
		} else if _, err := os.Lstat(link); err == nil {
			return nil, fmt.Errorf("%s exists and is not a link to a worktree; leaving it alone", link)
		}
		if err := os.Symlink(r.Path, link); err != nil {
			return nil, err
		}
	}
	return OpenCollection(dir)
}

// GatewayHarness is the harness recorded for the gateway collection at dir
// and its remote: "none" (with no remote) when the person chose to work
// without one, or "" when nothing is recorded or the folder does not exist
// yet.
func GatewayHarness(dir string) (name, remote string, err error) {
	c, err := OpenCollection(dir)
	if err != nil {
		if _, statErr := os.Stat(filepath.Join(dir, AdHocConfigDir, adHocRegistryFile)); os.IsNotExist(statErr) {
			return "", "", nil
		}
		return "", "", err
	}
	return c.Config.OpenClaw.Harness, c.Config.OpenClaw.HarnessRemote, nil
}

// RecordGatewayHarness records name as the harness of the gateway collection
// at dir, creating the folder when needed; "none" records that it works
// without one. A collection has one harness or none: a different one already
// recorded is an error.
func RecordGatewayHarness(dir, name, remote string) error {
	if _, err := LinkGatewayCollection(dir, nil); err != nil {
		return err
	}
	current, _, err := GatewayHarness(dir)
	if err != nil {
		return err
	}
	if current == name {
		return nil
	}
	if current != "" {
		return fmt.Errorf("collection %s already uses harness %s", filepath.Base(dir), current)
	}
	config, err := configInput(filepath.Join(dir, AdHocConfigDir))
	if err != nil {
		return err
	}
	f, err := os.OpenFile(config, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(f, "\n[openclaw]\nharness = %q\nharness_remote = %q\n", name, remote); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
