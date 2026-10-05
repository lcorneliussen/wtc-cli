package wtc

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/Masterminds/semver/v3"
	"gopkg.in/yaml.v3"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

func runtimeURLSafe(value string) error {
	u, err := url.Parse(value)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("runtime URL must be HTTP(S) without credentials, query or fragment")
	}
	return nil
}

func runtimeURLFromEnv(collection, name string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", `set -ea; [ ! -f "$1" ] || . "$1"; [ ! -f "$2" ] || . "$2"; printenv "$3"`, "wtc-runtime-url", filepath.Join(collection, ".env.collection"), filepath.Join(collection, ".env.collection.local"), name)
	cmd.Dir = collection
	var out statusLimitedBuffer
	out.limit = 4096
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("cannot read endpoint variable %s from collection env: %w", name, err)
	}
	value := strings.TrimSpace(out.String())
	if err := runtimeURLSafe(value); err != nil {
		return "", err
	}
	return value, nil
}

func (c *Context) runtimeBinary() (string, error) {
	name := c.Config.Runtime.Binary
	if name == "" {
		name = "dekit"
	}
	if strings.ContainsRune(name, filepath.Separator) && !filepath.IsAbs(name) {
		name = filepath.Join(c.Harness, name)
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("dekit 0.10.x is required; install the harness's pinned runtime tool: %w", err)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", err
	}
	out, err := statusCommand(5*time.Second, path, "--version")
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(out))
	if len(fields) != 2 || fields[0] != "dekit" {
		return "", fmt.Errorf("expected dekit 0.10.x, got %q", strings.TrimSpace(string(out)))
	}
	v, err := semver.NewVersion(fields[1])
	if err != nil || v.Major() != 0 || v.Minor() != 10 {
		return "", fmt.Errorf("this runtime adapter requires dekit 0.10.x, got %s", fields[1])
	}
	return path, nil
}

// runtimePlan copies native dekit task definitions, rebasing only filesystem
// paths and dependency names. It neither runs hooks nor writes generated files.
func (c *Context) runtimePlan(binary string) (*runtimeManifest, []byte, error) {
	if c.Config.Runtime.Backend != "dekit" {
		return nil, nil, fmt.Errorf("enable [runtime] backend = \"dekit\" in harness/wtc.toml")
	}
	_, repos, err := c.CatchUpInventory(CatchUpOptions{DryRun: true})
	if err != nil {
		return nil, nil, err
	}
	m := &runtimeManifest{Schema: 1, Root: c.runtimeRoot(), Binary: binary, Tasks: []runtimeTask{}}
	native := map[string]any{}
	for _, repo := range repos {
		path := filepath.Join(repo.path, ".harness", "dekit.tasks.yaml")
		if err := runtimeRegular(path); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return nil, nil, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, nil, err
		}
		var fragment struct {
			Tasks map[string]map[string]any `yaml:"tasks"`
		}
		decoder := yaml.NewDecoder(bytes.NewReader(data))
		decoder.KnownFields(true)
		if err := decoder.Decode(&fragment); err != nil {
			return nil, nil, fmt.Errorf("%s: %w", path, err)
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			return nil, nil, fmt.Errorf("%s: expected one YAML document", path)
		}
		dir := filepath.Base(repo.path)
		for local, task := range fragment.Tasks {
			if !runtimeTaskPath(local) || task == nil {
				return nil, nil, fmt.Errorf("invalid task path %q in %s", local, path)
			}
			full := dir + "/" + local
			item := runtimeTask{RuntimeItem: RuntimeItem{Collection: filepath.Base(c.Collection), Repo: repo.repo, Dir: dir, Path: full, Group: dir + "/" + strings.Split(local, "/")[0], Kind: "service", State: "unconfigured"}}
			if kind, _ := task["type"].(string); kind == "job" {
				item.Kind = "job"
			}
			item.Label, _ = task["label"].(string)
			if metadata, ok := task["x-wtc"].(map[string]any); ok {
				for key := range metadata {
					if key != "url" && key != "url_env" {
						return nil, nil, fmt.Errorf("%s: unsupported x-wtc field %s", full, key)
					}
				}
				item.URL, _ = metadata["url"].(string)
				if name, _ := metadata["url_env"].(string); name != "" {
					if !regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`).MatchString(name) {
						return nil, nil, fmt.Errorf("%s: invalid URL variable", full)
					}
					if filepath.IsAbs(binary) {
						value, err := runtimeURLFromEnv(c.Collection, name)
						if err != nil {
							return nil, nil, err
						}
						item.URL = value
					}
				}
				if item.URL != "" {
					if err := runtimeURLSafe(item.URL); err != nil {
						return nil, nil, fmt.Errorf("%s: %w", full, err)
					}
				}
				delete(task, "x-wtc")
			} else if _, exists := task["x-wtc"]; exists {
				return nil, nil, fmt.Errorf("%s: x-wtc must be an object", full)
			}
			if tags, ok := task["tags"].([]any); ok {
				for _, tag := range tags {
					if tag == "endpoint" || tag == "accessory" {
						item.Kind = tag.(string)
					}
				}
			}
			if ready, ok := task["ready"].(map[string]any); ok {
				item.Ready = true
				if item.URL == "" {
					item.URL, _ = ready["http"].(string)
				}
			}
			item.Autostart, _ = task["autostart"].(bool)
			if deps, ok := task["deps"].([]any); ok {
				for _, value := range deps {
					dep, ok := value.(string)
					if !ok {
						return nil, nil, fmt.Errorf("%s: dependency must be a task path", full)
					}
					if strings.HasPrefix(dep, "/") {
						dep = strings.TrimPrefix(dep, "/")
					} else {
						dep = dir + "/" + dep
					}
					if !runtimeTaskPath(dep) {
						return nil, nil, fmt.Errorf("%s: invalid dependency %q", full, dep)
					}
					item.Deps = append(item.Deps, dep)
				}
				task["deps"] = item.Deps
			}
			cwd, _ := task["cwd"].(string)
			if cwd == "" {
				cwd = "."
			}
			if !filepath.IsAbs(cwd) {
				cwd = filepath.Join(repo.path, cwd)
			}
			resolved, err := filepath.EvalSymlinks(cwd)
			if err != nil {
				return nil, nil, fmt.Errorf("%s cwd: %w", full, err)
			}
			base, baseErr := filepath.EvalSymlinks(repo.path)
			if baseErr != nil {
				return nil, nil, baseErr
			}
			rel, err := filepath.Rel(base, resolved)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return nil, nil, fmt.Errorf("%s cwd must stay inside its repository", full)
			}
			task["cwd"] = resolved
			if script, ok := task["script"].(string); ok && !filepath.IsAbs(script) {
				task["script"] = filepath.Join(repo.path, script)
			}
			item.Log = filepath.Join(m.Root, "logs", filepath.FromSlash(full)+".log")
			task["log"] = map[string]any{"enabled": true, "file": item.Log, "mode": "append"}
			native[full] = task
			m.Tasks = append(m.Tasks, item)
		}
	}
	if len(m.Tasks) == 0 {
		return nil, nil, fmt.Errorf("no opted-in repositories (.harness/dekit.tasks.yaml)")
	}
	for _, task := range m.Tasks {
		for _, dep := range task.Deps {
			if _, ok := native[dep]; !ok {
				return nil, nil, fmt.Errorf("%s: dependency %s is not present in this collection", task.Path, dep)
			}
		}
	}
	sort.Slice(m.Tasks, func(i, j int) bool { return m.Tasks[i].Path < m.Tasks[j].Path })
	data, err := yaml.Marshal(map[string]any{"kernel": map[string]string{"path": binary}, "tasks": native})
	if err != nil {
		return nil, nil, err
	}
	data = append([]byte("# Generated by wtc runtime render. Do not edit.\n"), data...)
	m.Digest = fmt.Sprintf("%x", sha256.Sum256(data))
	return m, data, nil
}

func (c *Context) renderRuntime() (*runtimeManifest, error) {
	if err := c.runtimePathsSafe(); err != nil {
		return nil, err
	}
	binary, err := c.runtimeBinary()
	if err != nil {
		return nil, err
	}
	m, data, err := c.runtimePlan(binary)
	if err != nil {
		return nil, err
	}
	old, err := c.runtimeManifest()
	if err != nil {
		return nil, err
	}
	if old != nil {
		m.Resources = old.Resources
		if err := runtimeRegular(filepath.Join(m.Root, "dekit.yaml")); err != nil {
			return nil, err
		}
		if err := runtimeOutputPathsSafe(old); err != nil {
			return nil, err
		}
		existing, err := os.ReadFile(filepath.Join(m.Root, "dekit.yaml"))
		if err != nil {
			return nil, err
		}
		if fmt.Sprintf("%x", sha256.Sum256(existing)) != old.Digest {
			return nil, fmt.Errorf("generated runtime config was edited; preserve changes before rendering")
		}
		if err := runtimeRegular(filepath.Join(m.Root, "dekit.yaml")); err != nil {
			return nil, err
		}
		if old.Digest == m.Digest {
			return old, nil
		}
		state, err := runtimeRunnerState(old)
		if err != nil {
			return nil, err
		}
		if state != "absent" {
			return nil, fmt.Errorf("runtime config changed; run wtc down before rendering")
		}
		// Clear down's saved task definitions before applying changed config.
		if _, err := runtimeCommand(old, false, "runner", "stop"); err != nil && !strings.Contains(err.Error(), "Runner is not running.") {
			return nil, err
		}
	} else if _, err := os.Lstat(m.Root); err == nil {
		return nil, fmt.Errorf("unmanaged runtime directory already exists")
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	complete := false
	defer func() {
		if old == nil && !complete {
			_ = os.RemoveAll(m.Root)
		}
	}()
	if err := os.MkdirAll(m.Root, 0700); err != nil {
		return nil, err
	}
	for _, task := range m.Tasks {
		if err := runtimeSafeParents(m.Root, task.Log); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(task.Log), 0700); err != nil {
			return nil, err
		}
	}
	if err := runtimeWrite(filepath.Join(m.Root, "dekit.yaml"), data); err != nil {
		return nil, err
	}
	manifest, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := runtimeWrite(filepath.Join(m.Root, "manifest.json"), manifest); err != nil {
		return nil, err
	}
	complete = true
	return m, nil
}

func runtimeSaveManifest(m *runtimeManifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return runtimeWrite(filepath.Join(m.Root, "manifest.json"), data)
}

func runtimeWrite(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".runtime-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func (c *Context) RenderRuntime(dryRun bool) (RuntimeResult, error) {
	result := RuntimeResult{Root: c.runtimeRoot(), Action: "render", Items: []RuntimeItem{}}
	var m *runtimeManifest
	var err error
	if dryRun {
		m, _, err = c.runtimePlan("dekit")
	} else {
		err = c.withRuntimeLock(func() error { m, err = c.renderRuntime(); return err })
	}
	if err == nil {
		for _, task := range m.Tasks {
			result.Items = append(result.Items, task.RuntimeItem)
		}
	}
	return result, err
}
