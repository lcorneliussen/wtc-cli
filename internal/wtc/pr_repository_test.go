package wtc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func enlistedIdentityFixture(t *testing.T, registered bool) (*Context, string) {
	t.Helper()
	t.Setenv("WTC_HARNESS_REPO", "harness-example")
	t.Setenv("TMPDIR", t.TempDir())
	t.Setenv("WTC_STATUS_RETURN_MERGED", "off")
	c := fixture(t)
	registry := "repos:\n  - name: harness-example\n    remote: https://example.invalid/harness-example.git\n    default_ref: origin/main\n"
	if registered {
		registry += "  - name: widget\n    remote: https://github.com/example/widget.git\n    default_ref: origin/main\n"
	}
	fixtureFile(t, c.registryPath(), registry, 0644)
	if err := c.load(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(c.Collection, "ext.widget")
	enlistedIdentityGit(t, path, "git@github.com:example/widget.git")
	return c, path
}

func enlistedIdentityGit(t *testing.T, path, remote string) {
	t.Helper()
	fixtureGit(t, "init", "-q", "-b", "topic", path)
	fixtureFile(t, filepath.Join(path, "README.md"), "Synthetic repository.\n", 0644)
	fixtureGit(t, "-C", path, "add", ".")
	fixtureGit(t, "-C", path, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "Synthetic base")
	fixtureGit(t, "-C", path, "remote", "add", "origin", remote)
	fixtureGit(t, "-C", path, "update-ref", "refs/remotes/origin/main", "HEAD")
}

func TestEnlistedRepositoryAliasesResolveForgeAndEnlist(t *testing.T) {
	for _, registered := range []bool{true, false} {
		name := "unmanaged"
		if registered {
			name = "registered"
		}
		t.Run(name, func(t *testing.T) {
			c, path := enlistedIdentityFixture(t, registered)
			rows := []StatusRepo{{Repo: "ext.widget", Dir: "ext.widget", Worktree: path, Slug: "example/widget"}}
			for _, spelling := range []string{"widget", "ext.widget"} {
				slug, forge := c.statusRecordForge(PRRecord{Repo: spelling}, rows)
				if slug != "example/widget" || forge != "github.com" {
					t.Errorf("%s resolved to %q %q", spelling, slug, forge)
				}
				r, err := c.EnlistPR(PRRecord{Repo: spelling, Number: "7"})
				want := "ext.widget"
				if registered {
					want = "widget"
				}
				if err != nil || r.Repo != want || r.Branch != "topic" || r.URL != "https://github.com/example/widget/pull/7" {
					t.Errorf("enlist %s: %+v, %v", spelling, r, err)
				}
			}
			records, err := c.ListPRs()
			if err != nil || len(records) != 1 {
				t.Fatalf("aliases created duplicate rows: %+v, %v", records, err)
			}
		})
	}
}

func TestEnlistedRepositoryRegisteredAliasUsesFullRemoteNotBasename(t *testing.T) {
	c, path := enlistedIdentityFixture(t, true)
	c.Registry.Repos[1].Remote = "ssh://git@bitbucket.org/example/component.git"
	fixtureGit(t, "-C", path, "remote", "set-url", "origin", "https://bitbucket.org/example/component.git")
	for _, name := range []string{"widget", "ext.widget"} {
		repo, err := c.prRepositories(name).resolve(name)
		if err != nil || repo == nil || repo.name != "widget" || repo.dir() != "ext.widget" {
			t.Fatalf("registered name lost full remote identity: %+v %v", repo, err)
		}
		if slug, forge := c.statusRecordForge(PRRecord{Repo: name}, nil); slug != "example/component" || forge != "bitbucket.org" {
			t.Fatalf("Bitbucket identity: %s %s", slug, forge)
		}
	}
}

func TestEnlistedRepositoryAliasAssociatesBranch(t *testing.T) {
	c, path := enlistedIdentityFixture(t, false)
	fixtureFile(t, c.PRFile(), "widget 7 topic\n", 0644) // Legacy spelling stays readable.
	bin := t.TempDir()
	calls := filepath.Join(t.TempDir(), "calls")
	t.Setenv("IDENTITY_CALLS", calls)
	fixtureFile(t, filepath.Join(bin, "gh"), `#!/bin/sh
printf '%s\n' "$*" >> "$IDENTITY_CALLS"
if [ "$2" = "view" ]; then
  printf '%s\n' '{"number":7,"state":"OPEN","title":"Synthetic change","baseRefName":"main"}'
else
  printf '[]\n'
fi
`, 0755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	snapshot, err := c.statusForgePreview(false, false)
	if err != nil || len(snapshot.PRs) != 1 || snapshot.PRs[0].State != "OPEN" {
		t.Fatalf("alias PR did not enrich: %+v, %v", snapshot.PRs, err)
	}
	found := false
	for _, row := range snapshot.Repos {
		if row.Worktree == path {
			found = row.PR != nil && row.PR.Number == "7"
		}
	}
	if !found {
		t.Fatal("alias did not attach enlisted PR to its branch")
	}
	data, _ := os.ReadFile(calls)
	if strings.Contains(string(data), "--head") {
		t.Fatalf("enlisted branch was rediscovered: %s", data)
	}
	_, targets, err := c.CatchUpInventory(CatchUpOptions{Repos: []string{"ext.widget"}, DryRun: true})
	if err != nil || len(targets) != 1 {
		t.Fatalf("inventory: %+v %v", targets, err)
	}
	if pr := c.catchUpPRState(targets[0], "topic"); pr.state != "OPEN" || pr.base != "main" {
		t.Fatalf("catch-up missed enlisted alias: %+v", pr)
	}
}

func TestEnlistedRepositoryAliasesRequireIdentityEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, remote string
	}{
		{"different-owner", "https://github.com/another/widget.git"},
		{"different-forge", "https://bitbucket.org/example/widget.git"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, path := enlistedIdentityFixture(t, true)
			fixtureGit(t, "-C", path, "remote", "set-url", "origin", tc.remote)
			repos := c.prRepositories()
			if _, err := repos.resolve("widget"); err == nil {
				t.Fatal("selected one of two repository identities sharing a basename")
			}
			if slug, forge := repos.forge("widget"); slug != "" || forge != "" {
				t.Fatalf("ambiguous alias selected forge: %s %s", slug, forge)
			}
			fixtureFile(t, c.PRFile(), "widget 7 topic\n", 0644)
			before, _ := os.ReadFile(c.PRFile())
			if _, err := c.EnlistPR(PRRecord{Repo: "widget", Number: "7"}); err == nil {
				t.Fatal("enlisted an ambiguous repository")
			}
			after, _ := os.ReadFile(c.PRFile())
			if string(after) != string(before) {
				t.Fatal("ambiguous enlist changed existing rows")
			}
			_, targets, err := c.CatchUpInventory(CatchUpOptions{Repos: []string{"ext.widget"}, DryRun: true})
			if err != nil || len(targets) != 1 {
				t.Fatalf("inventory: %+v %v", targets, err)
			}
			bin := t.TempDir()
			calls := filepath.Join(t.TempDir(), "calls")
			t.Setenv("IDENTITY_CALLS", calls)
			for _, cli := range []string{"gh", "bb"} {
				fixtureFile(t, filepath.Join(bin, cli), "#!/bin/sh\nprintf 'called\\n' >> \"$IDENTITY_CALLS\"\nexit 1\n", 0755)
			}
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			if pr := c.catchUpPRState(targets[0], "topic"); pr.state != "UNKNOWN" {
				t.Fatalf("catch-up guessed through ambiguous identity: %+v", pr)
			}
			if _, err := os.Stat(calls); !os.IsNotExist(err) {
				t.Fatal("ambiguous branch made a forge request")
			}
			// The explicit ext. directory still identifies its own repository.
			wantSlug, wantForge := catchUpForge(tc.remote)
			if slug, forge := repos.forge("ext.widget"); slug != wantSlug || forge != wantForge {
				t.Fatalf("distinct explicit directory lost identity: %s %s", slug, forge)
			}
		})
	}
	for _, tc := range []struct{ dir, remote, alias string }{
		{"ext.notes", "https://github.com/example/widget.git", "notes"},
		{"ext.ext.widget", "https://github.com/example/widget.git", "ext.widget"},
		{"ext.widget", "https://example.invalid/example/widget.git", "widget"},
	} {
		t.Run(tc.dir+tc.remote, func(t *testing.T) {
			c := fixture(t)
			c.Registry.Repos = nil
			enlistedIdentityGit(t, filepath.Join(c.Collection, tc.dir), tc.remote)
			if repo, err := c.prRepositories().resolve(tc.alias); err != nil || repo != nil {
				t.Fatalf("inferred alias without origin/name evidence: %+v %v", repo, err)
			}
		})
	}
}

func TestEnlistedRepositoryAliasesRejectMultipleWorktreesAndOutsideRoots(t *testing.T) {
	c, _ := enlistedIdentityFixture(t, true)
	enlistedIdentityGit(t, filepath.Join(c.Collection, "widget"), "https://github.com/example/widget.git")
	for _, name := range []string{"widget", "ext.widget"} {
		if _, err := c.prRepositories().resolve(name); err == nil {
			t.Fatalf("%s selected one of two possible worktrees", name)
		}
	}
	t.Run("outside-symlink", func(t *testing.T) {
		c := fixture(t)
		c.Registry.Repos = nil
		outside := filepath.Join(t.TempDir(), "widget")
		enlistedIdentityGit(t, outside, "https://github.com/example/widget.git")
		if err := os.Symlink(outside, filepath.Join(c.Collection, "ext.widget")); err != nil {
			t.Fatal(err)
		}
		if repo, err := c.prRepositories().resolve("widget"); err != nil || repo != nil {
			t.Fatalf("outside symlink supplied identity: %+v %v", repo, err)
		}
	})
	t.Run("containing-repository", func(t *testing.T) {
		c := fixture(t)
		c.Registry.Repos = nil
		fixtureGit(t, "init", "-q", c.Collection)
		fixtureGit(t, "-C", c.Collection, "remote", "add", "origin", "https://github.com/example/widget.git")
		fixtureFile(t, filepath.Join(c.Collection, "ext.widget", "README.md"), "Not a Git checkout.\n", 0644)
		if repo, err := c.prRepositories().resolve("widget"); err != nil || repo != nil {
			t.Fatalf("containing repository supplied identity: %+v %v", repo, err)
		}
	})
}

func TestEnlistedRepositoryAliasesPreserveLegacyMarkersAndRejectDuplicateRows(t *testing.T) {
	c, _ := enlistedIdentityFixture(t, true)
	sha := strings.Repeat("a", 40)
	markers := "# merged-pr ext.widget 7 2026-09-30T12:00:00Z SUCCESS\n# merged-commit ext.widget 7 " + sha + " main\n"
	fixtureFile(t, c.PRFile(), "# Keep this comment.\next.widget 7 topic\nother 8 other\n"+markers, 0644)
	r, err := c.EnlistPR(PRRecord{Repo: "widget", Number: "7", Branch: "topic", Title: "Synthetic change"})
	if err != nil || r.Repo != "ext.widget" {
		t.Fatalf("legacy row key was replaced: %+v %v", r, err)
	}
	records, err := c.ListPRs()
	if err != nil || len(records) != 2 || records[1].MergeCommit != sha || records[1].FinalChecks != "SUCCESS" {
		t.Fatalf("legacy merge facts lost: %+v %v", records, err)
	}
	data, _ := os.ReadFile(c.PRFile())
	if !strings.Contains(string(data), markers) || !strings.Contains(string(data), "# Keep this comment.\nother 8 other\n") {
		t.Fatalf("unrelated rows or markers changed: %s", data)
	}
	if err := c.UnlistPR("widget", "7"); err != nil {
		t.Fatal(err)
	}
	records, _ = c.ListPRs()
	if len(records) != 1 || records[0].Repo != "other" {
		t.Fatalf("alias unlist left row: %+v", records)
	}
	fixtureFile(t, c.PRFile(), "widget 7 topic\next.widget 7 topic\n"+markers, 0644)
	before, _ := os.ReadFile(c.PRFile())
	if _, err := c.EnlistPR(PRRecord{Repo: "widget", Number: "7"}); err == nil {
		t.Fatal("silently collapsed conflicting legacy rows")
	}
	after, _ := os.ReadFile(c.PRFile())
	if string(after) != string(before) {
		t.Fatal("duplicate detection changed legacy rows or markers")
	}
}

func TestEnlistedRepositoryAliasMissingBranchUsesExistingMergeRecording(t *testing.T) {
	for _, state := range []string{"settled", "pending", "error"} {
		t.Run(state, func(t *testing.T) {
			c, path := enlistedIdentityFixture(t, false)
			fixtureGit(t, "-C", path, "checkout", "--detach", "HEAD")
			fixtureFile(t, c.PRFile(), "widget 7 gone\n", 0644)
			other := filepath.Join(c.Workspace, "other", ".wtc-prs")
			fixtureFile(t, other, "widget 8 gone\n", 0644)
			bin := t.TempDir()
			calls := filepath.Join(t.TempDir(), "calls")
			t.Setenv("IDENTITY_CALLS", calls)
			t.Setenv("IDENTITY_STATE", state)
			fixtureFile(t, filepath.Join(bin, "gh"), `#!/bin/sh
printf '%s\n' "$*" >> "$IDENTITY_CALLS"
if [ "$2" = list ]; then printf '[]\n'; exit 0; fi
[ "$1 $2 $3 $4 $5" = 'pr view 7 --repo example/widget' ] || exit 2
[ "$IDENTITY_STATE" != error ] || exit 1
if [ "$IDENTITY_STATE" = pending ]; then check='{"status":"IN_PROGRESS"}'; else check='{"status":"COMPLETED","conclusion":"SUCCESS"}'; fi
printf '{"number":7,"state":"MERGED","title":"Synthetic change","mergedAt":"2026-09-30T12:00:00Z","statusCheckRollup":[%s]}\n' "$check"
`, 0755)
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			snapshot, err := c.statusForgePreview(false, true)
			if err != nil || len(snapshot.PRs) != 1 {
				t.Fatalf("snapshot: %+v %v", snapshot.PRs, err)
			}
			want := "MERGED"
			if state == "error" {
				want = "UNKNOWN"
			}
			if snapshot.PRs[0].State != want || snapshot.PRs[0].OnBranch {
				t.Fatalf("missing branch changed forge state: %+v", snapshot.PRs[0])
			}
			records, _ := c.ListPRs()
			if (records[0].MergedOn != "") != (state == "settled") {
				t.Fatalf("incorrect settled-merge recording: %+v", records)
			}
			data, _ := os.ReadFile(calls)
			if !strings.Contains(string(data), "pr view 7 --repo example/widget") {
				t.Fatalf("no by-number fallback: %s", data)
			}
			data, _ = os.ReadFile(other)
			if string(data) != "widget 8 gone\n" {
				t.Fatal("scoped merge recording changed another collection")
			}
			if _, err := os.Stat(other + ".lock"); !os.IsNotExist(err) {
				t.Fatal("scoped merge recording locked another collection")
			}
		})
	}
}

func TestEnlistedRepositoryAliasLiveRecordBlocksMergedReturn(t *testing.T) {
	c, path := enlistedIdentityFixture(t, false)
	t.Setenv("WTC_STATUS_RETURN_MERGED", "on")
	row := StatusRepo{Repo: "ext.widget", Dir: "ext.widget", Worktree: path, Branch: "topic", BranchKind: "branch", Tree: "clean"}
	snapshot := StatusSnapshot{Repos: []StatusRepo{row}}
	records := []PRRecord{{Repo: "widget", Number: "7", Branch: "topic"}, {Repo: "ext.widget", Number: "8", Branch: "topic"}}
	details := []statusPRDetail{{State: "MERGED", MergeCommit: fixtureGit(t, "-C", path, "rev-parse", "HEAD")}, {State: "UNKNOWN"}}
	c.statusReturnMerged(&snapshot, records, details)
	if branch := fixtureGit(t, "-C", path, "symbolic-ref", "--short", "HEAD"); branch != "topic" {
		t.Fatalf("live alias failed to preserve branch: %s", branch)
	}
}

func TestEnlistedRepositoryExactLinkedCheckoutRetainsAssociation(t *testing.T) {
	c, path := enlistedIdentityFixture(t, true)
	outside := filepath.Join(t.TempDir(), "widget")
	if err := os.Rename(path, outside); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(c.Collection, "widget")
	if err := os.Symlink(outside, linked); err != nil {
		t.Fatal(err)
	}
	r, err := c.EnlistPR(PRRecord{Repo: "widget", Number: "7"})
	if err != nil || r.Branch != "topic" {
		t.Errorf("linked checkout lost branch inference: %+v %v", r, err)
	}
	fixtureFile(t, c.PRFile(), "widget 7 topic\n", 0644)
	bin := t.TempDir()
	fixtureFile(t, filepath.Join(bin, "gh"), `#!/bin/sh
if [ "$2" = view ]; then
  printf '%s\n' '{"number":7,"state":"OPEN","baseRefName":"main"}'
else
  printf '[]\n'
fi
`, 0755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	snapshot, err := c.statusForgePreview(false, false)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range snapshot.Repos {
		if row.Dir == "widget" {
			found = row.PR != nil && row.PR.Number == "7"
		}
	}
	if !found {
		t.Error("linked checkout lost enlisted branch facts")
	}
	_, targets, err := c.CatchUpInventory(CatchUpOptions{Repos: []string{"widget"}, DryRun: true})
	if err != nil || len(targets) != 1 {
		t.Fatalf("inventory: %+v %v", targets, err)
	}
	if pr := c.catchUpPRState(targets[0], "topic"); pr.state != "OPEN" || pr.base != "main" {
		t.Errorf("linked checkout lost enlisted PR: %+v", pr)
	}
}

func TestEnlistedRepositoryExactCheckoutKeepsEmptyRemoteFallback(t *testing.T) {
	c := fixture(t)
	c.Registry.Repos[1].Remote = ""
	enlistedIdentityGit(t, filepath.Join(c.Collection, "widget"), "https://github.com/example/widget.git")
	r, err := c.EnlistPR(PRRecord{Repo: "widget", Number: "7"})
	if err != nil || r.URL != "https://github.com/example/widget/pull/7" || r.Branch != "topic" {
		t.Errorf("empty registry remote lost origin fallback: %+v %v", r, err)
	}
	if slug, forge := c.statusRecordForge(PRRecord{Repo: "widget"}, nil); slug != "example/widget" || forge != "github.com" {
		t.Errorf("empty registry remote lost status origin: %s %s", slug, forge)
	}
}

func TestEnlistedRepositoryUnlistCanReconcileDuplicatesAndAmbiguity(t *testing.T) {
	for _, ambiguous := range []bool{false, true} {
		name := "verified-duplicates"
		if ambiguous {
			name = "ambiguous-exact-key"
		}
		t.Run(name, func(t *testing.T) {
			c, path := enlistedIdentityFixture(t, true)
			if ambiguous {
				fixtureGit(t, "-C", path, "remote", "set-url", "origin", "https://github.com/another/widget.git")
			}
			fixtureFile(t, c.PRFile(), "widget 7 topic\next.widget 7 other\n# merged-pr widget 7 2026-09-30T12:00:00Z SUCCESS\n", 0644)
			if err := c.UnlistPR("widget", "7"); err != nil {
				t.Fatal(err)
			}
			records, _ := c.ListPRs()
			if ambiguous {
				if len(records) != 1 || records[0].Repo != "ext.widget" {
					t.Fatalf("ambiguous unlist changed another identity: %+v", records)
				}
			} else if len(records) != 0 {
				t.Fatalf("verified alias duplicates remain: %+v", records)
			}
			data, _ := os.ReadFile(c.PRFile())
			if strings.Contains(string(data), "# merged-pr widget 7") {
				t.Fatal("unlisted exact key kept merged marker")
			}
		})
	}
}

func TestEnlistedRepositoryHarnessAssociationUsesTargetIdentity(t *testing.T) {
	c := newWorkspaceFixture(t)
	t.Setenv("WTC_HARNESS_REPO", "widget")
	repos := c.prRepositories()
	if match, err := repos.matchesDir("agent-harness", "harness"); err != nil || !match {
		t.Errorf("target harness lost directory association: %v %v", match, err)
	}
	if match, err := repos.matchesDir("widget", "harness"); err != nil || match {
		t.Errorf("inherited harness redirected another repository: %v %v", match, err)
	}
}
