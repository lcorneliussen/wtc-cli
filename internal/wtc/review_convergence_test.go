package wtc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func convergenceFixtureFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

func readConvergenceFixture(t *testing.T, dir string) reviewConvergence {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "review-context.json"))
	var context reviewConvergence
	if err != nil || json.Unmarshal(data, &context) != nil {
		t.Fatalf("context: %s %v", data, err)
	}
	return context
}

func TestReviewConvergenceUsesLastCompatibleCompletedRound(t *testing.T) {
	root := t.TempDir()
	current := ReviewManifest{Repo: "widget", PR: "7", Forge: "github", Slug: "example/widget", Public: true, Round: 5}
	prior := func(name string, round, lines int, public, completed bool, slug string) {
		t.Helper()
		dir := filepath.Join(root, name)
		convergenceFixtureFile(t, filepath.Join(dir, "diff.patch"), strings.Repeat("patch line\n", lines))
		m := current
		m.Round, m.Public, m.Slug = round, public, slug
		if err := writeReviewManifest(dir, m); err != nil {
			t.Fatal(err)
		}
		if completed {
			convergenceFixtureFile(t, filepath.Join(dir, "summary.md"), "Independent result\n")
			convergenceFixtureFile(t, filepath.Join(dir, "verdict"), "changes-requested\n")
		}
		convergenceFixtureFile(t, filepath.Join(dir, "findings", "correctness.json"), `{"concern":"correctness","status":"issues","findings":[{"severity":"major","title":"Ordering invariant"}]}`)
	}
	prior("r1", 1, 10, true, true, current.Slug)
	prior("r2", 2, 20, true, true, current.Slug)
	prior("unrun-r3", 3, 900, true, false, current.Slug)
	prior("summary-only-r3", 3, 900, true, false, current.Slug)
	convergenceFixtureFile(t, filepath.Join(root, "summary-only-r3", "summary.md"), "Unfinished review\n")
	prior("empty-summary-r3", 3, 900, true, false, current.Slug)
	convergenceFixtureFile(t, filepath.Join(root, "empty-summary-r3", "summary.md"), "")
	convergenceFixtureFile(t, filepath.Join(root, "empty-summary-r3", "verdict"), "pass\n")
	prior("unknown-verdict-r3", 3, 900, true, false, current.Slug)
	convergenceFixtureFile(t, filepath.Join(root, "unknown-verdict-r3", "summary.md"), "Unfinished review\n")
	convergenceFixtureFile(t, filepath.Join(root, "unknown-verdict-r3", "verdict"), "pending\n")
	prior("private-r4", 4, 900, false, true, current.Slug)
	prior("foreign-r4", 4, 900, true, true, "example/other")
	dir := filepath.Join(root, "current")
	convergenceFixtureFile(t, filepath.Join(dir, "diff.patch"), strings.Repeat("patch line\n", 31))
	if err := copyReviewPrior(dir, root, current); err != nil {
		t.Fatal(err)
	}
	context := readConvergenceFixture(t, dir)
	if context.PreviousRound != 2 || context.PreviousPatchLines == nil || *context.PreviousPatchLines != 20 ||
		context.PatchLines == nil || *context.PatchLines != 31 || context.GrowthPercent == nil || *context.GrowthPercent != 55 || !context.ScopeGrew {
		t.Fatalf("wrong preceding reviewed scope: %+v", context)
	}
	if !reflect.DeepEqual(context.PreviousMajorConcerns, []string{"correctness"}) {
		t.Fatalf("lost prior majors: %+v", context)
	}
	for _, name := range []string{"r4.md"} {
		if _, err := os.Stat(filepath.Join(dir, "prior", name)); !os.IsNotExist(err) {
			t.Fatalf("incompatible round copied: %s", name)
		}
	}
	if report := reviewConvergenceReport(context); !strings.Contains(report, "20 to 31") || !strings.Contains(report, "round 2") {
		t.Fatalf("scope warning missing: %s", report)
	}
}

func TestReviewConvergenceNoHistoryAndCompletedErrorRound(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "current")
	m := ReviewManifest{Repo: "widget", PR: "7", Forge: "github", Slug: "example/widget", Public: true, Round: 2}
	convergenceFixtureFile(t, filepath.Join(dir, "diff.patch"), "one\ntwo\n")
	if err := copyReviewPrior(dir, root, m); err != nil {
		t.Fatal(err)
	}
	context := readConvergenceFixture(t, dir)
	if context.PatchLines == nil || *context.PatchLines != 2 || context.PreviousRound != 0 || context.PreviousPatchLines != nil || context.GrowthPercent != nil || reviewConvergenceReport(context) != "" {
		t.Fatalf("first bundle invented a previous comparison: %+v", context)
	}
	old := filepath.Join(root, "old")
	convergenceFixtureFile(t, filepath.Join(old, "diff.patch"), "one\n")
	previous := m
	previous.Round = 1
	if err := writeReviewManifest(old, previous); err != nil {
		t.Fatal(err)
	}
	convergenceFixtureFile(t, filepath.Join(old, "summary.md"), "A concern failed; coverage is incomplete.\n")
	convergenceFixtureFile(t, filepath.Join(old, "verdict"), "error\n")
	if err := copyReviewPrior(dir, root, m); err != nil {
		t.Fatal(err)
	}
	context = readConvergenceFixture(t, dir)
	if context.PreviousRound != 1 || context.PreviousPatchLines == nil || *context.PreviousPatchLines != 1 || !context.ScopeGrew {
		t.Fatalf("completed error round was lost: %+v", context)
	}
}

func TestReviewConvergenceThresholdAndUnavailableHistory(t *testing.T) {
	for _, tc := range []struct {
		name          string
		before, after int
		missing       bool
		grew          bool
	}{
		{"exact threshold", 20, 30, false, false},
		{"over threshold", 20, 31, false, true},
		{"smaller", 20, 10, false, false},
		{"empty then changed", 0, 1, false, true},
		{"empty unchanged", 0, 0, false, false},
		{"missing older patch", 0, 100, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			old, current := filepath.Join(root, "old"), filepath.Join(root, "current")
			if err := os.MkdirAll(old, 0755); err != nil {
				t.Fatal(err)
			}
			if err := writeReviewManifest(old, ReviewManifest{Round: 1}); err != nil {
				t.Fatal(err)
			}
			if !tc.missing {
				convergenceFixtureFile(t, filepath.Join(old, "diff.patch"), strings.Repeat("x\n", tc.before))
			}
			convergenceFixtureFile(t, filepath.Join(current, "diff.patch"), strings.Repeat("x\n", tc.after))
			if err := prepareReviewConvergence(current, ReviewManifest{Round: 2}, old); err != nil {
				t.Fatal(err)
			}
			got := readConvergenceFixture(t, current)
			if got.ScopeGrew != tc.grew {
				t.Fatalf("growth: %+v", got)
			}
			if tc.missing && (got.PreviousPatchLines != nil || got.GrowthPercent != nil) {
				t.Fatalf("missing history became a comparison: %+v", got)
			}
		})
	}
	legacy := t.TempDir()
	context, err := refreshReviewConvergence(legacy, ReviewManifest{Round: 4})
	if err != nil || context.PatchLines != nil || context.PreviousPatchLines != nil || reviewConvergenceReport(context) != "" {
		t.Fatalf("legacy bundle invented history: %+v %v", context, err)
	}
}

func TestReviewConvergenceDesignCheckpointFollowsLeadDispositions(t *testing.T) {
	dir := t.TempDir()
	context := reviewConvergence{Round: 3, PreviousRound: 2, GrowthThreshold: 50, PreviousMajorConcerns: []string{"correctness"}}
	if err := writeReviewConvergence(dir, context); err != nil {
		t.Fatal(err)
	}
	write := func(prior string) {
		convergenceFixtureFile(t, filepath.Join(dir, "findings", "correctness.json"), `{"concern":"correctness","status":"issues","findings":[{"severity":"major","title":"Ordering invariant","prior":"`+prior+`"}]}`)
		convergenceFixtureFile(t, filepath.Join(dir, "findings", "tests.json"), `{"concern":"tests","status":"issues","findings":[{"severity":"major","title":"New unrelated gap"}]}`)
	}
	write("still-open")
	early, err := refreshReviewConvergence(dir, ReviewManifest{Round: 2})
	if err != nil || len(early.DesignCheckpoints) != 0 {
		t.Fatalf("early design checkpoint: %+v %v", early, err)
	}
	got, err := refreshReviewConvergence(dir, ReviewManifest{Round: 3})
	if err != nil || !reflect.DeepEqual(got.DesignCheckpoints, []string{"correctness"}) || !strings.Contains(reviewConvergenceReport(got), "Does the design converge") {
		t.Fatalf("missing design checkpoint: %+v %v", got, err)
	}
	write("addressed")
	got, err = refreshReviewConvergence(dir, ReviewManifest{Round: 3})
	if err != nil || len(got.DesignCheckpoints) != 0 || reviewConvergenceReport(got) != "" {
		t.Fatalf("refuted major left a checkpoint: %+v %v", got, err)
	}
}
