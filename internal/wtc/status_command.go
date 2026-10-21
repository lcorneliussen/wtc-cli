package wtc

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Status refreshes must complete even when a forge or remote stops replying.
func statusCommand(timeout time.Duration, name string, args ...string) ([]byte, error) {
	return statusRun(timeout, (*exec.Cmd).CombinedOutput, name, args...)
}

func statusRun(timeout time.Duration, run func(*exec.Cmd) ([]byte, error), name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = 200 * time.Millisecond
	out, err := run(cmd)
	if ctx.Err() != nil {
		return nil, fmt.Errorf("%s timed out after %s", name, timeout)
	}
	if err != nil {
		detail := out
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			detail = exitErr.Stderr
		}
		return nil, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(detail)))
	}
	return out, nil
}

func statusGit(path string, args ...string) (string, error) {
	out, err := statusCommand(30*time.Second, "git", append([]string{"-C", path}, args...)...)
	return strings.TrimSpace(string(out)), err
}

// statusJSON reads forge JSON from stdout alone, so stderr cannot corrupt it.
func statusJSON(args ...string) ([]byte, error) {
	return statusRun(30*time.Second, forgeOutput, args[0], args[1:]...)
}
