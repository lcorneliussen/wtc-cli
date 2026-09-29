package wtc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewPostReceiptsDedupAndResolve(t *testing.T) {
	collection := filepath.Join(t.TempDir(), "sample")
	harness := filepath.Join(collection, "harness")
	repo := filepath.Join(collection, "app")
	bundle := filepath.Join(collection, "bundle")
	for _, dir := range []string{harness, filepath.Join(repo, ".git"), filepath.Join(bundle, "findings")} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(harness, ".harness-repos.yml"), []byte("repos:\n  - name: app\n    remote: https://github.com/example/app.git\n"), 0644); err != nil {
		t.Fatal(err)
	}
	head := "1234567890abcdef1234567890abcdef12345678"
	manifest := ReviewManifest{Repo: "app", PR: "7", Forge: "github", Slug: "example/app", URL: "https://github.com/example/app/pull/7", HeadSHA: head, Round: 1, RepoDir: repo}
	if err := writeReviewManifest(bundle, manifest); err != nil {
		t.Fatal(err)
	}
	summary := "**Local review: pass**\n\n`wtc-review v1 head=" + head + " verdict=pass blockers=0 round=1 lead=test:`\n"
	if err := os.WriteFile(filepath.Join(bundle, "summary.md"), []byte(summary), 0644); err != nil {
		t.Fatal(err)
	}
	finding := `{"concern":"code","status":"issues","findings":[{"severity":"major","file":"feature.go","line":4,"title":"Broken edge","detail":"Fails on empty input"}]}`
	if err := os.WriteFile(filepath.Join(bundle, "findings", "code.json"), []byte(finding), 0644); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	log := filepath.Join(bin, "requests.log")
	launcher := `#!/bin/sh
printf '%s\n' "$*" >> "$FAKE_GH_LOG"
case "$*" in
  *'--json headRefOid'*) printf '{"headRefOid":"1234567890abcdef1234567890abcdef12345678"}\n' ;;
  *'--paginate --slurp'*) printf '[[]]\n' ;;
  *'api graphql'*'reviewThreads'*) printf '{"data":{"repository":{"pullRequest":{"reviewThreads":{"nodes":[{"id":"THREAD_1","isResolved":false,"comments":{"nodes":[{"databaseId":401}]}}]}}}}}\n' ;;
  *'api graphql'*'resolveReviewThread'*) printf '{"data":{"resolveReviewThread":{"thread":{"isResolved":true}}}}\n' ;;
  *'pulls/7/comments'*) printf '{"id":401,"html_url":"https://github.com/example/app/pull/7#discussion_r401"}\n' ;;
  *'issues/comments/301'*) printf '{"id":301,"html_url":"https://github.com/example/app/pull/7#issuecomment-301"}\n' ;;
  *'issues/7/comments'*) printf '{"id":301,"html_url":"https://github.com/example/app/pull/7#issuecomment-301"}\n' ;;
  *) printf 'unexpected args: %s\n' "$*" >&2; exit 4 ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(launcher), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_GH_LOG", log)
	c, err := OpenCollection(collection)
	if err != nil {
		t.Fatal(err)
	}
	posted, err := c.PostReviewBundle(bundle, ReviewPostOptions{})
	if err != nil || posted.CommentID != "301" || posted.InlinePosted != 1 || posted.InlineFailed != 0 {
		t.Fatalf("post: %+v %v", posted, err)
	}
	if _, err := os.Stat(reviewReceiptPath(collection, "github", "example/app", "7", head, "301", "pass")); err != nil {
		t.Fatalf("missing local posting receipt: %v", err)
	}
	posted, err = c.PostReviewBundle(bundle, ReviewPostOptions{})
	if err != nil || posted.InlinePosted != 0 {
		t.Fatalf("duplicate inline post: %+v %v", posted, err)
	}
	resolved, err := c.ResolveReviewBundle(bundle, ReviewResolveOptions{Reply: "Fixed in the next change."})
	if err != nil || resolved.Resolved != 1 {
		t.Fatalf("resolve: %+v %v", resolved, err)
	}
	rows := readInlineRecords(bundle)
	if len(rows) != 1 || !rows[0].Resolved {
		t.Fatalf("inline record not resolved: %+v", rows)
	}
	requests, err := os.ReadFile(log)
	if err != nil || strings.Count(string(requests), "api -X POST repos/example/app/pulls/7/comments") != 2 || !strings.Contains(string(requests), "resolveReviewThread") {
		t.Fatalf("unexpected forge requests: %s %v", requests, err)
	}
}
