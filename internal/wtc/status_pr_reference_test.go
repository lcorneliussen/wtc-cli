package wtc

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStatusURLOnlyReferencesUseForgeWithoutCheckoutAuthority(t *testing.T) {
	for _, forge := range []string{"github.com", "bitbucket.org"} {
		for _, state := range []string{"OPEN", "CLOSED", "MERGED", "error", "pending"} {
			if forge == "bitbucket.org" && state == "pending" {
				continue
			}
			t.Run(forge+"/"+state, func(t *testing.T) {
				t.Setenv("TMPDIR", t.TempDir())
				c := newWorkspaceFixture(t)
				fixtureGit(t, "-C", c.Harness, "switch", "-qc", "topic")
				head := fixtureGit(t, "-C", c.Harness, "rev-parse", "HEAD")
				before := fixtureGit(t, "-C", c.Harness, "status", "--porcelain")
				t.Setenv("REFERENCE_STATE", state)
				t.Setenv("REFERENCE_HEAD", head)
				calls := filepath.Join(t.TempDir(), "calls")
				t.Setenv("REFERENCE_CALLS", calls)
				bin := t.TempDir()
				script := `#!/bin/sh
printf '%s\n' "$*" >> "$REFERENCE_CALLS"
[ "$2" != list ] || { printf '[]\n'; exit 0; }
[ "$REFERENCE_STATE" != error ] || exit 1
state="$REFERENCE_STATE"
check='{"status":"COMPLETED","conclusion":"SUCCESS"}'
if [ "$state" = pending ]; then state=MERGED; check='{"status":"IN_PROGRESS"}'; fi
printf '{"number":7,"id":7,"state":"%s","title":"Synthetic reference","mergedAt":"2026-10-29T12:00:00Z","merged_on":"2026-10-29T12:00:00Z","mergeCommit":{"oid":"%s"},"statusCheckRollup":[%s]}\n' "$state" "$REFERENCE_HEAD" "$check"
`
				for _, name := range []string{"gh", "bb"} {
					fixtureFile(t, filepath.Join(bin, name), script, 0755)
				}
				t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
				if forge == "bitbucket.org" {
					t.Setenv("BB_USERNAME", "fixture")
					t.Setenv("BB_API_TOKEN", "synthetic")
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"values":[]}`) }))
					old := statusBBAPI
					statusBBAPI = server.URL
					t.Cleanup(func() { statusBBAPI = old; server.Close() })
				}
				segment := "pull"
				if forge == "bitbucket.org" {
					segment = "pull-requests"
				}
				record := PRRecord{Repo: "reference", Number: "7", Branch: "topic", URL: "https://" + forge + "/example/widget/" + segment + "/7"}
				if forge == "github.com" {
					record.URL = strings.Replace(record.URL, "/example/widget/", "/Example/Widget/", 1)
				}
				record.URL += "?diff=split#discussion"
				if _, err := c.EnlistPR(record); err != nil {
					t.Fatal(err)
				}
				snapshot, err := c.statusForgePreview(false, true)
				if err != nil {
					t.Fatal(err)
				}
				if state == "CLOSED" {
					if len(snapshot.PRs) != 0 || len(snapshot.Orphans) != 0 {
						t.Fatalf("closed reference gained a live row/orphan: %+v", snapshot.PRs)
					}
				} else {
					want := state
					if state == "error" {
						want = "UNKNOWN"
					}
					if state == "pending" {
						want = "MERGED"
					}
					if len(snapshot.PRs) != 1 || snapshot.PRs[0].State != want {
						t.Fatalf("URL-only reference: %+v, want %s", snapshot.PRs, want)
					}
					row := snapshot.PRs[0]
					if row.URL == nil || *row.URL != "https://"+forge+"/example/widget/"+segment+"/7" {
						t.Fatalf("noncanonical displayed URL: %+v", row.URL)
					}
					if row.Slug != "example/widget" || row.OnBranch || row.worktreeDir != "" {
						t.Fatalf("wrong forge or checkout association: %+v", row)
					}
				}
				if raw, _ := os.ReadFile(calls); !strings.Contains(string(raw), "pr view 7") {
					t.Fatalf("no by-number forge lookup: %s", raw)
				}
				records, _ := c.ListPRs()
				if (records[0].MergedOn != "") != (state == "MERGED") {
					t.Fatalf("incorrect settled-merge recording: %+v", records)
				}
				for _, row := range snapshot.Repos {
					if row.PR != nil {
						t.Fatalf("reference attached to checkout: %+v", row)
					}
				}
				if got := fixtureGit(t, "-C", c.Harness, "branch", "--show-current"); got != "topic" {
					t.Fatal("reference moved local branch")
				}
				if got := fixtureGit(t, "-C", c.Harness, "rev-parse", "HEAD"); got != head {
					t.Fatal("reference changed HEAD")
				}
				if got := fixtureGit(t, "-C", c.Harness, "status", "--porcelain"); got != before {
					t.Fatal("reference changed worktree")
				}
				if repo, err := c.prRepositories().resolve("reference"); repo != nil || err != nil {
					t.Fatalf("reference registered local identity: %+v %v", repo, err)
				}
				if _, err := os.Stat(filepath.Join(c.Collection, "reference")); !os.IsNotExist(err) {
					t.Fatal("reference created checkout")
				}
				if _, _, err := c.CatchUpInventory(CatchUpOptions{Repos: []string{"reference"}, DryRun: true}); err == nil {
					t.Fatal("reference became a catch-up selector")
				}
			})
		}
	}
}

func TestStatusPRURLIdentityValidation(t *testing.T) {
	for _, tc := range []struct{ name, url, number, slug, forge string }{
		{"github", "https://github.com/Example/Widget/pull/7", "7", "example/widget", "github.com"},
		{"bitbucket", "https://bitbucket.org/Example/Widget/pull-requests/7/", "7", "Example/Widget", "bitbucket.org"},
		{"view-only-suffix", "https://GITHUB.COM/example/widget/pull/7?diff=split#discussion", "7", "example/widget", "github.com"},
		{"credentials", "https://fixture:synthetic-secret@github.com/example/widget/pull/7", "7", "", ""},
		{"user", "https://fixture@github.com/example/widget/pull/7", "7", "", ""},
		{"unsupported-host", "https://example.invalid/example/widget/pull/7", "7", "", ""},
		{"host-suffix", "https://github.com.example.invalid/example/widget/pull/7", "7", "", ""},
		{"http", "http://github.com/example/widget/pull/7", "7", "", ""},
		{"port", "https://github.com:443/example/widget/pull/7", "7", "", ""},
		{"number-mismatch", "https://github.com/example/widget/pull/8", "7", "", ""},
		{"zero", "https://github.com/example/widget/pull/0", "0", "", ""},
		{"leading-zero", "https://github.com/example/widget/pull/07", "7", "", ""},
		{"encoded-path", "https://github.com/example/%77idget/pull/7", "7", "", ""},
		{"dot-path", "https://github.com/example/../pull/7", "7", "", ""},
		{"extra-segment", "https://github.com/example/widget/pull/7/files", "7", "", ""},
		{"wrong-forge-route", "https://bitbucket.org/example/widget/pull/7", "7", "", ""},
		{"missing-slug", "https://github.com/widget/pull/7", "7", "", ""},
		{"malformed", "https://github.com/%", "7", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			slug, forge, err := statusParsePRURL(tc.url, tc.number)
			if slug != tc.slug || forge != tc.forge || (err != nil) != (tc.forge == "") {
				t.Fatalf("identity %q %q, error %v", slug, forge, err)
			}
			if err != nil && strings.Contains(err.Error(), "synthetic-secret") {
				t.Fatal("diagnostic leaks URL credentials")
			}
		})
	}
}

func TestStatusPRReferencePreservesCanonicalIdentity(t *testing.T) {
	good := &prRepository{name: "widget", remote: "git@github.com:Example/Widget.git"}
	for _, tc := range []struct {
		name  string
		repos prRepositories
		url   string
		want  bool
	}{
		{"matching-registry", prRepositories{"widget": {good}}, "https://github.com/example/widget/pull/7", true},
		{"different-owner", prRepositories{"widget": {good}}, "https://github.com/other/widget/pull/7", false},
		{"different-repo", prRepositories{"widget": {good}}, "https://github.com/example/other/pull/7", false},
		{"different-forge", prRepositories{"widget": {good}}, "https://bitbucket.org/Example/Widget/pull-requests/7", false},
		{"unsupported-registry", prRepositories{"widget": {&prRepository{remote: "https://example.invalid/widget.git"}}}, "https://github.com/example/widget/pull/7", false},
		{"ambiguous-alias", prRepositories{"widget": {good, &prRepository{remote: "https://github.com/other/widget.git"}}}, "https://github.com/example/widget/pull/7", false},
		{"ambiguous-checkout", prRepositories{"widget": {&prRepository{remote: good.remote, dirs: []string{"widget", "ext.widget"}}}}, "https://github.com/example/widget/pull/7", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			slug, forge, err := statusReadOnlyPRForge(tc.repos, PRRecord{Repo: "widget", Number: "7", URL: tc.url})
			if (err == nil) != tc.want {
				t.Fatalf("identity %s %s, error %v", slug, forge, err)
			}
			if !tc.want && (slug != "" || forge != "") {
				t.Fatal("conflicting URL returned lookup identity")
			}
		})
	}
}

func TestStatusPRReferenceInvalidURLIsNotQueriedOrRendered(t *testing.T) {
	for _, tc := range []struct{ name, url, remote string }{
		{"credentials", "https://fixture:synthetic-secret@github.com/example/widget/pull/7", ""},
		{"unsupported-host", "https://example.invalid/example/widget/pull/7", ""},
		{"number-mismatch", "https://github.com/example/widget/pull/8", ""},
		{"registry-conflict", "https://github.com/other/widget/pull/7", "https://github.com/example/widget.git"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TMPDIR", t.TempDir())
			c := newWorkspaceFixture(t)
			if tc.remote != "" {
				c.Registry.Repos = append(c.Registry.Repos, Repo{Name: "reference", Remote: tc.remote})
			}
			var progress []string
			c.StatusProgress = func(s string) { progress = append(progress, s) }
			calls := filepath.Join(t.TempDir(), "calls")
			t.Setenv("REFERENCE_CALLS", calls)
			bin := t.TempDir()
			fixtureFile(t, filepath.Join(bin, "gh"), "#!/bin/sh\nprintf 'called\\n' >> \"$REFERENCE_CALLS\"\nexit 1\n", 0755)
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			record := PRRecord{Repo: "reference", Number: "7", URL: tc.url}
			if _, err := c.EnlistPR(record); err != nil {
				t.Fatal(err)
			}
			snapshot, err := c.statusForgePreview(false, true)
			if err != nil || len(snapshot.PRs) != 1 || snapshot.PRs[0].State != "UNKNOWN" || snapshot.PRs[0].URL != nil {
				t.Fatalf("invalid reference: %+v %v", snapshot.PRs, err)
			}
			if _, err := os.Stat(calls); !os.IsNotExist(err) {
				t.Fatal("invalid reference contacted forge")
			}
			all := strings.Join(progress, "\n")
			if !strings.Contains(all, "Cannot look up enlisted PR reference#7:") {
				t.Fatalf("missing diagnostic: %s", all)
			}
			if strings.Contains(all, "synthetic-secret") || strings.Contains(all, tc.url) {
				t.Fatal("diagnostic echoes untrusted URL")
			}
		})
	}
}

func TestStatusPRReferenceRegisteredRowCanonicalURL(t *testing.T) {
	c, _ := enlistedIdentityFixture(t, true)
	record := PRRecord{Repo: "widget", Number: "7", Branch: "topic", URL: "https://github.com/Example/Widget/pull/7?diff=split#discussion"}
	if _, err := c.EnlistPR(record); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	fixtureFile(t, filepath.Join(bin, "gh"), `#!/bin/sh
if [ "$2" = list ]; then printf '[]\n'; else printf '%s\n' '{"number":7,"state":"OPEN","title":"Synthetic registered PR"}'; fi
`, 0755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	snapshot, err := c.statusForgePreview(false, false)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range snapshot.Repos {
		if row.Dir == "ext.widget" {
			found = true
			if row.PR == nil || row.PR.URL != "https://github.com/example/widget/pull/7" {
				t.Fatalf("registered row lost canonical link: %+v", row.PR)
			}
		}
	}
	if !found {
		t.Fatal("registered checkout missing")
	}
}

func TestStatusPRReferenceRecordedFactsSurviveLookupRefusal(t *testing.T) {
	for _, mode := range []string{"conflicting", "ambiguous", "invalid"} {
		t.Run(mode, func(t *testing.T) {
			c, path := enlistedIdentityFixture(t, true)
			fixtureGit(t, "-C", path, "checkout", "--detach", "HEAD")
			record := PRRecord{Repo: "widget", Number: "7", Branch: "topic", URL: "https://github.com/other/widget/pull/7", MergedOn: "2026-10-29T12:00:00Z", FinalChecks: "SUCCESS", MergeCommit: strings.Repeat("a", 40)}
			if mode == "invalid" {
				record.URL = "https://fixture:synthetic-secret@github.com/example/widget/pull/7"
			}
			if mode == "ambiguous" {
				record.URL = "https://github.com/example/widget/pull/7"
				other := filepath.Join(c.Collection, "widget")
				enlistedIdentityGit(t, other, "https://github.com/example/widget.git")
				fixtureGit(t, "-C", other, "checkout", "--detach", "HEAD")
			}
			fixtureFile(t, c.PRFile(), formatPR(record)+"\n# merged-pr widget 7 "+record.MergedOn+" SUCCESS\n# merged-commit widget 7 "+record.MergeCommit+" main\n", 0644)
			snapshot, err := c.statusForgePreview(false, false)
			if err != nil || len(snapshot.PRs) != 1 {
				t.Fatalf("snapshot: %+v %v", snapshot.PRs, err)
			}
			row := snapshot.PRs[0]
			if row.State != "MERGED" || row.MergedOn == nil || *row.MergedOn != record.MergedOn || row.MergeCommit != record.MergeCommit {
				t.Fatalf("lookup refusal erased recorded facts: %+v", row)
			}
			if row.OnBranch || row.worktreeDir != "" || row.URL != nil {
				t.Fatalf("refused URL acquired local authority/link: %+v", row)
			}
		})
	}
}

func TestStatusPRReferenceRegisteredBrowserURLKeepsOpenBlocker(t *testing.T) {
	for _, raw := range []string{"https://github.com/example/widget/pull/7/files", "http://github.com/example/widget/pull/7"} {
		t.Run(raw, func(t *testing.T) {
			c, path := enlistedIdentityFixture(t, true)
			fixtureGit(t, "-C", path, "checkout", "--detach", "HEAD")
			if _, err := c.EnlistPR(PRRecord{Repo: "widget", Number: "7", Branch: "topic", URL: raw}); err != nil {
				t.Fatal(err)
			}
			bin := t.TempDir()
			fixtureFile(t, filepath.Join(bin, "gh"), `#!/bin/sh
if [ "$2" = list ]; then printf '[]\n'; else printf '%s\n' '{"number":7,"state":"OPEN","title":"Synthetic registered PR"}'; fi
`, 0755)
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			snapshot, err := c.statusForgePreview(false, false)
			if err != nil || len(snapshot.PRs) != 1 || snapshot.PRs[0].State != "OPEN" {
				t.Fatalf("noncanonical registered URL hid PR: %+v %v", snapshot.PRs, err)
			}
			if snapshot.PRs[0].URL == nil || *snapshot.PRs[0].URL != "https://github.com/example/widget/pull/7" {
				t.Fatal("browser URL not canonicalized")
			}
			snapshot.Retirement = []RetirementAssessment{{Collection: snapshot.Collection}}
			statusRetirementFacts(&snapshot)
			if !strings.Contains(strings.Join(snapshot.Retirement[0].Blockers, "\n"), "pull request #7 remains open in status") {
				t.Fatal("open-PR retirement blocker disappeared")
			}
		})
	}
}
