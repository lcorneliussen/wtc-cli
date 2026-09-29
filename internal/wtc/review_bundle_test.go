package wtc

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublicReviewBundleUsesBaseConcernsAndExcludesLocalOverlays(t *testing.T) {
	collection := filepath.Join(t.TempDir(), "sample")
	harness := filepath.Join(collection, "harness")
	repo := filepath.Join(collection, "app")
	if err := os.MkdirAll(harness, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(repo, 0755); err != nil {
		t.Fatal(err)
	}
	registry := "repos:\n  - name: app\n    remote: https://github.com/example/app.git\n    default_ref: main\n"
	if err := os.WriteFile(filepath.Join(harness, ".harness-repos.yml"), []byte(registry), 0644); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
	}
	git("init", "-b", "main")
	git("config", "user.email", "example@example.invalid")
	git("config", "user.name", "Example")
	if err := os.MkdirAll(filepath.Join(repo, "review", "concerns"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "review", "concerns", "code.md"), []byte("---\nid: code\napplies: '*.go'\n---\nReview Go code.\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "review", "concerns", "docs.md"), []byte("---\nid: docs\napplies: '*.md'\n---\nReview docs.\n"), 0644); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-m", "base")
	git("switch", "-c", "feature")
	if err := os.WriteFile(filepath.Join(repo, "feature file.go"), []byte("package feature\n"), 0644); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-m", "add feature")
	if err := os.MkdirAll(filepath.Join(harness, "review", "concerns.d"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(harness, "review", "concerns.d", "private.md"), []byte("private overlay"), 0644); err != nil {
		t.Fatal(err)
	}
	c, err := OpenCollection(collection)
	if err != nil {
		t.Fatal(err)
	}
	first, err := c.BuildPublicReviewBundle(ReviewBundleOptions{Repo: "app", Base: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Files != 1 || first.Concerns != 1 || first.Manifest.Round != 1 {
		t.Fatalf("unexpected bundle: %+v", first)
	}
	if _, err := os.Stat(filepath.Join(first.Dir, "concerns", "code.md")); err != nil {
		t.Fatal(err)
	}
	changed, err := os.ReadFile(filepath.Join(first.Dir, "changed-files.txt"))
	if err != nil || string(changed) != "feature file.go\n" {
		t.Fatalf("changed path with space was lost: %q %v", changed, err)
	}
	for _, path := range []string{"concerns/docs.md", "concerns/private.md", "related", "downstream", "upstream"} {
		if _, err := os.Stat(filepath.Join(first.Dir, path)); !os.IsNotExist(err) {
			t.Fatalf("public bundle included %s", path)
		}
	}
	manifest, err := os.ReadFile(filepath.Join(first.Dir, "manifest.env"))
	if err != nil || !strings.Contains(string(manifest), "ROUND='1'") {
		t.Fatalf("invalid shell manifest: %s %v", manifest, err)
	}
	second, err := c.BuildPublicReviewBundle(ReviewBundleOptions{Repo: "app", Base: "main"})
	if err != nil || second.Manifest.Round != 2 {
		t.Fatalf("next round: %+v %v", second, err)
	}
}
