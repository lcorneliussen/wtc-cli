package openclaw

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// localGateway is the gateway's git service over a managed worktree on
// disk: the sandbox fetches from the clone's path, and a delivery is
// accepted as the plugin accepts it.
type localGateway struct{ dir, clone string }

func (g localGateway) Session() (SandboxSession, error) {
	s, err := GatewaySession(g.dir)
	s.Repo = "docs"
	return s, err
}

func (g localGateway) RemoteURL(string) string { return g.clone }

func (g localGateway) Deliver(commit, bundle string) (Delivery, error) {
	return AcceptSandboxBranch(g.dir, commit, bundle)
}

// sandboxCopy is what a sandbox gets of the managed worktree: its branch,
// one commit deep, without an origin.
func sandboxCopy(t *testing.T, c *Checkout) (localGateway, string, func(args ...string) string) {
	t.Helper()
	clone, err := git(c.Dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "workspace")
	run := func(dir string, args ...string) string {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return strings.TrimSpace(string(out))
	}
	run(t.TempDir(), "clone", "-q", "--depth", "1", "--branch", c.Branch(), "file://"+clone, dir)
	run(dir, "remote", "remove", "origin")
	return localGateway{dir: c.Dir, clone: clone}, dir, func(args ...string) string { return run(dir, args...) }
}

// upstream commits content to file on origin's main and makes the gateway
// fetch it.
func upstream(t *testing.T, c *Checkout, file, content string) {
	t.Helper()
	bare, _ := git(c.Dir, "remote", "get-url", "--push", "origin")
	other := t.TempDir()
	for _, args := range [][]string{{"clone", "-q", bare, other}, nil, {"-C", other, "add", "."},
		{"-C", other, "commit", "-qm", "upstream " + file}, {"-C", other, "push", "-q", "origin", "main"},
		{"-C", c.Dir, "fetch", "-q", bare, "+refs/heads/main:refs/remotes/origin/main"}} {
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

func writeAt(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSandboxCatchUpDeliversTheBranch(t *testing.T) {
	c, gw := managedWorktree(t, "https://github.com/example-org/docs.git")
	g, sb, sbgit := sandboxCopy(t, c)
	writeAt(t, sb, "notes.md", "session\n")
	sbgit("add", ".")
	sbgit("commit", "-qm", "work in the sandbox")
	upstream(t, c, "UPSTREAM", "moved\n")
	// Edits not committed yet, and what OpenClaw mounts into the checkout.
	writeAt(t, sb, "wip.md", "wip\n")
	writeAt(t, sb, ".openclaw/sandbox-skills/x/SKILL.md", "mirror\n")

	r, err := SandboxCatchUpCheckout(sb, g, false)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Merged || r.Delivered == nil || r.Delivered.Commits != 2 || r.Delivered.Merges != 1 || r.Reset {
		t.Fatalf("catch-up %+v %+v", r, r.Delivered)
	}
	if head := sbgit("rev-parse", "HEAD"); gw("rev-parse", "HEAD") != head || r.Delivered.Head != head {
		t.Fatalf("the gateway is at %s, the sandbox at %s", gw("rev-parse", "HEAD"), head)
	}
	if got := gw("diff", "--cached", "--name-only", "HEAD"); got != "" {
		t.Fatalf("the gateway's index is not its branch: %s", got)
	}
	if _, err := os.Stat(filepath.Join(sb, "wip.md")); err != nil {
		t.Fatal("the uncommitted file did not come back")
	}
	if got := sbgit("stash", "list"); got != "" {
		t.Fatalf("a stash was left behind: %s", got)
	}
	if sbgit("rev-parse", "--is-shallow-repository") != "false" || sbgit("rev-parse", "origin/main") != gw("rev-parse", "origin/main") {
		t.Fatal("the sandbox did not get the gateway's history and origin/main")
	}

	again, err := SandboxCatchUpCheckout(sb, g, false)
	if err != nil || again.Merged || again.Delivered != nil {
		t.Fatalf("second catch-up: %+v %v", again, err)
	}
}

func TestSandboxCatchUpConflict(t *testing.T) {
	c, gw := managedWorktree(t, "https://github.com/example-org/docs.git")
	g, sb, sbgit := sandboxCopy(t, c)
	writeAt(t, sb, "README.md", "session\n")
	sbgit("commit", "-qam", "work in the sandbox")
	upstream(t, c, "README.md", "upstream\n")
	before := gw("rev-parse", "HEAD")

	r, err := SandboxCatchUpCheckout(sb, g, false)
	if err != nil || strings.Join(r.Conflicts, " ") != "README.md" || r.Delivered != nil {
		t.Fatalf("a conflicting catch-up: %+v %v", r, err)
	}
	// Git's own merge state, for the agent to resolve with.
	if sbgit("show", ":1:README.md") != "docs" || sbgit("show", ":3:README.md") != "upstream" {
		t.Fatal("no merge stages in the sandbox")
	}
	if _, err := SandboxCatchUpCheckout(sb, g, true); err == nil || !strings.Contains(err.Error(), "still unresolved") {
		t.Fatalf("continue before resolving: %v", err)
	}
	sbgit("add", "README.md")
	if _, err := SandboxCatchUpCheckout(sb, g, true); err == nil || !strings.Contains(err.Error(), "conflict markers") {
		t.Fatalf("continue with the markers staged: %v", err)
	}
	if gw("rev-parse", "HEAD") != before {
		t.Fatal("the gateway moved before the conflict was resolved")
	}
	writeAt(t, sb, "README.md", "both\n")
	sbgit("add", "README.md")
	r, err = SandboxCatchUpCheckout(sb, g, true)
	if err != nil || !r.Merged || r.Delivered == nil || r.Delivered.Merges != 1 {
		t.Fatalf("continue: %+v %v", r, err)
	}
	if gw("rev-parse", "HEAD") != sbgit("rev-parse", "HEAD") || gw("show", "HEAD:README.md") != "both" {
		t.Fatal("the gateway did not take the resolved merge")
	}
}

func TestSandboxCatchUpAfterThePublication(t *testing.T) {
	c, gw := managedWorktree(t, "https://github.com/example-org/docs.git")
	g, sb, sbgit := sandboxCopy(t, c)
	writeAt(t, sb, "notes.md", "session\n")
	sbgit("add", ".")
	sbgit("commit", "-qm", "private")
	// The gateway published the files, which it commits itself.
	write(t, c, "notes.md", "session\n")
	gw("add", ".")
	gw("commit", "-qm", "Publish\n\nOpenClaw-Publication: 1")
	upstream(t, c, "UPSTREAM", "moved\n")

	r, err := SandboxCatchUpCheckout(sb, g, false)
	if err != nil || !r.Reset || !r.Merged || r.Delivered == nil {
		t.Fatalf("catch-up after a publication: %+v %v", r, err)
	}
	if gw("rev-parse", "HEAD^1") != gw("rev-parse", "HEAD@{1}") {
		t.Fatal("the merge does not extend the published branch")
	}
}

func TestAcceptSandboxBranchRefuses(t *testing.T) {
	c, gw := managedWorktree(t, "https://github.com/example-org/docs.git")
	head := gw("rev-parse", "HEAD")
	// A merge of something origin does not have.
	gw("switch", "-qc", "side")
	write(t, c, "side.md", "x\n")
	gw("add", ".")
	gw("commit", "-qm", "side")
	side := gw("rev-parse", "HEAD")
	gw("switch", "-q", c.Branch())
	merge := gw("commit-tree", "-p", head, "-p", side, "-m", "merge side", side+"^{tree}")
	if _, err := AcceptSandboxBranch(c.Dir, merge, ""); err == nil || !strings.Contains(err.Error(), "none of origin's branches") {
		t.Fatalf("a merge of a local branch: %v", err)
	}
	// A branch that does not extend the gateway's, which moved since.
	gw("commit", "-q", "--allow-empty", "-m", "the gateway moved")
	head = gw("rev-parse", "HEAD")
	if _, err := AcceptSandboxBranch(c.Dir, side, ""); err == nil || !strings.Contains(err.Error(), "does not extend") {
		t.Fatalf("a branch beside the gateway's: %v", err)
	}
	if gw("rev-parse", "HEAD") != head {
		t.Fatal("a refused delivery moved the branch")
	}
	if _, err := AcceptSandboxBranch(c.Dir, "HEAD", ""); err == nil {
		t.Fatal("a ref name was taken for a commit")
	}
}

func TestConflictMarkers(t *testing.T) {
	c, gw := managedWorktree(t, "https://github.com/example-org/docs.git")
	upstream(t, c, "README.md", "upstream\n")
	write(t, c, "README.md", "session\n")
	gw("commit", "-qam", "work")
	cmd := exec.Command("git", "-C", c.Dir, "merge", "-q", "origin/main")
	_ = cmd.Run()
	gw("add", "README.md")
	gw("commit", "-qm", "merge with markers")
	if got := conflictMarkers(c.Dir, gw("rev-parse", "HEAD")); strings.Join(got, " ") != "README.md" {
		t.Fatalf("markers in %v", got)
	}
	write(t, c, "README.md", "resolved\n")
	gw("commit", "-q", "--amend", "-am", "merge")
	if got := conflictMarkers(c.Dir, gw("rev-parse", "HEAD")); len(got) != 0 {
		t.Fatalf("markers in %v after resolving", got)
	}
}

func TestSandboxCatchUpKeepsConflictingEdits(t *testing.T) {
	c, gw := managedWorktree(t, "https://github.com/example-org/docs.git")
	g, sb, sbgit := sandboxCopy(t, c)
	writeAt(t, sb, "README.md", "uncommitted\n")
	upstream(t, c, "README.md", "upstream\n")

	r, err := SandboxCatchUpCheckout(sb, g, false)
	if err != nil || strings.Join(r.Unstashed, " ") != "README.md" || r.Delivered == nil {
		t.Fatalf("edits that conflict with the base: %+v %v", r, err)
	}
	// A fast-forward to the base: the gateway has the commit already.
	if gw("rev-parse", "HEAD") != gw("rev-parse", "origin/main") {
		t.Fatal("the gateway did not move to the base")
	}
	if !strings.Contains(sbgit("diff"), "uncommitted") || sbgit("stash", "list") == "" {
		t.Fatal("the conflicting edits are neither in the file nor kept in the stash")
	}
}

func TestSandboxCatchUpMarkersFromTheBase(t *testing.T) {
	c, _ := managedWorktree(t, "https://github.com/example-org/docs.git")
	g, sb, sbgit := sandboxCopy(t, c)
	writeAt(t, sb, "README.md", "session\n")
	sbgit("commit", "-qam", "work")
	// The base brings a fixture that looks like a conflict, and a conflict.
	upstream(t, c, "fixture.txt", "<<<<<<< ours\nx\n>>>>>>> theirs\n")
	upstream(t, c, "README.md", "upstream\n")
	if r, err := SandboxCatchUpCheckout(sb, g, false); err != nil || strings.Join(r.Conflicts, " ") != "README.md" {
		t.Fatalf("catch-up: %+v %v", r, err)
	}
	writeAt(t, sb, "README.md", "both\n")
	sbgit("add", "README.md")
	if r, err := SandboxCatchUpCheckout(sb, g, true); err != nil || r.Delivered == nil {
		t.Fatalf("the base's own markers blocked the merge: %+v %v", r, err)
	}
}

func TestAcceptSandboxBranchRefusesGatewayTrailers(t *testing.T) {
	c, gw := managedWorktree(t, "https://github.com/example-org/docs.git")
	head := gw("rev-parse", "HEAD")
	for _, body := range []string{"work\n\nCo-authored-by: Someone <someone@example.com>", "work\n\nOpenClaw-Publication: 1"} {
		commit := gw("commit-tree", "-p", head, "-m", body, head+"^{tree}")
		if _, err := AcceptSandboxBranch(c.Dir, commit, ""); err == nil || !strings.Contains(err.Error(), "only the gateway writes") {
			t.Errorf("%q: %v", body, err)
		}
	}
}
