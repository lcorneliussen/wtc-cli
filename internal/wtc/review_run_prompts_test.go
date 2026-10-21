package wtc

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestPrivateReviewRunFallsBackToEmbeddedPrompts(t *testing.T) {
	collection := filepath.Join(t.TempDir(), "task")
	harness := filepath.Join(collection, "harness")
	for _, name := range []string{"harness", "app"} {
		if err := os.MkdirAll(filepath.Join(collection, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(harness, ".harness-repos.yml"), []byte("repos:\n  - name: app\n    remote: https://github.com/example/app.git\n    default_ref: main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	write := func(path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	git := func(args ...string) {
		t.Helper()
		if output, err := exec.Command("git", append([]string{"-C", filepath.Join(collection, "app")}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s: %v", args, output, err)
		}
	}
	git("init", "-b", "main")
	git("config", "user.name", "Fixture")
	git("config", "user.email", "fixture@example.invalid")
	write(filepath.Join(collection, "app", "old.txt"), "base\n")
	git("add", ".")
	git("commit", "-m", "base")
	git("switch", "-c", "feature")
	write(filepath.Join(collection, "app", "feature.go"), "package feature\n")
	git("add", ".")
	git("commit", "-m", "feature")
	write(filepath.Join(harness, "review", "concerns", "code.md"), "---\nid: code\napplies: '*.go'\n---\nReview code.\n")

	// The launcher derives its output paths from the prompt path and fails
	// unless the prompt is the expected template, so a missing or wrong
	// template cannot pass.
	launcher := filepath.Join(collection, "review-agent.sh")
	write(launcher, `#!/bin/sh
bundle=$(dirname "$(dirname "$3")")
id=$(basename "$3" .md)
if [ "$id" = lead ]; then
  grep -q "${WANT_LEAD:?}" "$3" || exit 4
  printf '**Local review: pass**\n' > "$bundle/summary.md"
  printf 'pass\n' > "$bundle/verdict"
  exit 0
fi
grep -q 'You review one concern only' "$3" || exit 4
printf '{"concern":"%s","status":"ok","findings":[]}\n' "$id" > "$bundle/findings/$id.json"
`)
	if err := os.Chmod(launcher, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WTC_REVIEW_AGENT_CMD", launcher)
	c, err := OpenCollection(collection)
	if err != nil {
		t.Fatal(err)
	}
	opt := ReviewRunOptions{Strong: "test:", Standard: "test:", Fast: "test:", Lead: "test:", Parallel: 1}
	run := func() ReviewRunResult {
		t.Helper()
		bundle, err := c.BuildReviewBundle(ReviewBundleOptions{Repo: "app", Base: "main", NoCatchUp: true})
		if err != nil {
			t.Fatal(err)
		}
		if bundle.Manifest.Public {
			t.Fatalf("expected a private bundle: %+v", bundle.Manifest)
		}
		got, err := c.RunReviewBundle(bundle.Dir, opt)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}

	t.Setenv("WANT_LEAD", "You are the lead reviewer")
	if got := run(); got.Verdict != "pass" || got.Concerns != 1 {
		t.Fatalf("private run without harness prompts: %+v", got)
	}

	write(filepath.Join(harness, "review", "prompts", "lead.md"), "Harness lead override.\n{{SUMMARY_FILE}}\n")
	t.Setenv("WANT_LEAD", "Harness lead override")
	if got := run(); got.Verdict != "pass" {
		t.Fatalf("harness lead override with default concern prompt: %+v", got)
	}
}
