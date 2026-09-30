package main

import (
	"testing"

	"github.com/lcorneliussen/wtc-cli/internal/wtc"
)

func TestReviewReadyGateRejectsBlockersEvenWithPassingVerdict(t *testing.T) {
	zero, one := 0, 1
	for _, tc := range []struct {
		name  string
		state wtc.ReviewStatus
		allow bool
	}{
		{"current pass", wtc.ReviewStatus{State: "current", Verdict: "pass", Blockers: &zero}, true},
		{"current pass with notes", wtc.ReviewStatus{State: "current", Verdict: "pass-with-notes", Blockers: &zero}, true},
		{"pass with blocker", wtc.ReviewStatus{State: "current", Verdict: "pass", Blockers: &one}, false},
		{"notes with blocker", wtc.ReviewStatus{State: "current", Verdict: "pass-with-notes", Blockers: &one}, false},
		{"missing count", wtc.ReviewStatus{State: "current", Verdict: "pass"}, false},
		{"stale", wtc.ReviewStatus{State: "stale", Verdict: "pass", Blockers: &zero}, false},
		{"pending", wtc.ReviewStatus{State: "current", Verdict: "pending", Blockers: &zero}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := reviewReadyAllowed(tc.state); got != tc.allow {
				t.Fatalf("allowed=%t, want %t", got, tc.allow)
			}
		})
	}
}
