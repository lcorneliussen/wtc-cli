package wtc

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RuntimeItem is a repository-owned dekit task. Ready means its startup probe
// passed, not that it has continuous health monitoring.
type RuntimeItem struct {
	Collection string `json:"collection"`
	Repo       string `json:"repo"`
	Dir        string `json:"dir"`
	Path       string `json:"path"`
	Group      string `json:"group"`
	Kind       string `json:"kind"`
	Label      string `json:"label,omitempty"`
	URL        string `json:"url,omitempty"`
	State      string `json:"state"`
	ExitCode   *int   `json:"exit_code,omitempty"`
	Signal     *int   `json:"signal,omitempty"`
	Reason     string `json:"reason,omitempty"`
	Error      string `json:"error,omitempty"`
}

type runtimeTask struct {
	RuntimeItem
	Log       string   `json:"log"`
	Ready     bool     `json:"ready"`
	Autostart bool     `json:"autostart"`
	Deps      []string `json:"deps,omitempty"`
}

type runtimeResource struct {
	Dir   string `json:"dir"`
	Repo  string `json:"repo"`
	State string `json:"state"`
}

type runtimeManifest struct {
	Schema    int               `json:"schema"`
	Root      string            `json:"root"`
	Binary    string            `json:"binary"`
	Digest    string            `json:"digest"`
	Tasks     []runtimeTask     `json:"tasks"`
	Resources []runtimeResource `json:"resources,omitempty"`
}

type RuntimeResult struct {
	Root   string        `json:"root"`
	Action string        `json:"action"`
	Items  []RuntimeItem `json:"items"`
}

// The manifest records this root, so it is canonical: a collection reached
// through a symlinked parent must name the same runtime.
func (c *Context) runtimeRoot() string {
	collection := c.Collection
	if resolved, err := filepath.EvalSymlinks(collection); err == nil {
		collection = resolved
	}
	return filepath.Join(collection, ".wtc", "runtime")
}

func runtimeRegular(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("expected a regular file: %s", path)
	}
	return nil
}

func (c *Context) runtimePathsSafe() error {
	for _, path := range []string{filepath.Join(c.Collection, ".wtc"), c.runtimeRoot()} {
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("runtime directory must not be a symlink: %s", path)
		}
	}
	return nil
}

func (c *Context) runtimeManifest() (*runtimeManifest, error) {
	if err := c.runtimePathsSafe(); err != nil {
		return nil, err
	}
	path := filepath.Join(c.runtimeRoot(), "manifest.json")
	if err := runtimeRegular(path); os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m runtimeManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if m.Schema != 1 || m.Root != c.runtimeRoot() || !filepath.IsAbs(m.Binary) {
		return nil, fmt.Errorf("runtime manifest does not describe this collection")
	}
	for _, task := range m.Tasks {
		if task.Collection != filepath.Base(c.Collection) || filepath.Base(task.Dir) != task.Dir || !runtimeTaskPath(task.Dir) || !runtimeTaskPath(task.Path) ||
			!strings.HasPrefix(task.Path, task.Dir+"/") || task.Log != filepath.Join(m.Root, "logs", filepath.FromSlash(task.Path)+".log") {
			return nil, fmt.Errorf("invalid runtime task ownership")
		}
	}
	for _, resource := range m.Resources {
		if !runtimeTaskPath(resource.Dir) || filepath.Base(resource.Dir) != resource.Dir {
			return nil, fmt.Errorf("invalid runtime resource ownership")
		}
	}
	return &m, nil
}

// Refuse parent-directory symlinks for managed output paths.
func runtimeSafeParents(root, path string) error {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("runtime path escapes its root")
	}
	parent := filepath.Dir(path)
	for {
		info, err := os.Lstat(parent)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
			return fmt.Errorf("runtime output directory must not be a symlink: %s", parent)
		}
		if parent == root {
			break
		}
		parent = filepath.Dir(parent)
	}
	return nil
}

func runtimeOutputPathsSafe(m *runtimeManifest) error {
	for _, task := range m.Tasks {
		if err := runtimeSafeParents(m.Root, task.Log); err != nil {
			return err
		}
		if err := runtimeRegular(task.Log); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func runtimeTaskPath(path string) bool {
	if path == "" || strings.HasPrefix(path, "/") || strings.ContainsAny(path, ":\\*?[]{}@+\n\r") {
		return false
	}
	for _, p := range strings.Split(path, "/") {
		if p == "" || p == "." || p == ".." {
			return false
		}
	}
	return true
}

// RuntimeStatus only queries an existing runner. It never renders configuration,
// sources shell env, starts a runner, or executes repository hooks.
func (c *Context) RuntimeStatus() []RuntimeItem {
	m, err := c.runtimeManifest()
	if err != nil {
		return []RuntimeItem{{Collection: filepath.Base(c.Collection), Path: "runtime", State: "unknown", Error: err.Error()}}
	}
	if m == nil {
		return nil
	}
	return c.runtimeStatus(m)
}

func (c *Context) runtimeStatus(m *runtimeManifest) []RuntimeItem {
	items := make([]RuntimeItem, len(m.Tasks))
	for _, resource := range m.Resources {
		items = append(items, RuntimeItem{Collection: filepath.Base(c.Collection), Repo: resource.Repo, Dir: resource.Dir, Path: resource.Dir + "/@resources", Group: resource.Dir + "/resources", Kind: "resources", State: resource.State, Label: "resource hook"})
	}
	state, err := runtimeRunnerState(m)
	for i, task := range m.Tasks {
		items[i] = task.RuntimeItem
		items[i].State = state
		if err != nil {
			items[i].State = "unknown"
			items[i].Error = err.Error()
		}
	}
	if err != nil || state != "running" {
		return items
	}
	data, err := runtimeCommand(m, false, "ls")
	var response struct {
		Tasks []RuntimeItem `json:"tasks"`
	}
	if err == nil {
		err = json.Unmarshal(data, &response)
	}
	states := map[string]RuntimeItem{}
	for _, task := range response.Tasks {
		states[task.Path] = task
	}
	for i := range m.Tasks {
		item := &items[i]
		if err != nil {
			item.State = "unknown"
			item.Error = err.Error()
			continue
		}
		if task, ok := states[item.Path]; ok {
			item.State, item.ExitCode, item.Signal, item.Reason = task.State, task.ExitCode, task.Signal, task.Reason
		} else {
			item.State = "missing"
		}
	}
	return items
}

func runtimeSelect(m *runtimeManifest, target string) (string, []runtimeTask, error) {
	if target == "" {
		return "", m.Tasks, nil
	}
	if !runtimeTaskPath(target) {
		return "", nil, fmt.Errorf("expected a repository, task, or group path")
	}
	var tasks []runtimeTask
	exact := false
	for _, task := range m.Tasks {
		if task.Path == target {
			exact = true
			tasks = []runtimeTask{task}
			break
		}
		if task.Path != target && strings.HasPrefix(task.Path, target+"/") {
			tasks = append(tasks, task)
		}
	}
	if len(tasks) == 0 {
		return "", nil, fmt.Errorf("unknown runtime target %q", target)
	}
	if exact {
		return target, tasks, nil
	}
	return target + "/**", tasks, nil
}

func (c *Context) RuntimeLogPath(target string) (string, error) {
	m, err := c.runtimeManifest()
	if err != nil {
		return "", err
	}
	if m == nil {
		return "", fmt.Errorf("runtime is not configured")
	}
	_, tasks, err := runtimeSelect(m, target)
	if err != nil {
		return "", err
	}
	if len(tasks) != 1 {
		return "", fmt.Errorf("logs requires exactly one task path")
	}
	path := tasks[0].Log
	if err := runtimeSafeParents(m.Root, path); err != nil {
		return "", err
	}
	if err := runtimeRegular(path); err != nil {
		return "", err
	}
	return path, nil
}
