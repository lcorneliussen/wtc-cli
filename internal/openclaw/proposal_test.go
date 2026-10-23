package openclaw

import (
	"strings"
	"testing"
)

// What the agent proposes for the pull request stays out of the commit, is
// reported with the checkout, and belongs to its branch.
func TestProposal(t *testing.T) {
	c, git := managedWorktree(t, "https://github.com/example-org/docs.git")
	if p, err := c.Propose("", ""); p != nil || err != nil {
		t.Fatalf("nothing proposed: %+v %v", p, err)
	}
	if _, err := c.Propose("Add notes", "  What and why.\n"); err != nil {
		t.Fatal(err)
	}
	// One left out keeps the earlier one.
	p, err := c.Propose("", "What, why, and how it was checked.")
	if err != nil || p.Title != "Add notes" || p.Body != "What, why, and how it was checked." || p.Branch != "openclaw/72-add-thing" {
		t.Fatalf("%+v %v", p, err)
	}
	if got := git("status", "--porcelain"); got != "" {
		t.Fatalf("the proposal is in the worktree:\n%s", got)
	}
	status := CheckoutsStatus([]string{c.Dir}, false).Repos[0]
	if status.Proposal == nil || *status.Proposal != *p {
		t.Fatalf("status %+v", status.Proposal)
	}
	// Another branch in the same worktree has proposed nothing.
	git("switch", "-qc", "openclaw/73-other")
	if got := ReadProposal(c.Dir, "openclaw/73-other"); got != nil {
		t.Fatalf("another branch: %+v", got)
	}
}

func TestCheckProposal(t *testing.T) {
	for _, tc := range []struct{ title, body, want string }{
		{"Add notes", "", ""},
		{"two\nlines", "", "one line"},
		{strings.Repeat("a", MaxProposalTitle+1), "", "at most 256"},
		// JavaScript counts an emoji as two.
		{strings.Repeat("🦞", MaxProposalTitle/2+1), "", "at most 256"},
		{"", strings.Repeat("b", MaxProposalBody+1), "at most 8192"},
	} {
		err := CheckProposal(tc.title, tc.body)
		if tc.want == "" && err != nil || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
			t.Errorf("%.20q: %v, want %q", tc.title, err, tc.want)
		}
	}
}
