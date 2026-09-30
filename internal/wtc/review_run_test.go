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
	for _, dir := range []string{harness, filepath.Join(harness, "review", "prompts"), filepath.Join(bundle, "concerns"), filepath.Join(collection, "app", ".git")} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(harness, ".harness-repos.yml"), []byte("repos:\n  - name: app\n    remote: https://github.com/example/app.git\n"), 0644); err != nil {
		t.Fatal(err)
	}
	manifest := ReviewManifest{Repo: "app", Forge: "github", Slug: "example/app", HeadSHA: "1234567890abcdef1234567890abcdef12345678", Round: 1, RepoDir: filepath.Join(collection, "app")}
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
  printf '{"input_tokens":1200,"output_tokens":50,"cache_read_tokens":200,"cost_usd":0.02,"turns":2,"model":"sample-model"}\n' > "$WTC_REVIEW_STATS_FILE"
  exit 0
fi
findings=$(sed -n '1p' "$3")
id=$(sed -n '2p' "$3")
if [ "${BLOCKER_CONCERN:-}" = "$id" ]; then
  printf '{"concern":"%s","status":"issues","findings":[{"severity":"blocker","title":"Breaks execution"}]}\n' "$id" > "$findings"
  exit 0
fi
printf '{"concern":"%s","status":"ok","findings":[]}\n' "$id" > "$findings"
printf '{"input_tokens":100,"output_tokens":10,"cache_read_tokens":20,"cost_usd":0.01,"turns":1}\n' > "$WTC_REVIEW_STATS_FILE"
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
	statsData, err := os.ReadFile(filepath.Join(bundle, "stats", "code.json"))
	var stats reviewRunStats
	if err != nil || json.Unmarshal(statsData, &stats) != nil || stats.InputTokens == nil || *stats.InputTokens != 100 {
		t.Fatalf("custom launcher usage was lost: %s %v", statsData, err)
	}
	summary, err := os.ReadFile(filepath.Join(bundle, "summary.md"))
	if err != nil || !strings.Contains(string(summary), "| code | test: |") || !strings.Contains(string(summary), "| lead | test:sample-model |") || !strings.Contains(string(summary), "$0.03") {
		t.Fatalf("run statistics table missing: %s %v", summary, err)
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
	summary, err = os.ReadFile(filepath.Join(bundle, "summary.md"))
	if err != nil || strings.Count(string(summary), "wtc-review v1 head=") != 1 {
		t.Fatalf("status line missing or duplicated: %s %v", summary, err)
	}
	t.Setenv("FAIL_CONCERN", "")
	t.Setenv("BLOCKER_CONCERN", "code")
	got, err = c.RunReviewBundle(bundle, opt)
	if err != nil || got.Verdict != "changes-requested" || got.Blockers != 1 {
		t.Fatalf("open blocker did not close gate: %+v %v", got, err)
	}
	summary, err = os.ReadFile(filepath.Join(bundle, "summary.md"))
	if err != nil || !strings.Contains(string(summary), "**Local review: changes-requested**") || strings.Contains(string(summary), "**Local review: pass**") {
		t.Fatalf("runner verdict and summary heading disagree: %s %v", summary, err)
	}
}

func TestReviewStatsParseBuiltinUsage(t *testing.T) {
	t.Setenv("WTC_REVIEW_AGENT_CMD", "")
	for _, tc := range []struct {
		agent, output               string
		input, outputTokens, cached int64
	}{
		{"codex", "{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":300,\"cached_input_tokens\":100,\"output_tokens\":20}}\n{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":70,\"cached_input_tokens\":10,\"output_tokens\":5}}\n", 260, 25, 110},
		{"claude", "{\"type\":\"result\",\"usage\":{\"input_tokens\":12,\"output_tokens\":3,\"cache_read_input_tokens\":4},\"total_cost_usd\":0.05,\"num_turns\":2}", 12, 3, 4},
		{"grok", "{\"text\":\"done\",\"usage\":{\"input_tokens\":9,\"output_tokens\":2,\"cache_read_input_tokens\":1}}", 9, 2, 1},
	} {
		t.Run(tc.agent, func(t *testing.T) {
			stats := collectReviewStats(filepath.Join(t.TempDir(), "stats.json"), tc.agent, "", 0, "ok", []byte(tc.output))
			if stats.InputTokens == nil || *stats.InputTokens != tc.input || stats.OutputTokens == nil || *stats.OutputTokens != tc.outputTokens || stats.CacheReadTokens == nil || *stats.CacheReadTokens != tc.cached {
				t.Fatalf("wrong %s usage: %+v", tc.agent, stats)
			}
		})
	}
}
