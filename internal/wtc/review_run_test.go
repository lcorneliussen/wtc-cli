package wtc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewRunnerUsesSeparateProcessResultsAndDowngradesErrors(t *testing.T) {
	collection := filepath.Join(t.TempDir(), "sample")
	harness := filepath.Join(collection, "harness")
	bundle := filepath.Join(collection, "bundle")
	for _, dir := range []string{harness, filepath.Join(harness, "review", "prompts"), filepath.Join(bundle, "concerns")} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(harness, ".harness-repos.yml"), []byte("repos:\n  - name: app\n    remote: https://github.com/example/app.git\n"), 0644); err != nil {
		t.Fatal(err)
	}
	manifest := ReviewManifest{Repo: "app", HeadSHA: "1234567890abcdef1234567890abcdef12345678", Round: 1, RepoDir: filepath.Join(collection, "app")}
	if err := writeReviewManifest(bundle, manifest); err != nil {
		t.Fatal(err)
	}
	for path, body := range map[string]string{
		filepath.Join(harness, "review", "prompts", "concern.md"): "{{FINDINGS_FILE}}\n{{CONCERN_ID}}\n",
		filepath.Join(harness, "review", "prompts", "lead.md"):    "{{SUMMARY_FILE}}\n{{VERDICT_FILE}}\n",
		filepath.Join(bundle, "concerns", "code.md"):              "---\ntier: fast\n---\nReview code.\n",
		filepath.Join(bundle, "concerns", "compat.md"):            "---\nneeds: downstream\n---\nReview compatibility.\n",
	} {
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	launcher := filepath.Join(collection, "review-agent.sh")
	script := `#!/bin/sh
if [ "$(basename "$3")" = lead.md ]; then
  summary=$(sed -n '1p' "$3")
  verdict=$(sed -n '2p' "$3")
  printf '**Local review: pass**\n' > "$summary"
  printf 'pass\n' > "$verdict"
  exit 0
fi
findings=$(sed -n '1p' "$3")
id=$(sed -n '2p' "$3")
printf '{"concern":"%s","status":"ok","findings":[]}\n' "$id" > "$findings"
if [ "${FAIL_CONCERN:-}" = "$id" ]; then exit 3; fi
`
	if err := os.WriteFile(launcher, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WTC_REVIEW_AGENT_CMD", launcher)
	c, err := OpenCollection(collection)
	if err != nil {
		t.Fatal(err)
	}
	opt := ReviewRunOptions{Strong: "test:", Standard: "test:", Fast: "test:", Lead: "test:", Parallel: 2}
	got, err := c.RunReviewBundle(bundle, opt)
	if err != nil || got.Verdict != "pass" || got.Concerns != 2 {
		t.Fatalf("healthy run: %+v %v", got, err)
	}
	skipped, err := os.ReadFile(filepath.Join(bundle, "findings", "compat.json"))
	if err != nil || !strings.Contains(string(skipped), `"skipped"`) {
		t.Fatalf("downstream concern was not skipped: %s %v", skipped, err)
	}
	t.Setenv("FAIL_CONCERN", "code")
	got, err = c.RunReviewBundle(bundle, opt)
	if err != nil || got.Verdict != "pass-with-notes" {
		t.Fatalf("failed concern could pass: %+v %v", got, err)
	}
	var file reviewFindingFile
	data, err := os.ReadFile(filepath.Join(bundle, "findings", "code.json"))
	if err != nil || json.Unmarshal(data, &file) != nil || file.Status != "error" {
		t.Fatalf("failed concern result: %s %v", data, err)
	}
	summary, err := os.ReadFile(filepath.Join(bundle, "summary.md"))
	if err != nil || strings.Count(string(summary), "wtc-review v1 head=") != 1 {
		t.Fatalf("status line missing or duplicated: %s %v", summary, err)
	}
}
