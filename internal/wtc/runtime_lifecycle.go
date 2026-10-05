package wtc

import (
	"context"
	"encoding/json"
	"fmt"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func runtimeDependencies(m *runtimeManifest, selected []runtimeTask) []runtimeTask {
	byPath := map[string]runtimeTask{}
	seen := map[string]bool{}
	result := []runtimeTask{}
	for _, task := range m.Tasks {
		byPath[task.Path] = task
	}
	var visit func(runtimeTask)
	visit = func(task runtimeTask) {
		if seen[task.Path] {
			return
		}
		seen[task.Path] = true
		for _, dep := range task.Deps {
			visit(byPath[dep])
		}
		result = append(result, task)
	}
	for _, task := range selected {
		visit(task)
	}
	return result
}

func (c *Context) withRuntimeLock(fn func() error) error {
	path := filepath.Join(c.Collection, ".wtc-runtime.lock")
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("runtime lock must be a regular file")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	deadline := time.Now().Add(10 * time.Second)
	for {
		err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			break
		}
		if err != unix.EWOULDBLOCK || time.Now().After(deadline) {
			return fmt.Errorf("runtime is busy: %w", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	defer unix.Flock(int(f.Fd()), unix.LOCK_UN)
	return fn()
}

func runtimeCommand(m *runtimeManifest, sourceEnv bool, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	argv := append([]string{"-C", m.Root, "--json"}, args...)
	cmd := exec.CommandContext(ctx, m.Binary, argv...)
	if sourceEnv {
		collection := filepath.Dir(filepath.Dir(m.Root))
		args := append([]string{"-c", `set -ea; [ ! -f "$1" ] || . "$1"; [ ! -f "$2" ] || . "$2"; shift 2; exec "$@"`, "wtc-runtime", filepath.Join(collection, ".env.collection"), filepath.Join(collection, ".env.collection.local"), m.Binary}, argv...)
		cmd = exec.CommandContext(ctx, "/bin/sh", args...)
	}
	cmd.Dir = m.Root
	cmd.WaitDelay = 200 * time.Millisecond
	var stdout, stderr statusLimitedBuffer
	stdout.limit = 1 << 20
	stderr.limit = 4096
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("dekit %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

func runtimeRunnerState(m *runtimeManifest) (string, error) {
	data, err := runtimeCommand(m, false, "runner", "status")
	if err != nil {
		return "", err
	}
	var state struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return "", err
	}
	if state.Status == "" {
		return "", fmt.Errorf("dekit returned no runner state")
	}
	return state.Status, nil
}

func (c *Context) runtimeHooks(m *runtimeManifest, tasks []runtimeTask, hook string) error {
	seen := map[string]bool{}
	for _, task := range tasks {
		if task.Dir == "" {
			return fmt.Errorf("invalid runtime hook owner")
		}
		if seen[task.Dir] {
			continue
		}
		seen[task.Dir] = true
		worktree := filepath.Join(c.Collection, task.Dir)
		path := filepath.Join(worktree, ".harness", "runtime-"+hook+".sh")
		if err := runtimeRegular(path); os.IsNotExist(err) {
			if hook == "teardown" {
				for _, resource := range m.Resources {
					if resource.Dir == task.Dir {
						return fmt.Errorf("%s: resource teardown hook is missing", task.Dir)
					}
				}
			}
			continue
		} else if err != nil {
			return err
		}
		// Record ownership before a provision attempt, including partial failure.
		resourceIndex := -1
		for i := range m.Resources {
			if m.Resources[i].Dir == task.Dir {
				resourceIndex = i
				break
			}
		}
		if hook == "provision" {
			teardown := filepath.Join(worktree, ".harness", "runtime-teardown.sh")
			if err := runtimeRegular(teardown); err != nil {
				return fmt.Errorf("%s: provision requires a teardown hook: %w", task.Dir, err)
			}
			info, err := os.Stat(teardown)
			if err != nil {
				return err
			}
			if info.Mode()&0111 == 0 {
				return fmt.Errorf("runtime teardown hook must be executable: %s", teardown)
			}
			if resourceIndex < 0 {
				m.Resources = append(m.Resources, runtimeResource{Dir: task.Dir, Repo: task.Repo})
				resourceIndex = len(m.Resources) - 1
			}
			m.Resources[resourceIndex].State = "provision-attempted"
			if err := runtimeSaveManifest(m); err != nil {
				return err
			}
		}
		info, _ := os.Stat(path)
		if info.Mode()&0111 == 0 {
			return fmt.Errorf("runtime hook must be executable: %s", path)
		}
		cmd := exec.Command("/bin/sh", "-c", `set -ea; [ ! -f "$1" ] || . "$1"; [ ! -f "$2" ] || . "$2"; shift 2; exec "$@"`, "wtc-runtime-hook", filepath.Join(c.Collection, ".env.collection"), filepath.Join(c.Collection, ".env.collection.local"), path)
		cmd.Dir = worktree
		logPath := filepath.Join(m.Root, "hook-logs", task.Dir, hook+".log")
		if err := runtimeSafeParents(m.Root, logPath); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(logPath), 0700); err != nil {
			return err
		}
		log, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY|unix.O_NOFOLLOW, 0600)
		if err != nil {
			return err
		}
		cmd.Stdout, cmd.Stderr = io.MultiWriter(os.Stderr, log), io.MultiWriter(os.Stderr, log)
		runErr := cmd.Run()
		log.Close()
		if resourceIndex >= 0 {
			if hook == "provision" {
				if runErr == nil {
					m.Resources[resourceIndex].State = "provision-hook-completed"
				}
			} else if runErr == nil {
				m.Resources = append(m.Resources[:resourceIndex], m.Resources[resourceIndex+1:]...)
			} else {
				m.Resources[resourceIndex].State = "teardown-failed"
			}
			if err := runtimeSaveManifest(m); err != nil {
				return err
			}
		}
		if runErr != nil {
			err := runErr
			return fmt.Errorf("%s runtime %s failed: %w", task.Dir, hook, err)
		}
	}
	return nil
}

func (c *Context) RuntimeAction(action, target string, wait time.Duration) (RuntimeResult, error) {
	result := RuntimeResult{Root: c.runtimeRoot(), Action: action, Items: []RuntimeItem{}}
	err := c.withRuntimeLock(func() error {
		m, err := c.runtimeManifest()
		if err != nil {
			return err
		}
		if action == "up" || action == "provision" {
			m, err = c.renderRuntime()
			if err != nil {
				return err
			}
		}
		if m == nil {
			return fmt.Errorf("runtime is not configured; run wtc up")
		}
		if action != "down" {
			if err := runtimeOutputPathsSafe(m); err != nil {
				return err
			}
		}
		selector, tasks, err := runtimeSelect(m, target)
		if err != nil {
			return err
		}
		switch action {
		case "up", "provision":
			if err := c.runtimeHooks(m, runtimeDependencies(m, tasks), "provision"); err != nil {
				return err
			}
			if action == "provision" {
				return nil
			}
			args := []string{"up"}
			if selector != "" {
				args = []string{"start", selector}
			}
			if _, err := runtimeCommand(m, true, args...); err != nil {
				return err
			}
		case "down":
			state, err := runtimeRunnerState(m)
			if err != nil {
				return err
			}
			if state == "absent" {
				return nil
			}
			args := []string{"down"}
			if selector != "" {
				args = []string{"veto", selector}
			}
			if _, err := runtimeCommand(m, false, args...); err != nil {
				return err
			}
		case "restart":
			state, err := runtimeRunnerState(m)
			if err != nil {
				return err
			}
			if state != "running" {
				return fmt.Errorf("runner is %s; run wtc up first", state)
			}
			if selector == "" {
				selector = "**"
			}
			if _, err := runtimeCommand(m, false, "restart", selector); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown runtime action %q", action)
		}
		if action == "down" && target == "" {
			deadline := time.Now().Add(30 * time.Second)
			for {
				state, err := runtimeRunnerState(m)
				if err != nil {
					return err
				}
				if state == "absent" {
					break
				}
				if time.Now().After(deadline) {
					return fmt.Errorf("runtime shutdown not confirmed: %s", state)
				}
				time.Sleep(100 * time.Millisecond)
			}
		}
		if action == "down" && target != "" {
			stopped := map[string]bool{}
			for _, task := range tasks {
				stopped[task.Path] = true
			}
			for changed := true; changed; {
				changed = false
				for _, task := range m.Tasks {
					for _, dep := range task.Deps {
						if stopped[dep] && !stopped[task.Path] {
							stopped[task.Path] = true
							changed = true
						}
					}
				}
			}
			deadline := time.Now().Add(30 * time.Second)
			for {
				complete := true
				for _, item := range c.runtimeStatus(m) {
					if !stopped[item.Path] {
						continue
					}
					switch item.State {
					case "idle", "exited", "done", "absent":
					case "unknown", "missing":
						return fmt.Errorf("%s: shutdown not confirmed: %s", item.Path, item.State)
					default:
						complete = false
					}
				}
				if complete {
					break
				}
				if time.Now().After(deadline) {
					return fmt.Errorf("target runtime shutdown not confirmed")
				}
				time.Sleep(100 * time.Millisecond)
			}
		}
		if wait > 0 && (action == "up" || action == "restart") {
			wanted := map[string]bool{}
			for _, task := range tasks {
				if target != "" || task.Autostart || action == "restart" {
					wanted[task.Path] = true
				}
			}
			deadline := time.Now().Add(wait)
			for {
				items := c.RuntimeStatus()
				ready := true
				observed := map[string]bool{}
				for _, item := range items {
					if !wanted[item.Path] {
						continue
					}
					observed[item.Path] = true
					switch item.State {
					case "ready", "done":
					case "running":
						for _, task := range tasks {
							if task.Path == item.Path && (task.Ready || task.Kind == "job") {
								ready = false
							}
						}
					case "exited", "unknown", "missing", "failed", "absent":
						return fmt.Errorf("%s: %s %s %s", item.Path, item.State, item.Reason, item.Error)
					default:
						ready = false
					}
				}
				for path := range wanted {
					if !observed[path] {
						return fmt.Errorf("%s: runtime state unavailable", path)
					}
				}
				if ready {
					break
				}
				if time.Now().After(deadline) {
					return fmt.Errorf("runtime readiness timed out after %s; inspect wtc status and wtc logs", wait)
				}
				time.Sleep(100 * time.Millisecond)
			}
		}
		return nil
	})
	result.Items = c.RuntimeStatus()
	return result, err
}

// RetireRuntime uses the target's manifest even if runtime opt-in was removed.
// An unconfirmed shutdown or failed teardown keeps the worktrees and record.
func (c *Context) RetireRuntime() error {
	if _, err := os.Lstat(filepath.Join(c.runtimeRoot(), "manifest.json")); os.IsNotExist(err) {
		return nil
	}
	return c.withRuntimeLock(c.retireRuntime)
}

func (c *Context) retireRuntime() error {
	m, err := c.runtimeManifest()
	if err != nil {
		return err
	}
	if m == nil {
		return nil
	}
	state, err := runtimeRunnerState(m)
	if err != nil {
		return err
	}
	_, stopErr := runtimeCommand(m, false, "runner", "stop")
	if stopErr != nil && !(state == "absent" && strings.Contains(stopErr.Error(), "Runner is not running.")) {
		return stopErr
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		state, err = runtimeRunnerState(m)
		if err != nil {
			return err
		}
		if state == "absent" {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("runtime shutdown not confirmed: %s", state)
		}
		time.Sleep(100 * time.Millisecond)
	}
	tasks := append([]runtimeTask(nil), m.Tasks...)
	for _, resource := range m.Resources {
		tasks = append(tasks, runtimeTask{RuntimeItem: RuntimeItem{Dir: resource.Dir, Repo: resource.Repo}})
	}
	return c.runtimeHooks(m, tasks, "teardown")
}
