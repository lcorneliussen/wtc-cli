package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewRunPostUpdatesOneCommentOnSuccessAndFailure(t *testing.T) {
	collection := filepath.Join(t.TempDir(), "sample")
	for _, dir := range []string{
		filepath.Join(collection, "harness"),
		filepath.Join(collection, "app", ".git"),
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(collection, "harness", ".harness-repos.yml"), []byte("repos:\n  - name: app\n    remote: https://github.com/example/app.git\n"), 0644); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	forgeLog := filepath.Join(bin, "forge.log")
	gh := `#!/bin/sh
case "$*" in
  *'pr view 7'*'--json headRefOid'*) echo '{"headRefOid":"1234567890abcdef1234567890abcdef12345678"}' ;;
  *'pulls/7/comments?per_page=100'*) echo '[[]]' ;;
  *'issues/7/comments'*)
    body=$(cat)
    printf 'POST %s\n' "$body" >> "$FAKE_FORGE_LOG"
    echo '{"id":301,"html_url":"https://github.com/example/app/pull/7#issuecomment-301"}' ;;
  *'issues/comments/301'*)
    body=$(cat)
    printf 'PATCH %s\n' "$body" >> "$FAKE_FORGE_LOG"
    echo '{"id":301,"html_url":"https://github.com/example/app/pull/7#issuecomment-301"}' ;;
  *) echo "unexpected gh args: $*" >&2; exit 2 ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(gh), 0755); err != nil {
		t.Fatal(err)
	}
	agent := `#!/bin/sh
case "$(basename "$3")" in
  lead.md)
    if [ "${FAKE_REVIEW_FAIL:-}" = 1 ]; then exit 3; fi
    printf '**Local review: pass**\n' > "$4/summary.md"
    printf 'pass\n' > "$4/verdict" ;;
  *)
    printf '{"concern":"code","status":"ok","findings":[]}\n' > "$4/findings/code.json" ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "review-agent"), []byte(agent), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_FORGE_LOG", forgeLog)
	t.Setenv("WTC_REVIEW_AGENT_CMD", filepath.Join(bin, "review-agent"))
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	for _, tc := range []struct {
		name     string
		fail     bool
		lastLine string
	}{
		{"success", false, "verdict=pass"},
		{"failure", true, "verdict=error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bundle := filepath.Join(collection, tc.name)
			if err := os.MkdirAll(filepath.Join(bundle, "concerns"), 0755); err != nil {
				t.Fatal(err)
			}
			manifest := map[string]any{
				"repo": "app", "pr": "7", "forge": "github", "slug": "example/app",
				"head_sha": "1234567890abcdef1234567890abcdef12345678", "round": 1,
				"repo_dir": filepath.Join(collection, "app"), "public": true,
			}
			data, _ := json.Marshal(manifest)
			if err := os.WriteFile(filepath.Join(bundle, "manifest.json"), data, 0644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(bundle, "concerns", "code.md"), []byte("---\nid: code\ntier: fast\n---\nReview code.\n"), 0644); err != nil {
				t.Fatal(err)
			}
			if tc.fail {
				t.Setenv("FAKE_REVIEW_FAIL", "1")
			} else {
				t.Setenv("FAKE_REVIEW_FAIL", "")
			}
			if err := os.WriteFile(forgeLog, nil, 0644); err != nil {
				t.Fatal(err)
			}
			os.Args = []string{"wtc", "review", "run", bundle, "--collection", collection, "--post", "--strong", "test:", "--standard", "test:", "--fast", "test:", "--lead", "test:"}
			err := run()
			if (err != nil) != tc.fail {
				t.Fatalf("run error: %v; want failure %t", err, tc.fail)
			}
			body, err := os.ReadFile(forgeLog)
			if err != nil {
				t.Fatal(err)
			}
			log := string(body)
			if strings.Count(log, "POST ") != 1 || !strings.Contains(log, "verdict=pending") || !strings.Contains(log, tc.lastLine) {
				t.Fatalf("unexpected comment lifecycle: %s", log)
			}
			if tc.fail && strings.Count(log, "PATCH ") != 1 || !tc.fail && strings.Count(log, "PATCH ") != 2 {
				t.Fatalf("expected updates on one comment: %s", log)
			}
		})
	}
}
