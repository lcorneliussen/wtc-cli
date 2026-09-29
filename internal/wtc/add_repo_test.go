package wtc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddRepositoriesRunsInitAfterSecretLinking(t *testing.T) {
	c := newWorkspaceFixture(t)
	t.Setenv("WTC_CONFIG_ROOT", c.ConfigRoot)
	fixtureFile(t, filepath.Join(c.ConfigRoot, "widget", "secrets.txt"), "fixture secret\n", 0600)
	base, err := c.NewCollection(NewOptions{Slug: "existing"})
	if err != nil {
		t.Fatal(err)
	}
	target, err := OpenCollection(base.Collection)
	if err != nil {
		t.Fatal(err)
	}
	result, err := target.AddRepositories(AddRepoOptions{Repos: []string{"harness", "widget"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Added) != 1 || result.Added[0] != "widget" || len(result.Skipped) != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	worktree := filepath.Join(base.Collection, "widget")
	if head := fixtureGit(t, "-C", worktree, "rev-parse", "--abbrev-ref", "HEAD"); head != "HEAD" {
		t.Fatalf("new sibling should be detached, got %s", head)
	}
	init, err := os.ReadFile(filepath.Join(worktree, "init-ran"))
	if err != nil || string(init) != "existing|"+c.ConfigRoot+"|42001|secret-present" {
		t.Fatalf("init did not receive env and linked secret: %q (%v)", init, err)
	}
	if _, err := target.AddRepositories(AddRepoOptions{Repos: []string{"widget"}}); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("existing worktree was accepted: %v", err)
	}
}

func TestAddRepositoriesExplicitBranchAndTargetSelection(t *testing.T) {
	c := newWorkspaceFixture(t)
	other, err := c.NewCollection(NewOptions{Slug: "other"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := c.AddRepositories(AddRepoOptions{Collection: "other", Repos: []string{"widget"}, Branch: "review-fix"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Collection != other.Collection || fixtureGit(t, "-C", filepath.Join(other.Collection, "widget"), "branch", "--show-current") != "review-fix" {
		t.Fatalf("incorrect target or branch: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(c.Collection, "widget")); !os.IsNotExist(err) {
		t.Fatalf("modified source collection: %v", err)
	}
}

func TestAddRepositoriesValidatesBeforeWritingAndHonorsPreHook(t *testing.T) {
	c := newWorkspaceFixture(t)
	base, err := c.NewCollection(NewOptions{Slug: "existing"})
	if err != nil {
		t.Fatal(err)
	}
	target, err := OpenCollection(base.Collection)
	if err != nil {
		t.Fatal(err)
	}
	for _, options := range []AddRepoOptions{
		{Repos: []string{"widget", "missing"}},
		{Repos: []string{"widget", "widget"}},
		{Collection: "../escape", Repos: []string{"widget"}},
		{Repos: []string{"widget"}, Branch: "invalid branch"},
	} {
		if _, err := target.AddRepositories(options); err == nil {
			t.Fatalf("accepted invalid request: %+v", options)
		}
	}
	if _, err := os.Stat(filepath.Join(base.Collection, "widget")); !os.IsNotExist(err) {
		t.Fatalf("invalid request created worktree: %v", err)
	}
	fixtureFile(t, filepath.Join(target.Harness, "hooks", "wtc", "add-repo.pre.sh"), "#!/bin/sh\nexit 21\n", 0755)
	if _, err := target.AddRepositories(AddRepoOptions{Repos: []string{"widget"}}); err == nil || !strings.Contains(err.Error(), "add-repo.pre") {
		t.Fatalf("pre-hook did not veto: %v", err)
	}
	if _, err := os.Stat(filepath.Join(base.Collection, "widget")); !os.IsNotExist(err) {
		t.Fatalf("pre-hook veto created worktree: %v", err)
	}
}

func TestAddRepositoriesRegeneratesMissingEnvironmentWithCreatingControlRoot(t *testing.T) {
	c := newWorkspaceFixture(t)
	t.Setenv("WTC_CONFIG_ROOT", "")
	base, err := c.NewCollection(NewOptions{Slug: "existing"})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".env.collection", "mise.toml"} {
		if err := os.Remove(filepath.Join(base.Collection, name)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := c.AddRepositories(AddRepoOptions{Collection: "existing", Repos: []string{"widget"}}); err != nil {
		t.Fatal(err)
	}
	env, err := os.ReadFile(filepath.Join(base.Collection, ".env.collection"))
	if err != nil || !strings.Contains(string(env), c.ConfigRoot) {
		t.Fatalf("control root was not inherited: %q (%v)", env, err)
	}
}

func TestAddRepositoriesRefreshesChangedRegistryPortsBeforeInit(t *testing.T) {
	c := newWorkspaceFixture(t)
	base, err := c.NewCollection(NewOptions{Slug: "existing"})
	if err != nil {
		t.Fatal(err)
	}
	registry := filepath.Join(base.Collection, "harness", ".harness-repos.yml")
	data, err := os.ReadFile(registry)
	if err != nil {
		t.Fatal(err)
	}
	fixtureFile(t, registry, strings.Replace(string(data), "port_offset: 1", "port_offset: 2", 1), 0644)
	target, err := OpenCollection(base.Collection)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := target.AddRepositories(AddRepoOptions{Repos: []string{"widget"}}); err != nil {
		t.Fatal(err)
	}
	init, err := os.ReadFile(filepath.Join(base.Collection, "widget", "init-ran"))
	if err != nil || !strings.HasSuffix(string(init), "|42002") {
		t.Fatalf("init used stale port: %q (%v)", init, err)
	}
}

func TestAddRepositoriesRollsBackEarlierCheckoutOnLaterFailure(t *testing.T) {
	c := newWorkspaceFixture(t)
	base, err := c.NewCollection(NewOptions{Slug: "existing"})
	if err != nil {
		t.Fatal(err)
	}
	registry := filepath.Join(base.Collection, "harness", ".harness-repos.yml")
	file, err := os.OpenFile(registry, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("  - name: broken\n    remote: /nonexistent/wtc-fixture.git\n    default_ref: origin/main\n"); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	target, err := OpenCollection(base.Collection)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := target.AddRepositories(AddRepoOptions{Repos: []string{"widget", "broken"}}); err == nil || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("later checkout did not report rollback: %v", err)
	}
	if _, err := os.Stat(filepath.Join(base.Collection, "widget")); !os.IsNotExist(err) {
		t.Fatalf("earlier worktree was left behind: %v", err)
	}
	if _, err := target.AddRepositories(AddRepoOptions{Repos: []string{"widget"}}); err != nil {
		t.Fatalf("retry after rollback failed: %v", err)
	}
}
