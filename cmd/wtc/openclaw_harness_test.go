package main

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lcorneliussen/wtc-cli/internal/wtc"
	"github.com/spf13/cobra"
)

const platformRegistry = `schema_version: 1
repos:
  - name: api
    remote: git@github.com:example-org/api.git
    default_ref: origin/develop
  - name: web
    remote: https://github.com/example-org/web.git
    default_ref: origin/main
`

// harnessGateway is a gateway state dir with clones of a harness and of the
// repositories its registry names, as projects.add and the plugin make them.
func harnessGateway(t *testing.T) (state string, git func(dir string, args ...string) string) {
	t.Helper()
	for name, value := range map[string]string{"GIT_AUTHOR_NAME": "T", "GIT_AUTHOR_EMAIL": "t@example.com",
		"GIT_COMMITTER_NAME": "T", "GIT_COMMITTER_EMAIL": "t@example.com", "GIT_CONFIG_GLOBAL": "/dev/null"} {
		t.Setenv(name, value)
	}
	state = t.TempDir()
	git = func(dir string, args ...string) string {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	clone := func(name, files string) {
		dir := filepath.Join(state, "repos", name)
		git(state, "init", "-q", "-b", "main", dir)
		if files != "" {
			if err := os.WriteFile(filepath.Join(dir, ".harness-repos.yml"), []byte(files), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		git(dir, "commit", "-q", "--allow-empty", "-m", "init")
		git(dir, "add", ".")
		git(dir, "commit", "-q", "--allow-empty", "-m", "registry")
		git(dir, "remote", "add", "origin", "https://github.com/example-org/"+name+".git")
		for _, b := range []string{"main", "develop"} {
			git(dir, "update-ref", "refs/remotes/origin/"+b, "HEAD")
		}
	}
	clone("platform-harness", platformRegistry)
	clone("ops-harness", "repos:\n  - name: infra\n    remote: https://github.com/example-org/infra.git\n")
	clone("api", "")
	clone("web", "")
	clone("infra", "")
	t.Setenv("OPENCLAW_STATE_DIR", state)
	t.Setenv("OPENCLAW_SHELL", "exec")
	t.Chdir(t.TempDir())
	return state, git
}

func withGatewayConfig(t *testing.T, config string) {
	t.Helper()
	saved := runOpenClaw
	t.Cleanup(func() { runOpenClaw = saved })
	runOpenClaw = func(args ...string) (string, error) {
		if strings.Join(args, " ") != "config get plugins.entries.wtc.config --json" {
			t.Fatalf("unexpected openclaw %v", args)
		}
		return config, nil
	}
}

type spawned struct {
	Repo string `json:"repo"`
	Args struct {
		ProjectID       string `json:"projectId"`
		ProjectGitURL   string `json:"projectGitUrl"`
		WorktreeName    string `json:"worktreeName"`
		WorktreeBaseRef string `json:"worktreeBaseRef"`
	} `json:"args"`
}

func addRepoPlan(t *testing.T, args ...string) ([]spawned, error) {
	t.Helper()
	var asJSON bool
	root := &cobra.Command{Use: "wtc", SilenceUsage: true, SilenceErrors: true}
	addAddRepoCommand(root, &asJSON)
	root.SetArgs(append([]string{"add-repo"}, args...))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	stdout, stderr := os.Stdout, os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	devnull, _ := os.Open(os.DevNull)
	os.Stdout, os.Stderr = w, devnull
	runErr := root.Execute()
	os.Stdout, os.Stderr = stdout, stderr
	w.Close()
	out, _ := io.ReadAll(r)
	if runErr != nil {
		return nil, runErr
	}
	var plan struct {
		Intents []spawned `json:"intents"`
	}
	if err := json.Unmarshal(out, &plan); err != nil {
		t.Fatalf("plan: %v\n%s", err, out)
	}
	return plan.Intents, nil
}

func summary(intents []spawned) string {
	var parts []string
	for _, in := range intents {
		parts = append(parts, in.Repo+"@"+in.Args.ProjectID+":"+in.Args.WorktreeBaseRef)
	}
	return strings.Join(parts, " ")
}

// With one harness configured, a collection works in it: its first add-repo
// brings the harness itself, and names resolve through its registry, with
// their default branches.
func TestAddRepoResolvesThroughTheHarness(t *testing.T) {
	state, git := harnessGateway(t)
	withGatewayConfig(t, `{"teamTailnet": false, "harnesses": [{"name": "platform", "remote": "https://github.com/example-org/platform-harness"}]}`)

	intents, err := addRepoPlan(t, "api", "--worktree", "12-paging")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := summary(intents), "platform-harness@platform-harness:origin/main api@api:origin/develop"; got != want {
		t.Errorf("intents %s, want %s", got, want)
	}
	if got := git(filepath.Join(state, "repos", "api"), "config", "branch.openclaw/12-paging.wtcbase"); got != "develop" {
		t.Errorf("recorded base of api: %q", got)
	}
	folder := filepath.Join(state, "wtc-collections", "12-paging")
	if name, remote, err := wtc.GatewayHarness(folder); err != nil || name != "platform" || !strings.Contains(remote, "platform-harness") {
		t.Errorf("recorded harness %q %q %v", name, remote, err)
	}

	// Once the collection has its worktree of the harness, the registry is
	// read from there: the task may change it.
	wt := filepath.Join(state, "worktrees", "a1", "12-paging")
	git(filepath.Join(state, "repos", "platform-harness"), "worktree", "add", "-q", "-b", "openclaw/12-paging", wt)
	if err := os.WriteFile(filepath.Join(wt, ".harness-repos.yml"), []byte(platformRegistry+"  - name: docs\n    remote: https://github.com/example-org/web.git\n    default_ref: origin/develop\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	intents, err = addRepoPlan(t, "docs", "--worktree", "12-paging")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := summary(intents), "web@web:origin/develop"; got != want {
		t.Errorf("from the harness worktree: %s, want %s", got, want)
	}
	if h := gatewayHarness(folder, []string{wt}); h == nil || h.Name != "platform" || h.Path != wt {
		t.Errorf("status shows the harness: %+v", h)
	}
	// --base still wins over the registry's default branch.
	if intents, err = addRepoPlan(t, "api", "--worktree", "12-paging", "--base", "main"); err != nil || summary(intents) != "api@api:origin/main" {
		t.Errorf("--base: %s %v", summary(intents), err)
	}
	// A collection has one harness.
	if _, err := addRepoPlan(t, "api", "--worktree", "12-paging", "--harness", "ops"); err == nil || !strings.Contains(err.Error(), "uses harness platform") {
		t.Errorf("another harness: %v", err)
	}
}

// With several harnesses and no default, the choice has to be clear from
// the names; otherwise the agent is told to ask.
func TestAddRepoAsksWhichHarnessWhenItIsNotClear(t *testing.T) {
	_, _ = harnessGateway(t)
	withGatewayConfig(t, `{"harnesses": [
		{"name": "platform", "remote": "https://github.com/example-org/platform-harness"},
		{"name": "ops", "remote": "git@github.com:example-org/ops-harness.git", "ref": "develop"}]}`)

	intents, err := addRepoPlan(t, "infra", "--worktree", "7-dns")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := summary(intents), "ops-harness@ops-harness:origin/develop infra@infra:"; got != want {
		t.Errorf("only ops lists infra: %s, want %s", got, want)
	}
	for _, args := range [][]string{{"example-org/api", "--worktree", "8-x"}, {"api", "infra", "--worktree", "9-x"}} {
		if _, err := addRepoPlan(t, args...); err == nil || !strings.Contains(err.Error(), "Ask the person which harness") || !strings.Contains(err.Error(), "platform, ops") {
			t.Errorf("%v: %v", args, err)
		}
	}
	if intents, err := addRepoPlan(t, "api", "--worktree", "10-x", "--harness", "none"); err != nil || summary(intents) != "api@api:" {
		t.Errorf("--harness none: %s %v", summary(intents), err)
	}
	if intents, err := addRepoPlan(t, "api", "--worktree", "11-x", "--harness", "platform"); err != nil || !strings.HasPrefix(summary(intents), "platform-harness@") {
		t.Errorf("--harness platform: %s %v", summary(intents), err)
	}
	if _, err := addRepoPlan(t, "api", "--worktree", "12-x", "--harness", "nope"); err == nil || !strings.Contains(err.Error(), "no harness nope") {
		t.Errorf("unknown harness: %v", err)
	}
}

// Without harnesses, or without the openclaw CLI, add-repo is as before.
func TestAddRepoWithoutHarnesses(t *testing.T) {
	_, _ = harnessGateway(t)
	saved := runOpenClaw
	t.Cleanup(func() { runOpenClaw = saved })
	for _, run := range []func(...string) (string, error){
		func(...string) (string, error) { return "", errNoOpenClaw },
		func(...string) (string, error) {
			return `{"ok": false, "error": {"message": "Config path is valid but unset: plugins.entries.wtc.config."}}`, &exec.ExitError{}
		},
		func(...string) (string, error) { return `{"teamTailnet": true}`, nil },
	} {
		runOpenClaw = run
		intents, err := addRepoPlan(t, "api", "--worktree", "5-x")
		if err != nil || summary(intents) != "api@api:" {
			t.Errorf("%s %v", summary(intents), err)
		}
	}
	runOpenClaw = func(...string) (string, error) { return "boom", &exec.ExitError{} }
	if _, err := addRepoPlan(t, "api", "--worktree", "5-x"); err == nil || !strings.Contains(err.Error(), "reading plugins.entries.wtc.config") {
		t.Errorf("an unreadable config: %v", err)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("OPENCLAW_STATE_DIR"), "wtc-collections", "5-x")); !os.IsNotExist(err) {
		t.Errorf("an unreadable config made a folder: %v", err)
	}
	runOpenClaw = func(...string) (string, error) {
		return `{"harnesses": [{"name": "x", "remote": "https://example.org/x.git"}]}`, nil
	}
	if _, err := addRepoPlan(t, "api", "--worktree", "5-x"); err == nil || !strings.Contains(err.Error(), "not a GitHub or Bitbucket") {
		t.Errorf("a bad config: %v", err)
	}
}

// A URL resolves through the registry too, the harness is added once even
// when named, and an add-repo that fails binds the collection to nothing.
func TestAddRepoHarnessEdgeCases(t *testing.T) {
	state, _ := harnessGateway(t)
	withGatewayConfig(t, `{"harnesses": [{"name": "platform", "remote": "https://github.com/example-org/platform-harness"}]}`)
	intents, err := addRepoPlan(t, "git@github.com:example-org/api.git", "example-org/platform-harness", "--worktree", "3-x")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := summary(intents), "platform-harness@platform-harness:origin/main api@api:origin/develop"; got != want {
		t.Errorf("intents %s, want %s", got, want)
	}

	// A repository that is not on the gateway: the plan fails, nothing is
	// recorded, and the collection can still choose.
	if _, err := addRepoPlan(t, "example-org/elsewhere", "--worktree", "4-x"); err == nil || !strings.Contains(err.Error(), "not on the gateway yet") {
		t.Fatalf("not on the gateway: %v", err)
	}
	if name, _, err := wtc.GatewayHarness(filepath.Join(state, "wtc-collections", "4-x")); err != nil || name != "" {
		t.Errorf("a failed add-repo recorded %q %v", name, err)
	}
	if _, err := addRepoPlan(t, "api", "--worktree", "4-x", "--harness", "none"); err != nil {
		t.Errorf("choosing after a failed add-repo: %v", err)
	}

	// A harness that is not on the gateway, or has no registry.
	withGatewayConfig(t, `{"harnesses": [{"name": "gone", "remote": "https://github.com/example-org/gone-harness"}]}`)
	if _, err := addRepoPlan(t, "api", "--worktree", "5-x"); err == nil || !strings.Contains(err.Error(), "harness gone is not on the gateway yet") {
		t.Errorf("harness not on the gateway: %v", err)
	}
	withGatewayConfig(t, `{"harnesses": [{"name": "web", "remote": "https://github.com/example-org/web"}]}`)
	if _, err := addRepoPlan(t, "api", "--worktree", "6-x"); err == nil || !strings.Contains(err.Error(), "has no .harness-repos.yml") {
		t.Errorf("harness without a registry: %v", err)
	}
	for _, w := range []string{"5-x", "6-x"} {
		if _, err := os.Stat(filepath.Join(state, "wtc-collections", w)); !os.IsNotExist(err) {
			t.Errorf("%s: a folder was made: %v", w, err)
		}
	}
}

// An invalid worktree name is refused before anything is recorded, with or
// without harnesses.
func TestAddRepoRefusesABadWorktreeNameFirst(t *testing.T) {
	state, _ := harnessGateway(t)
	for _, config := range []string{`{}`, `{"harnesses": [{"name": "platform", "remote": "https://github.com/example-org/platform-harness"}]}`} {
		withGatewayConfig(t, config)
		if _, err := addRepoPlan(t, "api", "--worktree", "Bad/Name", "--base", "develop"); err == nil || !strings.Contains(err.Error(), "not a valid OpenClaw worktree name") {
			t.Errorf("%s: %v", config, err)
		}
		if out, _ := exec.Command("git", "-C", filepath.Join(state, "repos", "api"), "config", "--get-regexp", "wtcbase").Output(); len(out) != 0 {
			t.Errorf("%s: a base was recorded: %s", config, out)
		}
		if entries, _ := os.ReadDir(filepath.Join(state, "wtc-collections")); len(entries) != 0 {
			t.Errorf("%s: a folder was made", config)
		}
	}
}
