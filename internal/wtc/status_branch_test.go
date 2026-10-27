package wtc

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
  view)
    case "$*" in *mergeable,*) ;; *) exit 2 ;; esac
    printf '{"number":7,"state":"OPEN","title":"Synthetic PR","mergeable":"CONFLICTING","mergeStateStatus":"DRAFT","statusCheckRollup":[{"conclusion":"SUCCESS","status":"COMPLETED"}]}\n' ;;
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
	if detail.State != "OPEN" || detail.Checks != "SUCCESS" || detail.Merge != "CONFLICTING" || detail.Title != "Synthetic PR" {
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
	if number, err := statusDiscoverBranch("github.com", "example/widget", "different"); err != nil || number != "" {
		t.Fatalf("branch absent from a successful list: %q %v", number, err)
	}
	if _, err := statusDiscoverBranch("github.com", "example/other", "topic"); err == nil {
		t.Fatal("failed discovery was treated as a negative result")
	}
}

func TestStatusBitbucketDiscoveryReadsPastFirstPage(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("BB_USERNAME", "builder")
	t.Setenv("BB_API_TOKEN", "token")
	var sawFields bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Query().Get("fields"), "updated_on") && strings.Contains(r.URL.Query().Get("fields"), "comment_count") {
			sawFields = true
		}
		if r.URL.Query().Get("page") == "2" {
			w.Write([]byte(`{"values":[{"id":99,"state":"OPEN","updated_on":"2026-10-01T00:00:00Z","source":{"branch":{"name":"topic"}}}]}`))
			return
		}
		items := make([]string, 50)
		for i := range items {
			items[i] = fmt.Sprintf(`{"id":%d,"state":"OPEN","updated_on":"2026-10-01T00:00:00Z","source":{"branch":{"name":"other-%d"}}}`, i+1, i)
		}
		fmt.Fprintf(w, `{"values":[%s],"next":"http://%s%s?page=2"}`, strings.Join(items, ","), r.Host, r.URL.Path)
	}))
	defer server.Close()
	old := statusBBAPI
	statusBBAPI = server.URL
	defer func() { statusBBAPI = old }()
	number, err := statusDiscoverBranch("bitbucket.org", "example/widget", "topic")
	if err != nil || number != "99" || !sawFields {
		t.Fatalf("branch beyond first page was missed: %q %v fields %v", number, err, sawFields)
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
