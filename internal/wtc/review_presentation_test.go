package wtc

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewPresentationKeepsGateAndImportantFindingsVisible(t *testing.T) {
	m := ReviewManifest{Forge: "github", Slug: "example/app", HeadSHA: strings.Repeat("a", 40)}
	status := "`wtc-review v1 head=" + m.HeadSHA + " verdict=changes-requested blockers=1 round=1 lead=test:`"
	body := "**Local review: changes-requested** — Fix the edge.\n\n### Blockers\n\n- `src/a file.go:4` — Fails empty input.\n- `pr.md:50` — Clarify purpose.\n\n### Concerns\n\n| code | issues |\n\n### Minor / nits\n\n- Useful minor.\n\n### Addressed since previous round\n\n- Done.\n\n### Run stats\n\n| time | 1s |\n\n" + status + "\n"
	got := presentReviewSummary(body, m, map[string]bool{"src/a file.go": true})
	if !strings.HasPrefix(got, "🔴 **Local review:") || !strings.Contains(got, "/blob/"+m.HeadSHA+"/src/a%20file.go#L4") || !strings.Contains(got, "`pr.md:50`") || strings.Contains(got, "/pr.md#") {
		t.Fatalf("incorrect presentation: %s", got)
	}
	if strings.Count(got, "<details>") != 3 || strings.Count(got, "</details>") != 3 || strings.Count(got, status) != 1 || !strings.HasSuffix(got, status+"\n") {
		t.Fatalf("folds or gate record corrupted: %s", got)
	}
	if strings.Index(got, "### Blockers") > strings.Index(got, "<details>") || !strings.Contains(got, "</details>\n\n### Minor / nits") {
		t.Fatalf("open findings hidden: %s", got)
	}
	if got := presentReviewSummary(body, ReviewManifest{Forge: "bitbucket"}, nil); got != body {
		t.Fatal("Bitbucket received GitHub HTML")
	}
}

func TestReviewSourceLinksPreserveWhitespaceInGitFilenames(t *testing.T) {
	repo := t.TempDir()
	fixtureGit(t, "-C", repo, "init", "-b", "main")
	fixtureGit(t, "-C", repo, "config", "user.email", "example@example.invalid")
	fixtureGit(t, "-C", repo, "config", "user.name", "Example")
	for _, name := range []string{"\tfirst.go", " leading.go", "trailing.go "} {
		fixtureFile(t, filepath.Join(repo, name), "package example\n", 0644)
	}
	fixtureGit(t, "-C", repo, "add", ".")
	fixtureGit(t, "-C", repo, "commit", "-m", "fixture")
	m := ReviewManifest{Forge: "github", RepoDir: repo, Slug: "example/app", HeadSHA: fixtureGit(t, "-C", repo, "rev-parse", "HEAD")}
	files := reviewSourceFiles(m)
	for _, name := range []string{"\tfirst.go", " leading.go", "trailing.go "} {
		if !files[name] {
			t.Fatalf("Git filename changed: %q in %+v", name, files)
		}
	}
	if files[""] {
		t.Fatal("empty NUL field became a source file")
	}
	body := "**Local review: pass**\n\n`\tfirst.go:1` and ` leading.go:1`\n"
	got := presentReviewSummary(body, m, files)
	if !strings.Contains(got, "/%09first.go#L1") || !strings.Contains(got, "/%20leading.go#L1") {
		t.Fatalf("whitespace file source links missing: %s", got)
	}
}

func TestReviewPresentationDoesNotRewriteCodeExamples(t *testing.T) {
	body := "**Local review: pass**\n\n```md\n### Concerns\n`feature.go:9`\n```\n\n### Concerns\n\nChecked.\n\n`wtc-review v1 head=aaaaaaaaaaaa verdict=pass blockers=0 round=1`\n"
	got := presentReviewSummary(body, ReviewManifest{Forge: "github"}, map[string]bool{"feature.go": true})
	if strings.Contains(got, "/blob/") || strings.Count(got, "<details>") != 1 || !strings.Contains(got, "```md\n### Concerns\n`feature.go:9`\n```") {
		t.Fatalf("code example rewritten: %s", got)
	}
}
