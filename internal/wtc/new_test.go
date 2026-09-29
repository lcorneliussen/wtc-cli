package wtc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixtureGit(t *testing.T, args ...string) string {
	t.Helper()
	out, err := gitOutput(args...)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func fixtureFile(t *testing.T, path, data string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), mode); err != nil {
		t.Fatal(err)
	}
}

func newWorkspaceFixture(t *testing.T) *Context {
	t.Helper()
	// The fixture has the same shared-bare geometry as an actual workspace but
	// all remotes are local, so a fetch never reaches the network.
	t.Setenv("PATH", "/usr/bin:/bin")
	t.Setenv("WTC_HARNESS_REPO", "agent-harness")
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".bare"), 0755); err != nil {
		t.Fatal(err)
	}
	sources := map[string]string{}
	for _, name := range []string{"agent-harness", "widget"} {
		source := filepath.Join(root, "source-"+name)
		if err := os.Mkdir(source, 0755); err != nil {
			t.Fatal(err)
		}
		fixtureGit(t, "-C", source, "init", "-q", "-b", "main")
		fixtureFile(t, filepath.Join(source, "README.md"), name+"\n", 0644)
		sources[name] = source
	}
	harness := sources["agent-harness"]
	registry := "repos:\n" +
		"  - name: agent-harness\n    remote: " + harness + "\n    default_ref: origin/main\n" +
		"  - name: widget\n    remote: " + sources["widget"] + "\n    default_ref: origin/main\n    port_offset: 1\n    issues_prefix: wid-\n"
	fixtureFile(t, filepath.Join(harness, ".harness-repos.yml"), registry, 0644)
	fixtureFile(t, filepath.Join(harness, ".wtc-cli-version"), "0.1.9\n", 0644)
	fixtureFile(t, filepath.Join(harness, "collection-AGENTS.md"), "# Fixture\n", 0644)
	fixtureFile(t, filepath.Join(sources["widget"], ".harness", "init.sh"), "#!/bin/sh\nprintf ready > init-ran\n", 0755)
	for name, source := range sources {
		fixtureGit(t, "-C", source, "add", "-A")
		fixtureGit(t, "-C", source, "-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "fixture")
		bare := filepath.Join(root, ".bare", name+".git")
		fixtureGit(t, "init", "-q", "--bare", bare)
		fixtureGit(t, "--git-dir="+bare, "remote", "add", "origin", source)
		fixtureGit(t, "--git-dir="+bare, "fetch", "-q", "origin", "+refs/heads/*:refs/remotes/origin/*")
	}
	main := filepath.Join(root, "main")
	if err := os.Mkdir(main, 0755); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, "--git-dir="+filepath.Join(root, ".bare", "agent-harness.git"), "worktree", "add", "-q", "--detach", filepath.Join(main, "harness"), "origin/main")
	c, err := OpenCollection(main)
	if err != nil {
		t.Fatal(err)
	}
	c.ConfigRoot = filepath.Join(root, "control")
	return c
}

func TestNewCollectionCreatesDetachedSiblingsAndGeneratedSurfaces(t *testing.T) {
	c := newWorkspaceFixture(t)
	r, err := c.NewCollection(NewOptions{Slug: "demo", Repos: []string{"widget"}})
	if err != nil {
		t.Fatal(err)
	}
	if r.IntendedBranch != "demo" || !strings.HasSuffix(r.Collection, "/demo") {
		t.Fatalf("unexpected result: %+v", r)
	}
	for _, name := range []string{"harness", "widget"} {
		worktree := filepath.Join(r.Collection, name)
		if ref := fixtureGit(t, "-C", worktree, "rev-parse", "--abbrev-ref", "HEAD"); ref != "HEAD" {
			t.Errorf("%s should be detached, got %q", name, ref)
		}
	}
	for _, path := range []string{".env.collection", ".env.collection.local", "mise.toml", "HANDOFF.md", "WTC-SCOPE.md", "AGENTS.md", "widget/init-ran"} {
		if _, err := os.Stat(filepath.Join(r.Collection, path)); err != nil {
			t.Errorf("missing %s: %v", path, err)
		}
	}
	env, err := os.ReadFile(filepath.Join(r.Collection, ".env.collection"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"COLLECTION_PORT_BASE=42000", "WIDGET_PORT=42001", "WTC_CONFIG_ROOT='" + c.ConfigRoot + "'"} {
		if !strings.Contains(string(env), expected) {
			t.Errorf("generated env lacks %q", expected)
		}
	}
	other, err := c.NewCollection(NewOptions{Issue: "wid-7", Slug: "fix"})
	if err != nil {
		t.Fatal(err)
	}
	if len(other.Repositories) != 2 || other.Repositories[1] != "widget" {
		t.Fatalf("issue primary was not included: %+v", other)
	}
	env, _ = os.ReadFile(filepath.Join(other.Collection, ".env.collection"))
	if !strings.Contains(string(env), "COLLECTION_PORT_BASE=42100") {
		t.Errorf("second collection did not receive distinct port block: %s", env)
	}
}

func TestNewCollectionRejectsInvalidNameBeforeWriting(t *testing.T) {
	c := newWorkspaceFixture(t)
	if _, err := c.NewCollection(NewOptions{Slug: "../escape", Repos: []string{"widget"}}); err == nil {
		t.Fatal("path traversal accepted")
	}
	if _, err := os.Stat(filepath.Join(c.Workspace, "escape")); !os.IsNotExist(err) {
		t.Fatalf("unexpected destination created: %v", err)
	}
}

func TestNewCollectionPreHookCanVetoBeforeCreatingDirectory(t *testing.T) {
	c := newWorkspaceFixture(t)
	fixtureFile(t, filepath.Join(c.Harness, "hooks", "wtc", "new.pre.sh"), "#!/bin/sh\nexit 19\n", 0755)
	if _, err := c.NewCollection(NewOptions{Slug: "vetoed", Repos: []string{"widget"}}); err == nil || !strings.Contains(err.Error(), "new.pre") {
		t.Fatalf("pre-hook did not veto: %v", err)
	}
	if _, err := os.Stat(filepath.Join(c.Workspace, "vetoed")); !os.IsNotExist(err) {
		t.Fatalf("vetoed collection exists: %v", err)
	}
}
