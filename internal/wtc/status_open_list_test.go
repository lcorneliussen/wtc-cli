package wtc

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func statusHideBitbucketLogin(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	for _, name := range []string{"BB_API_USER", "BB_API_PASS", "BB_USERNAME", "BB_API_TOKEN", "ATLASSIAN_ACCOUNT_EMAIL", "ATLASSIAN_API_TOKEN"} {
		t.Setenv(name, "")
	}
}

func TestStatusListFingerprintNoticesCommentsAndChecks(t *testing.T) {
	bb := statusBBListItem{ID: 9, UpdatedOn: "2026-10-01T00:00:00Z", CommentCount: 1, TaskCount: 0, State: "OPEN", Title: "Listed"}
	if bb.fingerprint() != bb.fingerprint() {
		t.Fatal("Bitbucket fingerprint is unstable")
	}
	commented := bb
	commented.CommentCount = 2
	if commented.fingerprint() == bb.fingerprint() {
		t.Fatal("comment count was left out of the Bitbucket fingerprint")
	}
	gh := statusGHListItem{Number: 7, UpdatedAt: "2026-10-01T00:00:00Z", State: "OPEN", Title: "Listed"}
	gh.StatusCheckRollup = append(gh.StatusCheckRollup, struct {
		Conclusion string `json:"conclusion"`
		State      string `json:"state"`
		Status     string `json:"status"`
	}{Conclusion: "SUCCESS", Status: "COMPLETED"})
	failed := gh
	failed.StatusCheckRollup = append([]struct {
		Conclusion string `json:"conclusion"`
		State      string `json:"state"`
		Status     string `json:"status"`
	}{}, gh.StatusCheckRollup...)
	failed.StatusCheckRollup[0].Conclusion = "FAILURE"
	if gh.fingerprint() == failed.fingerprint() {
		t.Fatal("check conclusion was left out of the GitHub fingerprint")
	}
}

func TestStatusOpenListSkipsUnchangedGitHubPullRequests(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	listPath := filepath.Join(t.TempDir(), "list.json")
	views := filepath.Join(t.TempDir(), "views")
	lists := filepath.Join(t.TempDir(), "lists")
	open := `[{"number":7,"title":"Listed","state":"OPEN","updatedAt":"2026-10-01T00:00:00Z","reviewDecision":"APPROVED","headRefName":"topic","statusCheckRollup":[{"conclusion":"SUCCESS","status":"COMPLETED"}]},{"number":8,"title":"Also","state":"OPEN","updatedAt":"2026-10-01T00:00:00Z","headRefName":"other","statusCheckRollup":[{"conclusion":"SUCCESS","status":"COMPLETED"}]}]`
	if err := os.WriteFile(listPath, []byte(open), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WTC_TEST_GH_LIST", listPath)
	t.Setenv("WTC_TEST_GH_VIEWS", views)
	t.Setenv("WTC_TEST_GH_LISTS", lists)
	script := `#!/bin/sh
case "$2" in
  list) printf 'listed\n' >> "$WTC_TEST_GH_LISTS"; cat "$WTC_TEST_GH_LIST" ;;
  view) printf 'viewed\n' >> "$WTC_TEST_GH_VIEWS"; printf '%s\n' '{"number":7,"state":"MERGED","mergedAt":"2026-10-02T00:00:00Z","title":"Merged"}' ;;
  *) exit 2 ;;
esac
`
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	slug := "example/widget"
	first := statusEnrichRecord(PRRecord{Number: "7"}, slug, "github.com")
	second := statusEnrichRecord(PRRecord{Number: "8"}, slug, "github.com")
	if first.Title != "Listed" || first.Checks != "SUCCESS" || first.Review != "approved" || second.Title != "Also" {
		t.Fatalf("list did not supply both pull requests: %+v %+v", first, second)
	}
	if count := statusTestFileLines(t, lists); count != 1 || statusTestFileLines(t, views) != 0 {
		t.Fatalf("open list was not shared, lists=%d views=%d", count, statusTestFileLines(t, views))
	}
	past := time.Now().Add(-2 * time.Minute)
	if err := os.Chtimes(statusForgeEntryPath("openprs", "github.com", slug, "open"), past, past); err != nil {
		t.Fatal(err)
	}
	if again := statusEnrichRecord(PRRecord{Number: "7"}, slug, "github.com"); again.Title != "Listed" || again.Checks != "SUCCESS" {
		t.Fatalf("unchanged list replaced the saved detail: %+v", again)
	}
	if statusTestFileLines(t, views) != 0 || statusTestFileLines(t, lists) != 2 {
		t.Fatalf("unchanged pull request was viewed, lists=%d views=%d", statusTestFileLines(t, lists), statusTestFileLines(t, views))
	}
	failed := `[{"number":7,"title":"Listed","state":"OPEN","updatedAt":"2026-10-03T00:00:00Z","reviewDecision":"APPROVED","headRefName":"topic","statusCheckRollup":[{"conclusion":"FAILURE","status":"COMPLETED"}]}]`
	if err := os.WriteFile(listPath, []byte(failed), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(statusForgeEntryPath("openprs", "github.com", slug, "open"), past, past); err != nil {
		t.Fatal(err)
	}
	if updated := statusEnrichRecord(PRRecord{Number: "7"}, slug, "github.com"); updated.Checks != "FAILURE" || statusTestFileLines(t, views) != 0 {
		t.Fatalf("changed checks did not come from the list: %+v views=%d", updated, statusTestFileLines(t, views))
	}
	if err := os.WriteFile(listPath, []byte("[]"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(statusForgeEntryPath("openprs", "github.com", slug, "open"), past, past); err != nil {
		t.Fatal(err)
	}
	merged := statusEnrichRecord(PRRecord{Number: "7"}, slug, "github.com")
	if merged.State != "MERGED" || merged.MergedOn != "2026-10-02T00:00:00Z" || statusTestFileLines(t, views) != 1 {
		t.Fatalf("leaving the open list did not fetch the pull request: %+v views=%d", merged, statusTestFileLines(t, views))
	}
}

func TestStatusOpenListSkipsUnchangedBitbucketPullRequests(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("BB_USERNAME", "builder")
	t.Setenv("BB_API_TOKEN", "token")
	var phase atomic.Int32
	var listHits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		listHits.Add(1)
		fields := r.URL.Query().Get("fields")
		if !strings.Contains(fields, "updated_on") || !strings.Contains(fields, "comment_count") || !strings.Contains(fields, "task_count") {
			t.Errorf("list is missing comparison fields: %s", r.URL.RawQuery)
		}
		switch phase.Load() {
		case 0, 1:
			fmt.Fprint(w, bitbucketOpenPR(0, "Listed"))
		case 2:
			fmt.Fprint(w, bitbucketOpenPR(2, "Commented"))
		default:
			fmt.Fprint(w, `{"values":[]}`)
		}
	}))
	defer server.Close()
	old := statusBBAPI
	statusBBAPI = server.URL
	defer func() { statusBBAPI = old }()
	views := filepath.Join(t.TempDir(), "views")
	t.Setenv("WTC_TEST_BB_VIEWS", views)
	script := `#!/bin/sh
case "$2" in
  view) printf 'viewed\n' >> "$WTC_TEST_BB_VIEWS"; printf '%s\n' '{"id":9,"state":"MERGED","merged_on":"2026-10-02T00:00:00Z","title":"Merged"}' ;;
  *) exit 2 ;;
esac
`
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "bb"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	slug := "example/widget"
	first := statusEnrichRecord(PRRecord{Number: "9"}, slug, "bitbucket.org")
	if first.Title != "Listed" || first.Review != "approved" || first.State != "OPEN" || listHits.Load() != 1 || statusTestFileLines(t, views) != 0 {
		t.Fatalf("list did not replace the view: %+v hits=%d views=%d", first, listHits.Load(), statusTestFileLines(t, views))
	}
	phase.Store(1)
	past := time.Now().Add(-2 * time.Minute)
	if err := os.Chtimes(statusForgeEntryPath("openprs", "bitbucket.org", slug, "open"), past, past); err != nil {
		t.Fatal(err)
	}
	if again := statusEnrichRecord(PRRecord{Number: "9"}, slug, "bitbucket.org"); again.Title != "Listed" || listHits.Load() != 2 || statusTestFileLines(t, views) != 0 {
		t.Fatalf("unchanged pull request was viewed: %+v hits=%d views=%d", again, listHits.Load(), statusTestFileLines(t, views))
	}
	phase.Store(2)
	if err := os.Chtimes(statusForgeEntryPath("openprs", "bitbucket.org", slug, "open"), past, past); err != nil {
		t.Fatal(err)
	}
	if updated := statusEnrichRecord(PRRecord{Number: "9"}, slug, "bitbucket.org"); updated.Title != "Commented" || statusTestFileLines(t, views) != 0 {
		t.Fatalf("comment change did not come from the list: %+v views=%d", updated, statusTestFileLines(t, views))
	}
	phase.Store(3)
	if err := os.Chtimes(statusForgeEntryPath("openprs", "bitbucket.org", slug, "open"), past, past); err != nil {
		t.Fatal(err)
	}
	merged := statusEnrichRecord(PRRecord{Number: "9"}, slug, "bitbucket.org")
	if merged.State != "MERGED" || merged.MergedOn != "2026-10-02T00:00:00Z" || statusTestFileLines(t, views) != 1 {
		t.Fatalf("leaving the open list did not fetch the pull request: %+v views=%d", merged, statusTestFileLines(t, views))
	}
}

func bitbucketOpenPR(comments int, title string) string {
	return fmt.Sprintf(`{"values":[{"id":9,"state":"OPEN","draft":false,"title":%q,"updated_on":"2026-10-01T00:00:00Z","comment_count":%d,"task_count":0,"source":{"branch":{"name":"topic"}},"destination":{"branch":{"name":"main"}},"participants":[{"approved":true,"state":"approved"}]}]}`, title, comments)
}

func statusTestFileLines(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	lines := 0
	for _, b := range data {
		if b == '\n' {
			lines++
		}
	}
	return lines
}
