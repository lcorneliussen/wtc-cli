package wtc

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrivateReviewBundleIncludesSnapshotsOverlaysAndRelatedPatch(t *testing.T) {
	root := t.TempDir()
	collection := filepath.Join(root, "task")
	harness := filepath.Join(collection, "harness")
	for _, name := range []string{"harness", "library", "consumer", "framework"} {
		if err := os.MkdirAll(filepath.Join(collection, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	registry := "repos:\n" +
		"  - name: library\n    remote: https://github.com/example/library.git\n    default_ref: main\n    downstream: consumer\n" +
		"  - name: consumer\n    remote: https://github.com/example/consumer.git\n    production_ref: main\n" +
		"  - name: framework\n    remote: https://github.com/example/framework.git\n    default_ref: main\n    downstream: library\n"
	if err := os.WriteFile(filepath.Join(harness, ".harness-repos.yml"), []byte(registry), 0644); err != nil {
		t.Fatal(err)
	}
	git := func(name string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", filepath.Join(collection, name)}, args...)...)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s %v: %s: %v", name, args, output, err)
		}
	}
	write := func(name, path, body string) {
		t.Helper()
		full := filepath.Join(collection, name, path)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"library", "consumer", "framework"} {
		git(name, "init", "-b", "main")
		git(name, "config", "user.name", "Fixture")
		git(name, "config", "user.email", "fixture@example.invalid")
		write(name, "old.txt", name+" at main\n")
		if name == "library" {
			write(name, ".review/concerns/library.md", "---\nid: library\napplies: '*.go'\n---\nLibrary base concern.\n")
		}
		git(name, "add", ".")
		git(name, "commit", "-m", "base")
	}
	git("consumer", "update-ref", "refs/remotes/origin/main", "refs/heads/main")
	git("library", "switch", "-c", "feature")
	write("library", "feature.go", "package feature\n")
	git("library", "add", ".")
	git("library", "commit", "-m", "feature")
	git("consumer", "switch", "-c", "feature")
	write("consumer", "new.txt", "consumer on feature\n")
	git("consumer", "add", ".")
	git("consumer", "commit", "-m", "consumer feature")
	write("harness", "review/concerns/code.md", "---\nid: code\napplies: '*.go'\n---\nGeneric concern.\n")
	write("harness", "review/concerns.d/code.md", "---\nid: code\napplies: '*.go'\n---\nLocal overlay.\n")
	if err := os.WriteFile(filepath.Join(collection, ".wtc-prs"), []byte("consumer 8 feature\n"), 0644); err != nil {
		t.Fatal(err)
	}
	c, err := OpenCollection(collection)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := c.BuildReviewBundle(ReviewBundleOptions{Repo: "library", Base: "main", NoCatchUp: true})
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Manifest.Public || bundle.Manifest.Downstream != "consumer" || bundle.Manifest.Upstream != "framework" || bundle.Concerns != 2 || bundle.Related != 1 {
		t.Fatalf("incomplete private bundle: %+v", bundle)
	}
	for path, want := range map[string]string{
		"downstream/consumer/old/old.txt": "consumer at main",
		"downstream/consumer/new/new.txt": "consumer on feature",
		"upstream/framework/old/old.txt":  "framework at main",
		"concerns/code.md":                "Local overlay.",
		"concerns/library.md":             "Library base concern.",
		"related/consumer-pr8.patch":      "new.txt",
	} {
		body, err := os.ReadFile(filepath.Join(bundle.Dir, path))
		if err != nil || !strings.Contains(string(body), want) {
			t.Fatalf("%s: %q %v", path, body, err)
		}
	}
	if _, err := os.Stat(filepath.Join(bundle.Dir, "upstream", "framework", "new")); !os.IsNotExist(err) {
		t.Fatalf("framework on its main tip unexpectedly has a new snapshot: %v", err)
	}
	public, err := c.BuildPublicReviewBundle(ReviewBundleOptions{Repo: "library", Base: "main"})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"downstream", "upstream", "related"} {
		if _, err := os.Stat(filepath.Join(public.Dir, path)); !os.IsNotExist(err) {
			t.Fatalf("public bundle included private %s", path)
		}
	}
}

func TestReviewBundleChecksOutPRBranchAndStopsOnCatchUpFailure(t *testing.T) {
	c := newWorkspaceFixture(t)
	owner := filepath.Join(c.Workspace, ".bare", "widget.git")
	worktree := filepath.Join(c.Collection, "widget")
	fixtureGit(t, "--git-dir="+owner, "config", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*")
	fixtureGit(t, "--git-dir="+owner, "worktree", "add", "-q", "--detach", worktree, "origin/main")
	source := filepath.Join(c.Workspace, "source-widget")
	fixtureGit(t, "-C", source, "switch", "-q", "-c", "feature")
	fixtureFile(t, filepath.Join(source, "feature.go"), "package feature\n", 0644)
	fixtureGit(t, "-C", source, "add", "feature.go")
	fixtureGit(t, "-C", source, "-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "feature")
	head := fixtureGit(t, "-C", source, "rev-parse", "HEAD")
	for i := range c.Registry.Repos {
		if c.Registry.Repos[i].Name == "widget" {
			c.Registry.Repos[i].Remote = "https://github.com/example/widget.git"
		}
	}
	bin := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	gh := fmt.Sprintf("#!/bin/sh\ncase \"$*\" in\n  *'--json title,body,baseRefName,headRefOid,headRefName,url') printf '%%s\\n' '%s' ;;\n  *) exit 1 ;;\nesac\n", fmt.Sprintf(`{"title":"Feature","body":"Fixture","baseRefName":"main","headRefOid":"%s","headRefName":"feature","url":"https://github.com/example/widget/pull/7"}`, head))
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(gh), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	_, err := c.BuildReviewBundle(ReviewBundleOptions{Repo: "widget", PR: "7", Public: true})
	if err == nil || !strings.Contains(err.Error(), "catch-up before review") {
		t.Fatalf("review ignored an unsafe catch-up: %v", err)
	}
	if _, err := os.Stat(filepath.Join(c.Collection, ".wtc-catch-up.json")); err != nil {
		t.Fatalf("catch-up report missing after refusal: %v", err)
	}
	gh = strings.Replace(gh, "  *) exit 1 ;;", "  *'--json state,isDraft') printf '%s\\n' '[{\"state\":\"OPEN\",\"isDraft\":false}]' ;;\n  *) exit 1 ;;", 1)
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(gh), 0755); err != nil {
		t.Fatal(err)
	}
	registryPath := filepath.Join(c.Harness, ".harness-repos.yml")
	registry, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	registry = []byte(strings.Replace(string(registry), "default_ref: origin/main\n    port_offset", "default_ref: origin/feature\n    port_offset", 1))
	if err := os.WriteFile(registryPath, registry, 0644); err != nil {
		t.Fatal(err)
	}
	defaultBundle, err := c.BuildReviewBundle(ReviewBundleOptions{Repo: "widget", PR: "7", Public: true})
	if err != nil {
		report, _ := os.ReadFile(filepath.Join(c.Collection, ".wtc-catch-up.json"))
		t.Fatalf("default catch-up bundle failed: %v\n%s", err, report)
	}
	if defaultBundle.Manifest.HeadSHA != head || defaultBundle.Manifest.HeadBranch != "feature" || defaultBundle.Files != 1 {
		t.Fatalf("default catch-up bundle is incomplete: %+v", defaultBundle)
	}
	bundle, err := c.BuildReviewBundle(ReviewBundleOptions{Repo: "widget", PR: "7", Public: true, NoCatchUp: true})
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Manifest.HeadSHA != head || bundle.Manifest.HeadBranch != "feature" || fixtureGit(t, "-C", worktree, "branch", "--show-current") != "feature" {
		t.Fatalf("review did not move onto PR head: %+v", bundle.Manifest)
	}
}

func TestLegacyShellReviewRoundContinuesOnlyInPrivateBundle(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "old")
	if err := os.MkdirAll(old, 0755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"manifest.env": "REPO=widget\nPR=7\nHEAD_BRANCH=feature\nROUND=2\n",
		"summary.md":   "Earlier private review\n",
	} {
		if err := os.WriteFile(filepath.Join(old, name), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if got := nextReviewRound(root, "widget", "7", "feature"); got != 3 {
		t.Fatalf("next round = %d", got)
	}
	private := filepath.Join(root, "new-private")
	if err := copyReviewPrior(private, root, ReviewManifest{Repo: "widget", PR: "7", Round: 3}); err != nil {
		t.Fatal(err)
	}
	if body, err := os.ReadFile(filepath.Join(private, "prior", "r2.md")); err != nil || string(body) != "Earlier private review\n" {
		t.Fatalf("legacy private summary: %q %v", body, err)
	}
	public := filepath.Join(root, "new-public")
	if err := copyReviewPrior(public, root, ReviewManifest{Repo: "widget", PR: "7", Round: 3, Public: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(public, "prior")); !os.IsNotExist(err) {
		t.Fatalf("legacy summary entered public bundle: %v", err)
	}
}

func TestReviewArchiveAllowsInternalSymlinkAndRejectsEscape(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "source")
	if err := os.MkdirAll(repo, 0755); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-b", "main")
	git("config", "user.name", "Fixture")
	git("config", "user.email", "fixture@example.invalid")
	if err := os.WriteFile(filepath.Join(repo, "target.txt"), []byte("inside\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target.txt", filepath.Join(repo, "link.txt")); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-m", "internal link")
	owner := filepath.Join(repo, ".git")
	good := filepath.Join(t.TempDir(), "good")
	if err := archiveReviewCommit(owner, git("rev-parse", "HEAD"), good); err != nil {
		t.Fatal(err)
	}
	if target, err := os.Readlink(filepath.Join(good, "link.txt")); err != nil || target != "target.txt" {
		t.Fatalf("internal archive link: %q %v", target, err)
	}
	if err := os.Remove(filepath.Join(repo, "link.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../outside", filepath.Join(repo, "link.txt")); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-m", "escaping link")
	bad := filepath.Join(t.TempDir(), "bad")
	if err := archiveReviewCommit(owner, git("rev-parse", "HEAD"), bad); err == nil || !strings.Contains(err.Error(), "unsafe archive symlink") {
		t.Fatalf("escaping link was accepted: %v", err)
	}
}
