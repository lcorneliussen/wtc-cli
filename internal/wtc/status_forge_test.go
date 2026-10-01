package wtc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStatusEnrichRecordsKeepsIdentityAcrossOutOfOrderReplies(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	bin := t.TempDir()
	script := `#!/bin/sh
case "$3" in 1) sleep 0.2 ;; esac
printf '{"number":%s,"state":"OPEN","title":"Synthetic %s"}\n' "$3" "$3"
`
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	var progress []string
	c := &Context{Registry: Registry{Repos: []Repo{{Name: "widget", Remote: "https://github.com/example/widget.git"}}},
		StatusProgress: func(message string) { progress = append(progress, message) }}
	records := make([]PRRecord, 6)
	for i := range records {
		records[i] = PRRecord{Repo: "widget", Number: fmt.Sprint(i + 1)}
	}
	details, err := c.statusEnrichRecords(records, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i, detail := range details {
		want := fmt.Sprintf("Synthetic %d", i+1)
		if detail.Number != records[i].Number || detail.Title != want {
			t.Fatalf("detail %d crossed records: %+v", i, detail)
		}
	}
	if len(progress) != len(records) || !strings.Contains(progress[len(progress)-1], "6/6") {
		t.Fatalf("missing completion progress: %v", progress)
	}
}

func TestStatusGHDetailSeparatesChecksReviewAndMerge(t *testing.T) {
	raw := []byte(`{"number":7,"state":"OPEN","title":"Change widget","isDraft":false,"reviewDecision":"REVIEW_REQUIRED","mergeStateStatus":"BEHIND","reviewRequests":[{"login":"reviewer"}],"statusCheckRollup":[{"conclusion":"SUCCESS","status":"COMPLETED"},{"status":"IN_PROGRESS"}]}`)
	d, err := statusGHDetail(raw, PRRecord{Number: "7"})
	if err != nil {
		t.Fatal(err)
	}
	if d.Number != "7" || d.State != "OPEN" || d.Checks != "PENDING" || d.Merge != "BEHIND" || d.Review != "waiting" || d.Title != "Change widget" {
		t.Fatalf("wrong GitHub detail: %+v", d)
	}
	failed := []byte(`{"number":7,"state":"OPEN","statusCheckRollup":[{"conclusion":"FAILURE","status":"COMPLETED"},{"status":"IN_PROGRESS"}]}`)
	d, err = statusGHDetail(failed, PRRecord{Title: "fallback"})
	if err != nil || d.Checks != "FAILURE" || d.Review != "noreviewers" || d.Title != "fallback" {
		t.Fatalf("failed check or reviewer state lost: %+v %v", d, err)
	}
	pendingState := []byte(`{"number":7,"state":"OPEN","statusCheckRollup":[{"conclusion":"SUCCESS","status":"COMPLETED"},{"state":"PENDING"}]}`)
	d, err = statusGHDetail(pendingState, PRRecord{})
	if err != nil || d.Checks != "PENDING" {
		t.Fatalf("pending state hidden by passing check: %+v %v", d, err)
	}
}

func TestStatusDraftMergedAndUnknownFacts(t *testing.T) {
	draft, err := statusGHDetail([]byte(`{"number":8,"state":"OPEN","isDraft":true}`), PRRecord{})
	if err != nil || draft.State != "DRAFT" || draft.Checks != "draft" || draft.Review != "none" {
		t.Fatalf("wrong draft: %+v %v", draft, err)
	}
	merged, err := statusGHDetail([]byte(`{"number":8,"state":"MERGED","mergedAt":"2026-09-25T12:00:00Z"}`), PRRecord{})
	if err != nil || merged.Merge != "MERGED" || merged.Review != "merged" || merged.MergedOn == "" {
		t.Fatalf("wrong merged facts: %+v %v", merged, err)
	}
	if _, err := statusGHDetail([]byte(`{}`), PRRecord{}); err == nil {
		t.Fatal("missing forge identity must not become a healthy PR")
	}
	bb, err := statusBBDetail([]byte(`{"id":9,"state":"OPEN","participants":[{"approved":true}]}`), PRRecord{Title: "fallback"})
	if err != nil || bb.Number != "9" || bb.Review != "approved" || bb.Merge != "UNKNOWN" || bb.Title != "fallback" {
		t.Fatalf("wrong Bitbucket facts: %+v %v", bb, err)
	}
	bbMerged, err := statusBBDetail([]byte(`{"id":9,"state":"MERGED","updated_on":"2026-09-30T12:00:00Z","merged_on":"2026-09-20T12:00:00Z"}`), PRRecord{})
	if err != nil || bbMerged.MergedOn != "2026-09-20T12:00:00Z" {
		t.Fatalf("merge time replaced by last update: %+v %v", bbMerged, err)
	}
}

func TestStatusArchiveCountsWeekdayHours(t *testing.T) {
	friday := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	monday := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	wednesday := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	if got := statusWeekdayHours(friday, monday); got != 24 {
		t.Fatalf("weekend counted: %v", got)
	}
	if statusArchived(friday.Format(time.RFC3339), monday) || !statusArchived(friday.Format(time.RFC3339), wednesday) {
		t.Fatal("archive window ignored weekday hours")
	}
	t.Setenv("WTC_PR_ARCHIVE_HOURS", "12")
	if !statusArchived(friday.Format(time.RFC3339), monday) {
		t.Fatal("configured weekday archive window ignored")
	}
}

func TestStatusEnlistedSnapshotPreservesUnknownForgeState(t *testing.T) {
	c := newWorkspaceFixture(t)
	fixtureGit(t, "-C", c.Harness, "switch", "-qc", "topic")
	if _, err := c.EnlistPR(PRRecord{Repo: "agent-harness", Number: "12", Branch: "topic", Title: "Synthetic change"}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := c.StatusForgePreview()
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.PRs) != 1 || snapshot.PRs[0].Merge == nil || *snapshot.PRs[0].Merge != "UNKNOWN" || snapshot.PRs[0].Title != "Synthetic change" {
		t.Fatalf("unknown forge state was not preserved: %+v", snapshot.PRs)
	}
	for _, repo := range snapshot.Repos {
		if repo.Dir == "harness" && repo.PR != nil {
			t.Fatalf("unknown forge facts were claimed as a live PR: %+v", repo.PR)
		}
	}
}
