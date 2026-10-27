package wtc

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func readStatusWatcherProcess(pid int) (statusWatcherProcess, bool) {
	dir := filepath.Join("/proc", strconv.Itoa(pid))
	data, err := os.ReadFile(filepath.Join(dir, "stat"))
	if err != nil {
		return statusWatcherProcess{}, false
	}
	// comm may contain spaces and parentheses. Field 22 (starttime) is
	// relative to the final closing parenthesis, just before field 3.
	end := strings.LastIndexByte(string(data), ')')
	if end < 0 {
		return statusWatcherProcess{}, false
	}
	fields := strings.Fields(string(data[end+1:]))
	if len(fields) <= 19 {
		return statusWatcherProcess{}, false
	}
	args, err := os.ReadFile(filepath.Join(dir, "cmdline"))
	if err != nil || len(args) == 0 {
		return statusWatcherProcess{}, false
	}
	cwd, _ := os.Readlink(filepath.Join(dir, "cwd"))
	return statusWatcherProcess{start: fields[19], cwd: cwd, args: strings.TrimSuffix(string(args), "\x00")}, true
}
