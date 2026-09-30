package wtc

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Status refreshes must complete even when a forge or remote stops replying.
func statusCommand(timeout time.Duration, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = 200 * time.Millisecond
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("%s timed out after %s", name, timeout)
	}
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

func statusGit(path string, args ...string) (string, error) {
	out, err := statusCommand(30*time.Second, "git", append([]string{"-C", path}, args...)...)
	return strings.TrimSpace(string(out)), err
}

func statusJSON(args ...string) ([]byte, error) {
	return statusCommand(30*time.Second, args[0], args[1:]...)
}
