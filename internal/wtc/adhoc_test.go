package wtc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// adHocFixture isolates the per-user owner cache, registry and control root,
// and returns a directory holding synthetic source repositories.
func adHocFixture(t *testing.T) string {
	t.Helper()
	t.Setenv("PATH", "/usr/bin:/bin")
	t.Setenv("WTC_HARNESS_REPO", "")
	t.Setenv("WTC_CONFIG_ROOT", "")
	root := t.TempDir()
	t.Setenv("WTC_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	return root
}

func adHocSource(t *testing.T, root, name string, branches ...string) string {
	t.Helper()
	source := filepath.Join(root, "src", name)
	fixtureFile(t, filepath.Join(source, "README.md"), name+"\n", 0644)
	fixtureGit(t, "-C", source, "init", "-q", "-b", "main")
	commit := func(message string) {
		fixtureGit(t, "-C", source, "add", "-A")
		fixtureGit(t, "-C", source, "-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", message)
	}
	commit("main")
	for _, branch := range branches {
		fixtureGit(t, "-C", source, "switch", "-q", "-c", branch)
		fixtureFile(t, filepath.Join(source, branch+".txt"), branch+"\n", 0644)
		commit(branch)
		fixtureGit(t, "-C", source, "switch", "-q", "main")
	}
	return source
}

func realPath(t *testing.T, path string) string {
	t.Helper()
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return real
}

func initAdHoc(t *testing.T, dir string) *Context {
	t.Helper()
	if _, err := InitAdHoc(dir); err != nil {
		t.Fatal(err)
	}
	c, err := Discover(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !c.AdHoc {
		t.Fatalf("expected an ad-hoc collection: %+v", c)
	}
	return c
}

func addAdHoc(t *testing.T, c *Context, opt AdHocRepoOptions) Repo {
	t.Helper()
	repo, err := c.RegisterAdHocRepo(opt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.AddRepositories(AddRepoOptions{Repos: []string{repo.Name}}); err != nil {
		t.Fatal(err)
	}
	return repo
}

func TestInitAdHocCreatesCollectionInAnEmptyFolder(t *testing.T) {
	root := adHocFixture(t)
	dir := filepath.Join(root, "task")
	c := initAdHoc(t, dir)
	if c.Collection != dir || c.Harness != filepath.Join(dir, ".wtc", "config") {
		t.Fatalf("unexpected geometry: %+v", c)
	}
	if c.ConfigRoot != filepath.Join(root, "config", "wtc") {
		t.Fatalf("ad-hoc control root should be per user, got %s", c.ConfigRoot)
	}
	for _, path := range []string{"AGENTS.md", "WTC-SCOPE.md", ".env.collection", "mise.toml", ".wtc/skills/wtc-start/SKILL.md"} {
		if _, err := os.Stat(filepath.Join(dir, path)); err != nil {
			t.Fatalf("missing %s: %v", path, err)
		}
	}
	if harness, err := c.HarnessRepoName(); err != nil || harness != "" {
		t.Fatalf("ad-hoc collection has no harness repository: %q %v", harness, err)
	}
	nested := filepath.Join(dir, "sub")
	if err := os.Mkdir(nested, 0755); err != nil {
		t.Fatal(err)
	}
	if found, err := Discover(nested); err != nil || found.Collection != dir {
		t.Fatalf("discovery from a subfolder: %v %+v", err, found)
	}
	if _, err := InitAdHoc(nested); err == nil || !strings.Contains(err.Error(), "already inside a collection") {
		t.Fatalf("nested init was accepted: %v", err)
	}
	busy := filepath.Join(root, "busy")
	fixtureFile(t, filepath.Join(busy, "notes.txt"), "keep\n", 0644)
	if _, err := InitAdHoc(busy); err == nil || !strings.Contains(err.Error(), "not empty") {
		t.Fatalf("non-empty folder was accepted: %v", err)
	}
	if _, err := Discover(filepath.Join(root, "src")); err == nil || !strings.Contains(err.Error(), "wtc init") {
		t.Fatalf("discovery outside a collection should name wtc init: %v", err)
	}
}

func TestAdHocAddRepoClonesIntoUserCacheAndRemembersIt(t *testing.T) {
	root := adHocFixture(t)
	source := adHocSource(t, root, "widget")
	remote := "file://" + source
	c := initAdHoc(t, filepath.Join(root, "first"))
	repo := addAdHoc(t, c, AdHocRepoOptions{Spec: remote})
	if repo.Name != "widget" || repo.DefaultRef != "origin/main" || repo.Owner != "" {
		t.Fatalf("unexpected registration: %+v", repo)
	}
	owner, err := c.bareFor("widget")
	if err != nil {
		t.Fatal(err)
	}
	if owner, err = filepath.EvalSymlinks(owner); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(owner, filepath.Join("data", "bare", "local")) {
		t.Fatalf("owner outside the per-user cache: %s", owner)
	}
	worktree := filepath.Join(c.Collection, "widget")
	if head := fixtureGit(t, "-C", worktree, "rev-parse", "--abbrev-ref", "HEAD"); head != "HEAD" {
		t.Fatalf("worktree should rest detached, got %s", head)
	}
	if common := fixtureGit(t, "-C", worktree, "rev-parse", "--path-format=absolute", "--git-common-dir"); realPath(t, common) != owner {
		t.Fatalf("worktree owner %s, want %s", common, owner)
	}
	// The next ad-hoc collection adds it by name, sharing the same owner.
	second := initAdHoc(t, filepath.Join(root, "second"))
	again := addAdHoc(t, second, AdHocRepoOptions{Spec: "widget"})
	if again.Remote != remote || again.DefaultRef != "origin/main" {
		t.Fatalf("per-user registry was not reused: %+v", again)
	}
	if common := fixtureGit(t, "-C", filepath.Join(second.Collection, "widget"), "rev-parse", "--path-format=absolute", "--git-common-dir"); realPath(t, common) != owner {
		t.Fatalf("second collection did not share the owner: %s", common)
	}
	// A different repository may not take a remembered name.
	other := adHocSource(t, root, "other")
	if _, err := second.RegisterAdHocRepo(AdHocRepoOptions{Spec: "file://" + other, Name: "widget"}); err == nil || !strings.Contains(err.Error(), "--name") {
		t.Fatalf("name collision was accepted: %v", err)
	}
	if _, err := second.RegisterAdHocRepo(AdHocRepoOptions{Spec: "unknown"}); err == nil || !strings.Contains(err.Error(), "clone URL") {
		t.Fatalf("unknown name was accepted: %v", err)
	}
}

func TestAdHocCheckoutOwnerStartsFromTargetNotItsBranch(t *testing.T) {
	root := adHocFixture(t)
	source := adHocSource(t, root, "service", "develop")
	checkout := filepath.Join(root, "checkout")
	fixtureGit(t, "clone", "-q", source, checkout)
	fixtureGit(t, "-C", checkout, "switch", "-q", "-c", "local-feature")
	c := initAdHoc(t, filepath.Join(root, "task"))
	var asked string
	repo := addAdHoc(t, c, AdHocRepoOptions{Spec: checkout, Ask: func(name, suggested string) (string, error) {
		asked = name + "@" + suggested
		return "develop", nil
	}})
	if asked != "service@main" {
		t.Fatalf("target prompt got %q", asked)
	}
	owner := fixtureGit(t, "-C", checkout, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if repo.Owner != owner || repo.DefaultRef != "origin/develop" || repo.Remote != source {
		t.Fatalf("unexpected registration: %+v", repo)
	}
	worktree := filepath.Join(c.Collection, "service")
	want := fixtureGit(t, "-C", source, "rev-parse", "develop")
	if got := fixtureGit(t, "-C", worktree, "rev-parse", "HEAD"); got != want {
		t.Fatalf("worktree at %s, want develop tip %s", got, want)
	}
	if branch := fixtureGit(t, "-C", checkout, "branch", "--show-current"); branch != "local-feature" {
		t.Fatalf("existing checkout was moved to %s", branch)
	}
	if _, err := c.RegisterAdHocRepo(AdHocRepoOptions{Spec: "service", Target: "main"}); err == nil {
		t.Fatal("re-registration silently changed the development tip")
	}
	if _, err := c.RegisterAdHocRepo(AdHocRepoOptions{Spec: "file://" + source, Name: "missing-branch", Target: "nope"}); err == nil || !strings.Contains(err.Error(), "no branch") {
		t.Fatalf("missing target branch was accepted: %v", err)
	}
}

func TestAdHocRegistryKeepsHandEditsAndLocalHooksRun(t *testing.T) {
	root := adHocFixture(t)
	c := initAdHoc(t, filepath.Join(root, "task"))
	registry := filepath.Join(c.Harness, "repos.yml")
	data, err := os.ReadFile(registry)
	if err != nil {
		t.Fatal(err)
	}
	fixtureFile(t, registry, string(data)+"  # hand-written note\n", 0644)
	marker := filepath.Join(root, "hook-ran")
	fixtureFile(t, filepath.Join(c.Harness, "hooks", "wtc", "add-repo.post.sh"), "#!/bin/sh\ncat > '"+marker+"'\n", 0755)
	source := adHocSource(t, root, "widget")
	addAdHoc(t, c, AdHocRepoOptions{Spec: "file://" + source, Target: "main"})
	updated, err := os.ReadFile(registry)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated), "# hand-written note") || !strings.Contains(string(updated), "name: widget") {
		t.Fatalf("registry lost hand edits or the entry:\n%s", updated)
	}
	payload, err := os.ReadFile(marker)
	if err != nil || !strings.Contains(string(payload), `"repositories":"widget"`) {
		t.Fatalf("collection-local hook did not run: %q %v", payload, err)
	}
}

func TestAdHocRefusesWorkspaceOperationsAndRetiresInPlace(t *testing.T) {
	root := adHocFixture(t)
	c := initAdHoc(t, filepath.Join(root, "task"))
	addAdHoc(t, c, AdHocRepoOptions{Spec: "file://" + adHocSource(t, root, "widget")})
	if _, err := c.NewCollection(NewOptions{Slug: "next"}); err == nil {
		t.Fatal("wtc new is not available in ad-hoc collections yet")
	}
	if _, _, err := c.CatchUpInventory(CatchUpOptions{All: true, DryRun: true}); err == nil {
		t.Fatal("an ad-hoc collection must not sweep its parent folder")
	}
	if _, err := c.AddRepositories(AddRepoOptions{Collection: "elsewhere", Repos: []string{"widget"}}); err == nil {
		t.Fatal("an ad-hoc collection has no sibling collections")
	}
	if _, err := c.RefreshRegistry(); err == nil {
		t.Fatal("an ad-hoc collection has no bare-owner map")
	}
	owner, err := c.bareFor("widget")
	if err != nil {
		t.Fatal(err)
	}
	result, err := c.RetireCollection(RetireOptions{Name: "task", Self: true, InPlace: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.FolderRemoved {
		t.Fatalf("collection folder remains: %+v", result)
	}
	if _, err := os.Stat(owner); err != nil {
		t.Fatalf("retire removed the shared owner: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "data", "repos.yml")); err != nil {
		t.Fatalf("retire removed the per-user registry: %v", err)
	}
}

func TestRemoteKeySeparatesHostsAndRejectsTraversal(t *testing.T) {
	for remote, want := range map[string]string{
		"git@github.com:acme/api.git":        "github.com/acme/api",
		"https://github.com/acme/api":        "github.com/acme/api",
		"ssh://git@git.example:22/x/api.git": "git.example/x/api",
		"/srv/git/api.git":                   "local/srv/git/api",
	} {
		key, err := remoteKey(remote)
		if err != nil || strings.Join(key, "/") != want {
			t.Fatalf("%s: got %v %v, want %s", remote, key, err, want)
		}
	}
	for _, remote := range []string{"https://example.com/../x", "api", "git@host:"} {
		if _, err := remoteKey(remote); err == nil {
			t.Fatalf("accepted %q", remote)
		}
	}
}
