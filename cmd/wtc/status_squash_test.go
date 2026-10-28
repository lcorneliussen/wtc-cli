package main

import (
	"github.com/lcorneliussen/wtc-cli/internal/wtc"
	"strings"
	"testing"
)

func TestStatusSquashTableAndDenseNotes(t *testing.T) {
	for _, state := range []string{"landed", "follow-up", "unknown"} {
		snapshot := wtc.StatusSnapshot{Collection: "sample", Repos: []wtc.StatusRepo{{Dir: "widget", Branch: "topic", BranchDisplay: "topic [gone]", Ahead: 3, AheadRef: "origin/main", AheadBasis: "development-tip", UpstreamState: "gone", Landing: &wtc.StatusLanding{State: state}}}, PRs: []wtc.StatusPRRow{{Repo: "widget", Number: "7", State: "MERGED", OnBranch: true, Landing: &wtc.StatusLanding{State: state}, DisplayTitle: state + "; synthetic change"}}}
		table := statusTable(snapshot, false, 140)
		want := map[string]string{"landed": "catch-up", "follow-up": "follow-up", "unknown": "unknown"}[state]
		if !strings.Contains(table, want) || !strings.Contains(table, "upstream gone") || !strings.Contains(table, "↑3 vs origin/main") {
			t.Fatalf("%s table:\n%s", state, table)
		}
		dense := strings.Join(statusSyncNotes(snapshot, 48), "\n")
		if !strings.Contains(dense, "upstream gone") {
			t.Fatalf("dense observation lost: %s", dense)
		}
		markdown := snapshot.Markdown()
		if !strings.Contains(markdown, "vs origin/main") || !strings.Contains(markdown, snapshot.PRs[0].LandingLabel()) {
			t.Fatalf("markdown: %s", markdown)
		}
	}
}
