package wtc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStatusBranchListMatchesExactSourceBranch(t *testing.T) {
	gh := []byte(`[{"number":2,"headRefName":"other"},{"number":7,"headRefName":"topic"}]`)
	number, err := statusParseBranchList("github.com", gh, "topic")
	if err != nil || number != "7" {
		t.Fatalf("wrong GitHub branch match: %q %v", number, err)
	}
	bb := []byte(`{"pullRequests":[{"id":3,"source":{"branch":{"name":"other"}}},{"id":9,"source":{"branch":{"name":"topic"}}}]}`)
	number, err = statusParseBranchList("bitbucket.org", bb, "topic")
	if err != nil || number != "9" {
		t.Fatalf("wrong Bitbucket branch match: %q %v", number, err)
	}
	if number, err := statusParseBranchList("github.com", gh, "topic-more"); err != nil || number != "" {
		t.Fatalf("substring branch match: %q %v", number, err)
	}
	if _, err := statusParseBranchList("github.com", []byte(`{}`), "topic"); err == nil {
		t.Fatal("malformed GitHub list accepted as empty")
	}
}

func TestStatusBranchDiscoveryCachesOnlyValidAnswers(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	bin := t.TempDir()
	gh := filepath.Join(bin, "gh")
	script := `#!/bin/sh
case "$2" in
  list) printf '[{"number":7,"headRefName":"topic"}]\n' ;;
  view) printf '{"number":7,"state":"OPEN","title":"Synthetic PR","statusCheckRollup":[{"conclusion":"SUCCESS","status":"COMPLETED"}]}\n' ;;
  *) exit 2 ;;
esac
`
	if err := os.WriteFile(gh, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	number, err := statusDiscoverBranch("github.com", "example/widget", "topic")
	if err != nil || number != "7" {
		t.Fatalf("discovery failed: %q %v", number, err)
	}
	detail := statusEnrichRecord(PRRecord{Number: number}, "example/widget", "github.com")
	if detail.State != "OPEN" || detail.Checks != "SUCCESS" || detail.Title != "Synthetic PR" {
		t.Fatalf("enrichment failed: %+v", detail)
	}
	if err := os.WriteFile(gh, []byte("#!/bin/sh\nexit 2\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if cached, err := statusDiscoverBranch("github.com", "example/widget", "topic"); err != nil || cached != number {
		t.Fatalf("valid discovery cache missed: %q %v", cached, err)
	}
	if cached := statusEnrichRecord(PRRecord{Number: number}, "example/widget", "github.com"); cached.State != "OPEN" {
		t.Fatalf("valid detail cache missed: %+v", cached)
	}
	if _, err := statusDiscoverBranch("github.com", "example/widget", "different"); err == nil {
		t.Fatal("failed discovery was treated as a negative result")
	}
}

func TestStatusForgePreviewFindsUnenlistedBranchPR(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	bin := t.TempDir()
	gh := filepath.Join(bin, "gh")
	script := `#!/bin/sh
case "$2" in
  list) printf '[{"number":7,"headRefName":"topic"}]\n' ;;
  view) printf '{"number":7,"state":"OPEN","title":"Synthetic PR","statusCheckRollup":[{"conclusion":"SUCCESS","status":"COMPLETED"}]}\n' ;;
  *) exit 2 ;;
esac
`
	if err := os.WriteFile(gh, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	c := newWorkspaceFixture(t)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	c.Registry.Repos[0].Remote = "https://github.com/example/agent-harness.git"
	fixtureGit(t, "-C", c.Harness, "switch", "-qc", "topic")
	snapshot, err := c.StatusForgePreview()
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.PRs) != 0 {
		t.Fatalf("unenlisted PR entered the enlisted section: %+v", snapshot.PRs)
	}
	for _, row := range snapshot.Repos {
		if row.Dir == "harness" {
			if row.PR == nil || row.PR.Number != "7" || row.PR.Checks != "SUCCESS" {
				t.Fatalf("open branch PR not shown inline: %+v", row.PR)
			}
			return
		}
	}
	t.Fatal("harness row missing")
}
