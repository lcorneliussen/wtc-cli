package wtc

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestStatusRetirementMatchesFreshDryRunAndDoesNotWrite(t *testing.T) {
	c, target := retirementFixture(t)
	before := retirementTree(t, c.Workspace)
	snapshot := StatusSnapshot{Collection: "finished", Repos: []StatusRepo{{Collection: "finished"}}}
	// A sweep explicitly selects the fixture's sibling and cannot write its caches.
	c.statusRetirement(&snapshot, true)
	report, err := c.InspectRetirement("finished", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Retirement) != 1 {
		t.Fatalf("missing status property: %+v", snapshot)
	}
	got, want := snapshot.Retirement[0], report.Assessment
	got.ObservedAt = ""
	want.ObservedAt = ""
	if !reflect.DeepEqual(got, want) || got.State != "unknown" {
		t.Fatalf("status/dry-run differ: %+v %+v", got, want)
	}
	if !reflect.DeepEqual(before, retirementTree(t, c.Workspace)) {
		t.Fatal("assessment wrote files, hooks or Git evidence")
	}
	fixtureFile(t, filepath.Join(target, "widget", "rescue.txt"), "preserve", 0600)
	// The saved property is an old observation; a new dry-run must see new work.
	newer, err := c.InspectRetirement("finished", nil)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Retirement[0].State != "unknown" || newer.Assessment.State != "blocked" || len(newer.Assessment.Blockers) == 0 {
		t.Fatal("fresh dry-run reused cached assessment")
	}
}

func TestStatusRetirementObservationsAddBlockersWithoutClosingUnknowns(t *testing.T) {
	snapshot := StatusSnapshot{Collection: "example", Retirement: []RetirementAssessment{{Collection: "example", State: "unknown", Blockers: []string{}, Unknowns: []string{"Archive completeness and external resources unverified"}}},
		Repos:   []StatusRepo{{Collection: "example", Dir: "widget", PR: &StatusPRFacts{Number: "42"}}},
		PRs:     []StatusPRRow{{Repo: "widget", Number: "41", State: "MERGED", Follow: &StatusFollow{Stage: "tip"}}},
		Runtime: []RuntimeItem{{Collection: "example", Repo: "widget", Path: "web/server", State: "ready"}, {Collection: "other", Repo: "widget", Path: "other/server", State: "ready"}}}
	statusRetirementFacts(&snapshot)
	statusRetirementFacts(&snapshot)
	a := snapshot.Retirement[0]
	if a.State != "blocked" || len(a.Blockers) != 3 || len(a.Unknowns) != 1 || strings.Contains(strings.Join(a.Blockers, " "), "other/server") {
		t.Fatalf("lost scoped findings or unknowns: %+v", a)
	}
	text := snapshot.Markdown()
	if !strings.Contains(text, "## Retirement") || !strings.Contains(text, "wtc retire example --dry-run") {
		t.Fatalf("missing next step: %s", text)
	}
}

func TestStatusSnapshotIncludesRetirementProperty(t *testing.T) {
	c, _ := retirementFixture(t)
	s, err := c.StatusLocalSnapshot(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Retirement) != 1 || s.Retirement[0].Collection != filepath.Base(c.Collection) || s.Retirement[0].ObservedAt == "" || s.Retirement[0].State == "ready" {
		t.Fatalf("missing conservative assessment: %+v", s.Retirement)
	}
}

func TestLiveStatusSweepRetirementHasNoDuplicatesAndKeepsLeftovers(t *testing.T) {
	c, _ := retirementFixture(t)
	fixtureFile(t, filepath.Join(c.Workspace, "leftovers", RetiredMarkerFile), `{"schema_version":1}`, 0600)
	fixtureFile(t, filepath.Join(c.Workspace, "leftovers", "rescue.txt"), "preserve", 0600)
	s, _, err := c.StatusLiveSnapshot(true, true)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, a := range s.Retirement {
		counts[a.Collection]++
		if a.Collection == "leftovers" && a.State != "blocked" {
			t.Fatal("lost leftover preservation")
		}
	}
	if len(counts) != 3 || counts[filepath.Base(c.Collection)] != 1 || counts["finished"] != 1 || counts["leftovers"] != 1 {
		t.Fatalf("dropped or duplicated assessments: %+v", counts)
	}
	var missing StatusSnapshot
	c.appendRetirement(&missing, "absent")
	if len(missing.Retirement) != 1 || missing.Retirement[0].State != "unknown" || len(missing.Retirement[0].Unknowns) == 0 {
		t.Fatal("missing evidence fallback lost unknown state")
	}
}

func TestRetirementMarkdownUsesInvokedBrand(t *testing.T) {
	old := os.Args
	os.Args = []string{"taba"}
	t.Cleanup(func() { os.Args = old })
	s := StatusSnapshot{Retirement: []RetirementAssessment{{Collection: "example", State: "unknown"}}}
	text := s.Markdown()
	if !strings.Contains(text, "taba retire example --dry-run") || strings.Contains(text, "wtc retire") {
		t.Fatalf("wrong command name: %s", text)
	}
}
