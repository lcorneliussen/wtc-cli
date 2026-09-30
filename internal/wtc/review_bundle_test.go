package wtc

import (
	"encoding/json"
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
	if err := os.WriteFile(filepath.Join(first.Dir, "summary.md"), []byte("First public round\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := writeInlineRecords(first.Dir, []ReviewInlineRecord{{Key: "012345abcd", ID: "123"}, {Key: "def567abcd"}}); err != nil {
		t.Fatal(err)
	}
	privateDir := filepath.Join(collection, ".wtc-reviews", "private-round")
	if err := os.MkdirAll(privateDir, 0755); err != nil {
		t.Fatal(err)
	}
	privateManifest := first.Manifest
	privateManifest.Public = false
	privateData, err := json.Marshal(privateManifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(privateDir, "manifest.json"), privateData, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(privateDir, "summary.md"), []byte("private context\n"), 0644); err != nil {
		t.Fatal(err)
	}
	second, err := c.BuildPublicReviewBundle(ReviewBundleOptions{Repo: "app", Base: "main"})
	if err != nil || second.Manifest.Round != 2 {
		t.Fatalf("next round: %+v %v", second, err)
	}
	prior, err := os.ReadFile(filepath.Join(second.Dir, "prior", "r1.md"))
	if err != nil || string(prior) != "First public round\n" {
		t.Fatalf("public prior round missing or mixed with private context: %q %v", prior, err)
	}
	keys, err := os.ReadFile(filepath.Join(second.Dir, "prior", "inline-keys.txt"))
	if err != nil || string(keys) != "012345abcd\n" {
		t.Fatalf("posted inline keys missing or unposted key included: %q %v", keys, err)
	}
	bin := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	gh := `#!/bin/sh
case "$*" in
	  *'--json title,body,baseRefName,headRefOid,headRefName,url') [ "${GH_FAIL_PR_INFO:-}" != 1 ] || exit 1; cat "$GH_PR_INFO" ;;
  *'--json comments') [ "${GH_FAIL_COMMENTS:-}" != 1 ] || exit 1; cat "$GH_CONVERSATION" ;;
  'api '*) cat "$GH_INLINE" ;;
  *) exit 2 ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(gh), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	writeFixture := func(name, body string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	t.Setenv("GH_PR_INFO", writeFixture("pr.json", `{"title":"Improve app","body":"Review this change","baseRefName":"main","headRefName":"feature","url":"https://github.com/example/app/pull/7"}`))
	t.Setenv("GH_CONVERSATION", writeFixture("comments.json", `{"comments":[{"author":{"login":"author"},"createdAt":"2026-01-01T11:00:00Z","body":"follow-up reply"}]}`))
	t.Setenv("GH_INLINE", writeFixture("inline.json", `[[]]`))
	third, err := c.BuildPublicReviewBundle(ReviewBundleOptions{Repo: "app", PR: "7", Base: "main"})
	if err != nil {
		t.Fatal(err)
	}
	comments, err := os.ReadFile(filepath.Join(third.Dir, "prior", "comments.md"))
	if err != nil || !strings.Contains(string(comments), "follow-up reply") {
		t.Fatalf("GitHub reply context missing: %s %v", comments, err)
	}
	t.Setenv("GH_FAIL_COMMENTS", "1")
	fourth, err := c.BuildPublicReviewBundle(ReviewBundleOptions{Repo: "app", PR: "7", Base: "main"})
	if err != nil {
		t.Fatalf("forge outage prevented local bundle: %v", err)
	}
	if _, err := os.Stat(filepath.Join(fourth.Dir, "prior", "comments.md")); !os.IsNotExist(err) {
		t.Fatalf("unavailable comment context was written: %v", err)
	}
	t.Setenv("GH_FAIL_PR_INFO", "1")
	fifth, err := c.BuildPublicReviewBundle(ReviewBundleOptions{Repo: "app", PR: "7", Base: "main"})
	if err != nil {
		t.Fatalf("forge outage prevented local review bundle: %v", err)
	}
	prText, err := os.ReadFile(filepath.Join(fifth.Dir, "pr.md"))
	if err != nil || !strings.Contains(string(prText), "PR #7 not readable") || fifth.Manifest.URL != "https://github.com/example/app/pull/7" {
		t.Fatalf("offline PR fallback missing: %s %+v %v", prText, fifth.Manifest, err)
	}
}
