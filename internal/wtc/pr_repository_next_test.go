package wtc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnlistedRepositoryRetainedMergedAliasKeepsNext(t *testing.T) {
	c, path := enlistedIdentityFixture(t, false)
	merged := fixtureGit(t, "-C", path, "rev-parse", "HEAD")
	fixtureFile(t, filepath.Join(path, "followup"), "Synthetic follow-up\n", 0644)
	fixtureGit(t, "-C", path, "add", "followup")
	fixtureGit(t, "-C", path, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "Synthetic follow-up")
	fixtureFile(t, c.PRFile(), "widget 7 topic\n", 0644)
	bin := t.TempDir()
	fixtureFile(t, filepath.Join(bin, "gh"), "#!/bin/sh\nif [ \"$2\" = view ]; then printf '%s\\n' '{\"number\":7,\"state\":\"MERGED\",\"mergeCommit\":{\"oid\":\""+merged+"\"}}'; else printf '[]\\n'; fi\n", 0755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	snapshot, err := c.statusForgePreview(false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.PRs) != 1 || !snapshot.PRs[0].OnBranch {
		t.Fatalf("not an associated merged PR: %+v", snapshot.PRs)
	}
	for _, row := range snapshot.Repos {
		if row.Dir == "ext.widget" && (row.Next == nil || row.Next.Step != "new-branch") {
			t.Fatalf("alias lost retained-merge next action: %+v", row.Next)
		}
	}
	if err := c.WriteStatusSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	cached, err := c.ReadStatusSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range cached.Repos {
		if row.Dir == "ext.widget" {
			found = true
			if row.Next == nil || row.Next.Step != "new-branch" {
				t.Fatalf("cached snapshot lost verified next action: %+v", row.Next)
			}
		}
	}
	if !found {
		t.Fatal("cached snapshot lost the associated repository")
	}
	raw, err := json.Marshal(snapshot)
	if err != nil || strings.Contains(string(raw), "worktreeDir") || strings.Contains(string(raw), "worktree_dir") {
		t.Fatalf("internal association changed public JSON: %s %v", raw, err)
	}
}

func TestEnlistedRepositoryGitHubIdentityIgnoresOwnerAndRepoCase(t *testing.T) {
	c, _ := enlistedIdentityFixture(t, true)
	c.Registry.Repos[1].Remote = "https://github.com/Example/Widget.git"
	r, err := c.EnlistPR(PRRecord{Repo: "widget", Number: "7"})
	if err != nil || r.Repo != "widget" || r.Branch != "topic" {
		t.Fatalf("case-only identity mismatch: %+v %v", r, err)
	}
	if match, err := c.prRepositories().matchesDir("widget", "ext.widget"); err != nil || !match {
		t.Fatalf("case-only branch association: %v %v", match, err)
	}
	t.Run("unmanaged-directory-case", func(t *testing.T) {
		c, path := enlistedIdentityFixture(t, false)
		if err := os.Rename(path, filepath.Join(c.Collection, "ext.Widget")); err != nil {
			t.Fatal(err)
		}
		r, err := c.EnlistPR(PRRecord{Repo: "Widget", Number: "7"})
		if err != nil || r.Repo != "ext.Widget" || r.Branch != "topic" {
			t.Fatalf("case-only basename mismatch: %+v %v", r, err)
		}
	})
}

func TestEnlistedRepositoryRemoteLessRegistrationNeedsAliasEvidence(t *testing.T) {
	c, _ := enlistedIdentityFixture(t, true)
	c.Registry.Repos[1].Remote = ""
	if _, err := c.EnlistPR(PRRecord{Repo: "widget", Number: "7"}); err == nil || !strings.Contains(err.Error(), "verify registry remotes") {
		t.Fatalf("unconfigured alias needs a registry diagnostic: %v", err)
	}
	if _, err := os.Stat(c.PRFile()); !os.IsNotExist(err) {
		t.Fatal("ambiguous enlistment wrote a row")
	}
	r, err := c.EnlistPR(PRRecord{Repo: "ext.widget", Number: "7"})
	if err != nil || r.Repo != "ext.widget" || r.Branch != "topic" {
		t.Fatalf("explicit checkout identity was lost: %+v %v", r, err)
	}
}

func TestEnlistedRepositoryVerifiedNextAssociationAndLegacyFallback(t *testing.T) {
	for _, tc := range []struct {
		name, resolved, enlisted, repo, dir string
		want                                bool
	}{
		{"verified-alias", "ext.widget", "widget", "ext.widget", "ext.widget", true},
		{"verified-other-directory", "ext.widget", "widget", "widget", "widget", false},
		{"legacy-exact", "", "widget", "widget", "widget", true},
		{"legacy-no-prefix-guess", "", "widget", "ext.widget", "ext.widget", false},
		{"independent-forge-same-slug", "ext.widget", "widget", "other", "other", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pr := StatusPRRow{Repo: tc.enlisted, Slug: "example/widget", worktreeDir: tc.resolved}
			row := StatusRepo{Repo: tc.repo, Dir: tc.dir, Slug: "example/widget", Forge: "bitbucket.org"}
			if got := statusPRMatchesRepo(pr, row); got != tc.want {
				t.Fatalf("branch association: got %v want %v", got, tc.want)
			}
		})
	}
}

func TestEnlistedRepositoryCaseFoldingDoesNotCrossForges(t *testing.T) {
	c, path := enlistedIdentityFixture(t, true)
	c.Registry.Repos[1].Remote = "https://bitbucket.org/Example/widget.git"
	fixtureGit(t, "-C", path, "remote", "set-url", "origin", "https://bitbucket.org/example/widget.git")
	if _, err := c.prRepositories().resolve("widget"); err == nil {
		t.Fatal("extended GitHub case rules to another forge without evidence")
	}
}
