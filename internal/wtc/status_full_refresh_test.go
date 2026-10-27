package wtc

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStatusFullRefreshLogsWhatTheListMissed(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	t.Setenv("WTC_FORGE_FULL_REFRESH", "0")
	listPath := filepath.Join(t.TempDir(), "list.json")
	views := filepath.Join(t.TempDir(), "views")
	open := `[{"number":7,"title":"Listed","state":"OPEN","updatedAt":"2026-10-01T00:00:00Z","reviewDecision":"APPROVED","headRefName":"topic","statusCheckRollup":[{"conclusion":"SUCCESS","status":"COMPLETED"}]}]`
	if err := os.WriteFile(listPath, []byte(open), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WTC_TEST_GH_LIST", listPath)
	t.Setenv("WTC_TEST_GH_VIEWS", views)
	script := `#!/bin/sh
case "$2" in
  list) cat "$WTC_TEST_GH_LIST" ;;
  view) printf 'viewed\n' >> "$WTC_TEST_GH_VIEWS"; printf '%s\n' '{"number":7,"state":"OPEN","title":"Listed","reviewDecision":"APPROVED","statusCheckRollup":[{"conclusion":"FAILURE","status":"COMPLETED"}]}' ;;
  *) exit 2 ;;
esac
`
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	slug := "example/widget"
	record := PRRecord{Number: "7"}
	incremental := statusEnrichRecord(record, slug, "github.com")
	if incremental.Checks != "SUCCESS" || statusTestFileLines(t, views) != 0 {
		t.Fatalf("incremental read fetched the pull request: %+v views=%d", incremental, statusTestFileLines(t, views))
	}
	t.Setenv("WTC_FORGE_FULL_REFRESH", "600")
	full := statusEnrichRecord(record, slug, "github.com")
	if full.Checks != "FAILURE" || statusTestFileLines(t, views) != 1 {
		t.Fatalf("full refresh did not replace the list: %+v views=%d", full, statusTestFileLines(t, views))
	}
	logText := statusReadAuditLog(t)
	if !strings.Contains(logText, "incremental miss github.com example/widget #7 stamp checks:SUCCESS->FAILURE") {
		t.Fatalf("miss was not logged: %s", logText)
	}
	if again := statusEnrichRecord(record, slug, "github.com"); again.Checks != "FAILURE" || statusTestFileLines(t, views) != 1 {
		t.Fatalf("full refresh repeated inside the interval: %+v views=%d", again, statusTestFileLines(t, views))
	}
	past := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(statusForgeEntryPath("full", "github.com", slug, "7"), past, past); err != nil {
		t.Fatal(err)
	}
	if agreed := statusEnrichRecord(record, slug, "github.com"); agreed.Checks != "FAILURE" || statusTestFileLines(t, views) != 2 {
		t.Fatalf("aged full refresh did not run: %+v views=%d", agreed, statusTestFileLines(t, views))
	}
	if strings.Count(statusReadAuditLog(t), "incremental miss") != 1 {
		t.Fatalf("an agreeing full refresh was logged as a miss: %s", statusReadAuditLog(t))
	}
}

func TestStatusFullRefreshLogsAnOpenPullRequestMissingFromTheList(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	t.Setenv("WTC_FORGE_FULL_REFRESH", "600")
	views := filepath.Join(t.TempDir(), "views")
	t.Setenv("WTC_TEST_GH_VIEWS", views)
	script := `#!/bin/sh
case "$2" in
  list) printf '[]\n' ;;
  view) printf 'viewed\n' >> "$WTC_TEST_GH_VIEWS"; printf '%s\n' '{"number":7,"state":"OPEN","title":"Still open"}' ;;
  *) exit 2 ;;
esac
`
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	detail := statusEnrichRecord(PRRecord{Number: "7"}, "example/widget", "github.com")
	if detail.State != "OPEN" || !strings.Contains(statusReadAuditLog(t), "list-omitted state:OPEN") {
		t.Fatalf("omitted open pull request was not recorded: %+v log=%s", detail, statusReadAuditLog(t))
	}
}

func TestStatusPipelineFullRefreshLogsWhatThePageMissed(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	t.Setenv("WTC_FORGE_FULL_REFRESH", "600")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("BB_USERNAME", "builder")
	t.Setenv("BB_API_TOKEN", "token")
	hits, targeted := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Query().Get("target.ref_name") == "main" {
			targeted++
			w.Write([]byte(`{"values":[{"build_number":12,"state":{"name":"COMPLETED","result":{"name":"FAILED"}},"target":{"ref_name":"main","selector":{"type":"branches"},"commit":{"hash":"ccccccc"}}}]}`))
			return
		}
		w.Write([]byte(`{"values":[{"build_number":11,"state":{"name":"COMPLETED","result":{"name":"SUCCESSFUL"}},"target":{"ref_name":"main","selector":{"type":"branches"},"commit":{"hash":"bbbbbbb"}}}]}`))
	}))
	defer server.Close()
	old := statusBBAPI
	statusBBAPI = server.URL
	defer func() { statusBBAPI = old }()
	first, err := statusBBBuild("team/widget", "main")
	if err != nil || first == nil || *first.Build != "12" || *first.Checks != "FAILURE" || hits != 2 || targeted != 1 {
		t.Fatalf("full pipeline read = %+v hits=%d targeted=%d err=%v", first, hits, targeted, err)
	}
	logText := statusReadAuditLog(t)
	if !strings.Contains(logText, "incremental miss bitbucket.org team/widget main page build:11->12 checks:SUCCESS->FAILURE") {
		t.Fatalf("pipeline miss was not logged: %s", logText)
	}
	second, err := statusBBBuild("team/widget", "main")
	if err != nil || second == nil || *second.Build != "12" || hits != 2 || targeted != 1 {
		t.Fatalf("pipeline audit repeated inside the interval: %+v hits=%d targeted=%d err=%v", second, hits, targeted, err)
	}
}

func statusReadAuditLog(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(statusIncrementalMissLog())
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
