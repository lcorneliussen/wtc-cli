package wtc

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Native views stop cooperatively. Legacy shell wrappers need a fallback,
// restricted to a recognized command and a verified collection scope.
func stopRetiredStatusWatchers(target string) error {
	if err := stopRegisteredStatusWatchers(target); err != nil {
		return err
	}
	output, err := exec.Command("ps", "ax", "-o", "pid=,args=").Output()
	if err != nil {
		return nil
	}
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || !possibleStatusWatcher(strings.Join(fields[1:], " ")) {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil || pid <= 1 || pid == os.Getpid() {
			continue
		}
		stopRetiredStatusWatcher(target, pid, readStatusWatcherProcess, func(pid int) {
			if process, err := os.FindProcess(pid); err == nil {
				_ = process.Kill()
			}
		})
	}
	return nil
}

func stopRetiredStatusWatcher(target string, pid int, read func(int) (statusWatcherProcess, bool), kill func(int)) {
	before, ok := read(pid)
	if !ok || !retiredStatusWatcherCommand(target, before.cwd, before.args) {
		return
	}
	// Re-read immediately before signaling: a changed start time, command,
	// or working directory invalidates the candidate rather than its PID.
	after, ok := read(pid)
	if ok && before == after {
		kill(pid)
	}
}

type statusWatcherProcess struct {
	start, cwd, args string
}

func possibleStatusWatcher(command string) bool {
	// ps is only a coarse inventory filter; exact NUL-delimited argv is read
	// from the OS before a candidate can be signaled, including paths with spaces.
	if strings.Contains(command, "/wtc-status-tui.sh") || strings.Contains(command, "/wtc-status.sh") {
		return true
	}
	argv := statusWatcherArgs(command)
	if len(argv) == 0 {
		return false
	}
	switch filepath.Base(argv[0]) {
	case "wtc-status-tui.sh", "wtc-status.sh":
		return true
	}
	return false
}

func statusWatcherArgs(command string) []string {
	argv := strings.Fields(command)
	if strings.Contains(command, "\x00") {
		argv = strings.Split(command, "\x00")
	}
	if len(argv) > 0 {
		if name := filepath.Base(argv[0]); name == "bash" || name == "sh" || name == "zsh" {
			argv = argv[1:]
		}
	}
	return argv
}

func retiredStatusWatcherCommand(target, cwd, command string) bool {
	argv := statusWatcherArgs(command)
	if len(argv) == 0 {
		return false
	}
	resolved, err := filepath.EvalSymlinks(target)
	if err != nil {
		return false
	}
	script := argv[0]
	if !filepath.IsAbs(script) && cwd != "" {
		script = filepath.Join(cwd, script)
	}
	for _, name := range []string{"wtc-status-tui.sh", "wtc-status.sh"} {
		if filepath.Clean(script) == filepath.Join(target, "harness", "tools", name) ||
			filepath.Clean(script) == filepath.Join(resolved, "harness", "tools", name) {
			if name == "wtc-status-tui.sh" {
				return true
			}
			for _, arg := range argv[1:] {
				if arg == "--tui" {
					return true
				}
			}
		}
	}
	return false
}
