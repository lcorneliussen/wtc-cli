package wtc

import (
	"testing"

	"github.com/lcorneliussen/wtc-cli/internal/next"
)

// A local row's facts become its next step: pushing is the agent's here,
// and the pull request's facts read as the forge reports them.
func TestStatusNextFromALocalRow(t *testing.T) {
	// No git in the worktree: the mapping is what is tested here, and git
	// must not read the checkout the test runs in. Predicting the merge is
	// tested in next and on the gateway.
	none := t.TempDir()
	branch := func(fields StatusRepo) StatusRepo {
		fields.Repo, fields.BranchKind, fields.Branch, fields.baseRef, fields.Worktree = "api", "branch", "12-paging", "origin/main", none
		return fields
	}
	for _, tc := range []struct {
		name string
		row  StatusRepo
		want string
		by   string
	}{
		{"commits to push: the agent's draft PR", branch(StatusRepo{Ahead: 2}), "push", next.ByAgent},
		{"failed checks", branch(StatusRepo{PR: &StatusPRFacts{Number: "7", Checks: "FAILURE", Merge: "CLEAN", Review: "waiting"}}), "fix-checks", next.ByAgent},
		{"conflicting with its base", branch(StatusRepo{PR: &StatusPRFacts{Number: "7", Checks: "SUCCESS", Merge: "CONFLICTING"}}), "merge", next.ByAgent},
		{"changes requested", branch(StatusRepo{PR: &StatusPRFacts{Number: "7", Checks: "SUCCESS", Merge: "CLEAN", Review: "changes"}}), "address-review", next.ByAgent},
		{"approved and green", branch(StatusRepo{PR: &StatusPRFacts{Number: "7", Checks: "SUCCESS", Merge: "CLEAN", Review: "approved"}}), "merge-pr", next.ByPerson},
		{"a merge in progress", branch(StatusRepo{Operation: "merge", Conflict: true}), "resolve", next.ByAgent},
		{"a pushed branch behind its base: the agent decides", branch(StatusRepo{Behind: 2}), "catch-up", next.ByWTC},
		{"uncommitted files: wait", branch(StatusRepo{Ahead: 1, Behind: 2, Changed: 3}), "", ""},
		{"detached and current: nothing", StatusRepo{Repo: "api", BranchKind: "detached", Branch: "main", Worktree: none}, "", ""},
	} {
		got := next.For(statusFacts(tc.row))
		switch {
		case tc.want == "" && got != nil:
			t.Errorf("%s: %+v, want none", tc.name, got)
		case tc.want != "" && (got == nil || got.Step != tc.want || got.By != tc.by):
			t.Errorf("%s: %+v, want %s by %s", tc.name, got, tc.want, tc.by)
		case tc.want == "catch-up" && got.Approve != next.ApproveAgent:
			t.Errorf("%s: a catch-up onto a branch needs the agent's go: %+v", tc.name, got)
		}
	}
}
