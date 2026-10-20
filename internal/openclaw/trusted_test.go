package openclaw

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A program an agent could plant in a worktree's git config: it leaves a
// mark when it runs, which no command of wtc's may cause.
func plant(t *testing.T, name string) (script, mark string) {
	t.Helper()
	dir := t.TempDir()
	mark = filepath.Join(dir, name+".ran")
	script = filepath.Join(dir, name)
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntouch "+mark+"\ncat\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return script, mark
}

func ran(mark string) bool {
	_, err := os.Stat(mark)
	return err == nil
}

// Settings with a fixed name are overridden: wtc works, and nothing runs.
func TestGitIgnoresProgramsFromTheWorktreeConfig(t *testing.T) {
	c, git := managedWorktree(t, "/dev/null/unused")
	bare := git("remote", "get-url", "--push", "origin")
	git("remote", "set-url", "origin", bare)

	gpg, gpgMark := plant(t, "gpg")
	fsmonitor, fsmonitorMark := plant(t, "fsmonitor")
	hooks := t.TempDir()
	hook, hookMark := plant(t, "post-merge")
	if err := os.Rename(hook, filepath.Join(hooks, "post-merge")); err != nil {
		t.Fatal(err)
	}
	write(t, c, "notes.md", "one\n")
	git("add", ".")
	git("commit", "-qm", "work")
	for _, kv := range [][2]string{
		{"gpg.program", gpg}, {"gpg.ssh.program", gpg}, {"log.showSignature", "true"}, {"commit.gpgSign", "true"},
		{"core.fsmonitor", fsmonitor}, {"core.hooksPath", hooks},
	} {
		git("config", kv[0], kv[1])
	}
	c, err := OpenCheckout(c.Dir, "")
	if err != nil {
		t.Fatal(err)
	}
	write(t, c, "notes.md", "two\n")

	if s := CheckoutsStatus([]string{c.Dir}, false).Repos[0]; s.Error != "" || s.Dirty != 1 {
		t.Fatalf("status: %+v", s)
	}
	git("-c", "core.fsmonitor=false", "-c", "core.hooksPath=/dev/null", "checkout", "--", "notes.md")
	if _, err := c.PlanPublish(PublishOptions{}); err != nil {
		t.Fatalf("publish plan: %v", err)
	}
	other := t.TempDir()
	for _, args := range [][]string{{"clone", "-q", bare, other}, {"-C", other, "commit", "-q", "--allow-empty", "-m", "upstream"}, {"-C", other, "push", "-q", "origin", "main"}} {
		gitIn(t, args...)
	}
	if result, err := c.CatchUp(); err != nil || !result.Merged {
		t.Fatalf("catch-up: %+v %v", result, err)
	}
	for name, mark := range map[string]string{"gpg.program": gpgMark, "core.fsmonitor": fsmonitorMark, "a post-merge hook": hookMark} {
		if ran(mark) {
			t.Errorf("%s ran", name)
		}
	}
}

// Settings named by the agent, such as a filter driver, cannot be
// overridden by name: wtc refuses the checkout and says which key it is.
func TestGitRefusesAConfigThatCanRunPrograms(t *testing.T) {
	filter, filterMark := plant(t, "filter")
	for _, tc := range []struct {
		name, key, value string
		worktree         bool
	}{
		{"a clean filter", "filter.evil.clean", filter, false},
		{"an ssh command", "core.sshCommand", filter, false},
		{"an upload-pack", "remote.origin.uploadpack", filter, false},
		{"an include", "include.path", "/dev/null", false},
		{"a filter in the worktree's own config", "filter.evil.clean", filter, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, git := managedWorktree(t, "https://github.com/example-org/docs.git")
			write(t, c, ".gitattributes", "* filter=evil\n")
			git("add", ".gitattributes")
			git("commit", "-qm", "attributes")
			args := []string{"config"}
			if tc.worktree {
				git("config", "extensions.worktreeConfig", "true")
				args = append(args, "--worktree")
			}
			git(append(args, tc.key, tc.value)...)
			want := "sets " + strings.ToLower(tc.key)
			if s := CheckoutsStatus([]string{c.Dir}, false).Repos[0]; !strings.Contains(s.Error, want) {
				t.Errorf("status: %+v", s)
			}
			if _, err := c.PlanPublish(PublishOptions{}); err == nil || !strings.Contains(err.Error(), want) {
				t.Errorf("publish plan: %v", err)
			}
			if _, err := c.CatchUp(); err == nil || !strings.Contains(err.Error(), want) {
				t.Errorf("catch-up: %v", err)
			}
			if tc.worktree {
				if _, err := c.CatchUp(); err == nil || !strings.Contains(err.Error(), "--worktree --unset-all") {
					t.Errorf("the fix for a worktree key names --worktree: %v", err)
				}
			}
			if ran(filterMark) {
				t.Errorf("the planted program ran")
			}
		})
	}
}

func gitIn(t *testing.T, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
