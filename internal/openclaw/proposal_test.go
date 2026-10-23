package openclaw

import (
	"encoding/json"
	"os"
	"path/filepath"
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

// A merged pull request says whether the checkout has its merge commit yet,
// so that the card can offer the catch-up that brings it in.
func TestStatusOfAMergedPullRequest(t *testing.T) {
	c, git := managedWorktree(t, "https://github.com/example-org/docs.git")
	write(t, c, "notes.md", "one\n")
	git("add", ".")
	git("commit", "-qm", "Add notes")
	merge := git("commit-tree", "-p", "origin/main", "-p", "HEAD", "-m", "Merge pull request #1", "HEAD^{tree}")
	bin := t.TempDir()
	script := "#!/bin/sh\nprintf '%s' '[{\"number\":1,\"url\":\"u\",\"state\":\"MERGED\",\"isDraft\":false,\"title\":\"t\",\"statusCheckRollup\":[],\"mergeable\":\"UNKNOWN\",\"reviewDecision\":\"\",\"mergeCommit\":{\"oid\":\"" + merge + "\"}}]'\n"
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	// The card reads the JSON: false has to be in it.
	encoded := func() string {
		out, err := json.Marshal(CheckoutsStatus([]string{c.Dir}, true).Repos[0].PullRequest)
		if err != nil {
			t.Fatal(err)
		}
		return string(out)
	}
	if got := encoded(); !strings.Contains(got, `"state":"merged"`) || !strings.Contains(got, `"mergedIn":false`) {
		t.Fatalf("before the catch-up: %s", got)
	}
	git("merge", "-q", "--ff-only", merge)
	if got := encoded(); !strings.Contains(got, `"mergedIn":true`) {
		t.Fatalf("after it: %s", got)
	}
	// Without a merge commit to go by, it says nothing.
	os.WriteFile(filepath.Join(bin, "gh"), []byte(strings.Replace(script, `,"mergeCommit":{"oid":"`+merge+`"}`, "", 1)), 0o755)
	if got := encoded(); strings.Contains(got, "mergedIn") {
		t.Fatalf("no merge commit: %s", got)
	}
}
