package openclaw

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectHost(t *testing.T) {
	for _, tc := range []struct {
		want HostKind
		env  map[string]string
	}{
		{HostOpenClaw, map[string]string{"OPENCLAW_SHELL": "exec", "HERDR_SESSION": "s"}},
		// Codex's bash tool and the Control UI terminal: the gateway's env only.
		{HostOpenClaw, map[string]string{"OPENCLAW_STATE_DIR": "/home/node/.openclaw", "CODEX_HOME": "/x"}},
		{HostHerdr, map[string]string{"HERDR_WORKSPACE_ID": "w"}},
		{HostTerminal, map[string]string{"OPENCLAW_SHELL": "", "OPENCLAW_HOME": "/home/node"}},
	} {
		if got := DetectHost(func(k string) string { return tc.env[k] }); got != tc.want {
			t.Errorf("%v: got %s, want %s", tc.env, got, tc.want)
		}
	}
}

func TestManagedCollections(t *testing.T) {
	state := t.TempDir()
	for _, dir := range []string{"p1/wtc-smoke", "p2/wtc-smoke", "p1/other"} {
		if err := os.MkdirAll(filepath.Join(state, "worktrees", dir), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(state, "worktrees", dir, ".git"), []byte("gitdir: x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Not a worktree: no .git.
	_ = os.MkdirAll(filepath.Join(state, "worktrees", "p3", "stray"), 0o755)
	got, err := ManagedCollections(state)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || len(got["wtc-smoke"]) != 2 || len(got["other"]) != 1 {
		t.Fatalf("collections %v", got)
	}
}

// managedWorktree is what the gateway hands a child session: a worktree of a
// clone, on openclaw/<name>, with origin on originURL and a remote main.
func managedWorktree(t *testing.T, originURL string) (*Checkout, func(args ...string) string) {
	t.Helper()
	// Every git command in the test, wtc's own included, commits as this
	// identity: a CI machine has none configured.
	for name, value := range map[string]string{"GIT_AUTHOR_NAME": "T", "GIT_AUTHOR_EMAIL": "t@example.com",
		"GIT_COMMITTER_NAME": "T", "GIT_COMMITTER_EMAIL": "t@example.com", "GIT_CONFIG_GLOBAL": "/dev/null"} {
		t.Setenv(name, value)
	}
	root := t.TempDir()
	run := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=T", "GIT_AUTHOR_EMAIL=t@example.com",
			"GIT_COMMITTER_NAME=T", "GIT_COMMITTER_EMAIL=t@example.com", "GIT_CONFIG_GLOBAL=/dev/null")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return strings.TrimSpace(string(out))
	}
	bare, repo, wt := filepath.Join(root, "origin.git"), filepath.Join(root, "docs"), filepath.Join(root, "72-add-thing")
	run(root, "init", "-q", "--bare", "-b", "main", bare)
	run(root, "clone", "-q", bare, repo)
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("docs\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(repo, "add", ".")
	run(repo, "commit", "-qm", "init")
	run(repo, "push", "-q", "origin", "HEAD:main")
	run(repo, "remote", "set-head", "origin", "main")
	run(repo, "remote", "set-url", "origin", originURL)
	run(repo, "remote", "set-url", "--push", "origin", bare)
	run(repo, "worktree", "add", "-q", "-b", "openclaw/72-add-thing", wt)
	c, err := OpenCheckout(wt, "")
	if err != nil {
		t.Fatal(err)
	}
	return c, func(args ...string) string { return run(wt, args...) }
}

func write(t *testing.T, c *Checkout, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(c.Dir, name)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(c.Dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestOpenCheckout(t *testing.T) {
	c, git := managedWorktree(t, "https://github.com/example-org/docs.git")
	if c.Worktree != "72-add-thing" || c.Dest != "main" || c.Repo != "docs" || c.Origin.Forge != ForgeGitHub {
		t.Fatalf("checkout %+v", c)
	}
	if got := CurrentWorktree(c.Dir); got != "72-add-thing" {
		t.Fatalf("current worktree %q", got)
	}
	git("switch", "-qc", "feature")
	if _, err := OpenCheckout(c.Dir, ""); err == nil || !strings.Contains(err.Error(), "managed worktree") {
		t.Fatalf("a branch the gateway does not own was accepted: %v", err)
	}
	if _, err := OpenCheckout(t.TempDir(), ""); err == nil {
		t.Fatal("a directory that is no checkout was accepted")
	}
}

func TestPlanPublishOnGitHub(t *testing.T) {
	c, git := managedWorktree(t, "https://github.com/example-org/docs.git")
	if _, err := c.PlanPublish(PublishOptions{}); err == nil || !strings.Contains(err.Error(), "nothing to publish") {
		t.Fatalf("an unchanged worktree: %v", err)
	}
	write(t, c, "notes.md", "one\n")
	write(t, c, ".github/workflows/ci.yml", "on: push\n")
	plan, err := c.PlanPublish(PublishOptions{Title: "Add notes"})
	if err != nil {
		t.Fatal(err)
	}
	in := plan.Intents[0]
	if in.Tool != ToolGitHubPublish || in.Actor != ActorRepoSession || in.Repo != "docs" || in.Args.(PublishArgs).Title != "Add notes" {
		t.Fatalf("intent %+v", in)
	}
	if strings.Join(plan.Pending, " ") != ".github/workflows/ci.yml notes.md" {
		t.Fatalf("pending %v", plan.Pending)
	}
	if len(plan.Warnings) != 2 || !strings.Contains(plan.Warnings[1], "workflow") {
		t.Fatalf("warnings %v", plan.Warnings)
	}
	if got := git("status", "--porcelain"); !strings.Contains(got, "notes.md") {
		t.Fatalf("planning changed the checkout:\n%s", got)
	}
}

func TestPlanPublishRefusesWhatIsNotSafe(t *testing.T) {
	t.Run("credentials lying around", func(t *testing.T) {
		c, _ := managedWorktree(t, "https://github.com/example-org/docs.git")
		write(t, c, "notes.md", "one\n")
		write(t, c, "config/.env.local", "TOKEN=x\n")
		write(t, c, ".env.example", "TOKEN=\n")
		_, err := c.PlanPublish(PublishOptions{})
		if err == nil || !strings.Contains(err.Error(), "config/.env.local") || strings.Contains(err.Error(), ".env.example") {
			t.Fatalf("got %v", err)
		}
		if _, err := c.PlanPublish(PublishOptions{AllowSensitive: true}); err != nil {
			t.Fatalf("--allow-sensitive: %v", err)
		}
	})
	t.Run("rewritten published history", func(t *testing.T) {
		c, git := managedWorktree(t, "https://github.com/example-org/docs.git")
		write(t, c, "notes.md", "one\n")
		git("add", ".")
		git("commit", "-qm", "work")
		git("push", "-q", "origin", "openclaw/72-add-thing")
		git("fetch", "-q", strings.TrimSpace(git("remote", "get-url", "--push", "origin")), "+refs/heads/*:refs/remotes/origin/*")
		write(t, c, "notes.md", "two\n")
		if _, err := c.PlanPublish(PublishOptions{}); err != nil {
			t.Fatalf("work on top of the published head: %v", err)
		}
		git("commit", "-qa", "--amend", "-m", "rewritten")
		if _, err := c.PlanPublish(PublishOptions{}); err == nil || !strings.Contains(err.Error(), "rewritten") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("rewritten history the gateway published", func(t *testing.T) {
		// The gateway pushes without leaving a remote-tracking ref; what it
		// leaves is a marked commit on the branch.
		c, git := managedWorktree(t, "https://github.com/example-org/docs.git")
		write(t, c, "notes.md", "one\n")
		git("add", ".")
		git("commit", "-qm", "Publish openclaw/72-add-thing\n\nOpenClaw-Publication: 53a47a99")
		write(t, c, "notes.md", "two\n")
		if _, err := c.PlanPublish(PublishOptions{}); err != nil {
			t.Fatalf("work on top of the published head: %v", err)
		}
		git("commit", "-qa", "--amend", "-m", "Tidied")
		write(t, c, "notes.md", "three\n")
		if _, err := c.PlanPublish(PublishOptions{}); err == nil || !strings.Contains(err.Error(), "rewritten") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("an unfinished merge", func(t *testing.T) {
		c, git := managedWorktree(t, "https://github.com/example-org/docs.git")
		write(t, c, "README.md", "ours\n")
		git("commit", "-qam", "ours")
		git("switch", "-qc", "other", "origin/main")
		write(t, c, "README.md", "theirs\n")
		git("commit", "-qam", "theirs")
		git("switch", "-q", "openclaw/72-add-thing")
		exec.Command("git", "-C", c.Dir, "merge", "other").Run()
		if _, err := c.PlanPublish(PublishOptions{}); err == nil || !strings.Contains(err.Error(), "merge") {
			t.Fatalf("got %v", err)
		}
	})
}

func TestPlanPublishOffGitHub(t *testing.T) {
	c, git := managedWorktree(t, "git@bitbucket.org:example-ws/docs.git")
	write(t, c, "notes.md", "one\n")
	if _, err := c.PlanPublish(PublishOptions{}); err == nil || !strings.Contains(err.Error(), "commit your work first") {
		t.Fatalf("uncommitted work on Bitbucket: %v", err)
	}
	git("add", ".")
	git("commit", "-qm", "work")
	plan, err := c.PlanPublish(PublishOptions{Title: "Add notes"})
	if err != nil {
		t.Fatal(err)
	}
	in := plan.Intents[0]
	want := PullRequestArgs{Owner: "example-ws", Repo: "docs", Source: "openclaw/72-add-thing", Dest: "main", Title: "Add notes", Draft: true}
	if in.Tool != "" || in.Forge != ForgeBitbucket || in.Args != want {
		t.Fatalf("intent %+v", in)
	}
}

func TestCatchUpMergesAndNeverPushes(t *testing.T) {
	c, git := managedWorktree(t, "/dev/null/unused")
	bare := git("remote", "get-url", "--push", "origin")
	git("remote", "set-url", "origin", bare)
	c, err := OpenCheckout(c.Dir, "")
	if err != nil {
		t.Fatal(err)
	}
	advance := func(file, content string) {
		t.Helper()
		other := t.TempDir()
		for _, args := range [][]string{{"clone", "-q", bare, other}, nil, {"-C", other, "add", "."},
			{"-C", other, "-c", "user.name=T", "-c", "user.email=t@example.com", "commit", "-qm", "upstream " + file}, {"-C", other, "push", "-q", "origin", "main"}} {
			if args == nil {
				if err := os.WriteFile(filepath.Join(other, file), []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
				continue
			}
			if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, out)
			}
		}
	}
	write(t, c, "notes.md", "one\n")
	git("add", ".")
	git("commit", "-qm", "work")

	advance("UPSTREAM", "moved\n")
	result, err := c.CatchUp()
	if err != nil || !result.Merged {
		t.Fatalf("catch-up: %+v %v", result, err)
	}
	if got := git("rev-list", "--count", "--merges", "HEAD"); got != "1" {
		t.Fatalf("%s merge commits, want 1", got)
	}
	if out, _ := exec.Command("git", "-C", bare, "branch", "--list", "openclaw/*").Output(); len(out) != 0 {
		t.Fatalf("catch-up pushed: %s", out)
	}
	if again, err := c.CatchUp(); err != nil || again.Merged {
		t.Fatalf("second catch-up: %+v %v", again, err)
	}

	advance("notes.md", "theirs\n")
	before := git("rev-parse", "HEAD")
	if _, err := c.CatchUp(); err == nil || !strings.Contains(err.Error(), "notes.md") {
		t.Fatalf("conflicting catch-up: %v", err)
	}
	if git("rev-parse", "HEAD") != before || git("status", "--porcelain") != "" {
		t.Fatal("a conflicted catch-up left the checkout changed")
	}
}

func TestStatus(t *testing.T) {
	c, git := managedWorktree(t, "git@bitbucket.org:example-ws/docs.git")
	write(t, c, "notes.md", "one\n")
	git("add", ".")
	git("commit", "-qm", "Publish openclaw/72-add-thing\n\nOpenClaw-Publication: 53a47a99")
	write(t, c, "notes.md", "two\n")
	git("commit", "-qam", "More")
	write(t, c, "stray.txt", "x\n")
	write(t, c, "README.md", "changed\n")
	got := CheckoutsStatus([]string{c.Dir, t.TempDir()}, true)
	if got.Schema != CollectionSchema || len(got.Repos) != 2 {
		t.Fatalf("collection %+v", got)
	}
	want := CheckoutStatus{Path: c.Dir, Repo: "docs", Forge: ForgeBitbucket, Branch: "openclaw/72-add-thing", Base: "origin/main",
		Ahead: 2, Dirty: 2, Untracked: 1, Published: true, Unpublished: 1}
	if got.Repos[0] != want {
		t.Errorf("got  %+v\nwant %+v", got.Repos[0], want)
	}
	if got.Repos[1].Error == "" {
		t.Errorf("a directory that is no checkout: %+v", got.Repos[1])
	}
}

func TestPlanPublishInASandboxCopy(t *testing.T) {
	// What a sandbox gets: the session's branch, no origin, the skills mirror.
	c, git := managedWorktree(t, "https://github.com/example-org/docs.git")
	git("remote", "remove", "origin")
	c, err := OpenCheckout(c.Dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if !c.Private {
		t.Fatal("a checkout without origin is not taken for a sandbox's copy")
	}
	write(t, c, "notes.md", "one\n")
	write(t, c, ".openclaw/sandbox-skills/github/SKILL.md", "mirror\n")
	plan, err := c.PlanPublish(PublishOptions{Title: "Add notes"})
	if err != nil {
		t.Fatal(err)
	}
	in := plan.Intents[0]
	if in.Tool != ToolGitHubPublish || in.Forge != ForgeUnknown || in.Args.(PublishArgs).Title != "Add notes" {
		t.Fatalf("intent %+v", in)
	}
	if strings.Join(plan.Pending, " ") != "notes.md" {
		t.Fatalf("pending %v: the skills mirror is not the agent's work", plan.Pending)
	}
	write(t, c, ".env", "TOKEN=x\n")
	if _, err := c.PlanPublish(PublishOptions{}); err == nil || !strings.Contains(err.Error(), ".env") {
		t.Fatalf("a .env in a sandbox's copy: %v", err)
	}
}
