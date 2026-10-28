package wtc

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestStatusSquashReproduction(t *testing.T) {
	for _, late := range []bool{false, true} {
		t.Run(map[bool]string{false: "fully-landed", true: "late-push"}[late], func(t *testing.T) {
			c, target, squash := squashFixture(t, true)
			head := fixtureGit(t, "-C", target.path, "rev-parse", "HEAD")
			fixtureGit(t, "-C", target.path, "config", "branch.topic.remote", "origin")
			fixtureGit(t, "-C", target.path, "config", "branch.topic.merge", "refs/heads/topic")
			if late {
				fixtureFile(t, filepath.Join(target.path, "followup.txt"), "follow-up\n", 0644)
				fixtureGit(t, "-C", target.path, "add", "followup.txt")
				fixtureGit(t, "-C", target.path, "-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "follow-up")
			}
			fixtureFile(t, c.PRFile(), "widget 7 topic - Synthetic change\n# merged-pr widget 7 2026-10-27T12:00:00Z SUCCESS\n# merged-commit widget 7 "+squash+" main\n# merged-head widget 7 "+squash+" "+head+"\n", 0644)
			snapshot, err := c.statusForgePreview(false, false)
			if err != nil {
				t.Fatal(err)
			}
			data, _ := json.Marshal(snapshot)
			t.Logf("late=%v raw ahead=%d; merge message=%q", late, snapshot.Repos[1].Ahead, fixtureGit(t, "-C", target.path, "show", "-s", "--format=%s", squash))
			var payload StatusSnapshot
			if err := json.Unmarshal(data, &payload); err != nil {
				t.Fatal(err)
			}
			want := "landed"
			if late {
				want = "follow-up"
			}
			if len(payload.PRs) != 1 || payload.PRs[0].Landing == nil || payload.PRs[0].Landing.State != want {
				t.Errorf("want %s classification; got %s", want, data)
			}
			if !strings.Contains(string(data), `"upstream_state":"gone"`) || !strings.Contains(string(data), `"ahead_ref":"origin/main"`) {
				t.Errorf("missing ahead basis/gone upstream: %s", data)
			}
			if !catchUpGitOK(target.path, "rev-parse", "--verify", "refs/heads/topic") {
				t.Fatal("status removed local branch")
			}
		})
	}
}

func TestStatusSquashEvidenceBoundaries(t *testing.T) {
	for _, kind := range []string{"normal", "custom-message", "tip-revert", "tip-changes", "tip-rename", "partial-unknown", "missing-merge", "wrong-merge", "unavailable-head", "diverged-head"} {
		t.Run(kind, func(t *testing.T) {
			_, target, merge := squashFixture(t, true)
			head := fixtureGit(t, "-C", target.path, "rev-parse", "HEAD")
			detail := statusPRDetail{State: "MERGED", MergeCommit: merge, HeadCommit: head}
			want := "landed"
			switch kind {
			case "normal":
				fixtureGit(t, "-C", target.path, "update-ref", "refs/remotes/origin/main", head)
			case "custom-message": // The fixture's squash message is a filename, with no PR/branch reference.
			case "tip-revert", "tip-changes":
				fixtureGit(t, "-C", target.path, "config", "user.name", "fixture")
				fixtureGit(t, "-C", target.path, "config", "user.email", "fixture@example.invalid")
				fixtureGit(t, "-C", target.path, "switch", "--detach", "origin/main")
				if kind == "tip-revert" {
					fixtureGit(t, "-C", target.path, "revert", "--no-edit", merge)
				} else {
					fixtureFile(t, filepath.Join(target.path, "feature.txt"), "one\ntwo\nunrelated later edit\n", 0644)
					fixtureGit(t, "-C", target.path, "add", "feature.txt")
					fixtureGit(t, "-C", target.path, "commit", "-qm", "later base change")
				}
				fixtureGit(t, "-C", target.path, "update-ref", "refs/remotes/origin/main", "HEAD")
				fixtureGit(t, "-C", target.path, "switch", "topic")
				detail.HeadCommit = ""
			case "tip-rename":
				fixtureGit(t, "-C", target.path, "config", "user.name", "fixture")
				fixtureGit(t, "-C", target.path, "config", "user.email", "fixture@example.invalid")
				fixtureGit(t, "-C", target.path, "switch", "--detach", "origin/main")
				fixtureGit(t, "-C", target.path, "mv", "feature.txt", "renamed.txt")
				fixtureGit(t, "-C", target.path, "commit", "-qm", "rename after landing")
				fixtureGit(t, "-C", target.path, "update-ref", "refs/remotes/origin/main", "HEAD")
				fixtureGit(t, "-C", target.path, "switch", "topic")
				detail.HeadCommit = "" // cumulative patch still establishes historical landing
			case "partial-unknown", "missing-merge", "wrong-merge", "diverged-head":
				fixtureFile(t, filepath.Join(target.path, "feature.txt"), "one\ntwo\nthree\n", 0644)
				fixtureGit(t, "-C", target.path, "add", "feature.txt")
				fixtureGit(t, "-C", target.path, "-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "later hunk")
				want = "unknown"
				switch kind {
				case "partial-unknown":
					detail.HeadCommit = ""
				case "missing-merge":
					detail.MergeCommit = strings.Repeat("a", 40)
				case "wrong-merge":
					detail.MergeCommit = fixtureGit(t, "-C", target.path, "rev-parse", merge+"^")
					detail.HeadCommit = ""
				case "diverged-head":
					detail.HeadCommit = merge
				}
			case "unavailable-head":
				detail.HeadCommit = strings.Repeat("a", 40)
			}
			row, err := statusLocalRepo(target)
			if err != nil {
				t.Fatal(err)
			}
			got := statusMergedLanding(row, detail)
			if got.State != want {
				t.Fatalf("got %+v; want %s", got, want)
			}
		})
	}
}

func TestStatusMergedHeadProvenanceAndRoundTrip(t *testing.T) {
	c := fixture(t)
	const merge = "1111111111111111111111111111111111111111"
	const head = "2222222222222222222222222222222222222222"
	_, err := c.EnlistPR(PRRecord{Repo: "widget", Number: "7", Branch: "topic"})
	if err != nil {
		t.Fatal(err)
	}
	records, _ := c.ListPRs()
	detail := statusPRDetail{Number: "7", State: "MERGED", MergedOn: "2026-10-27T12:00:00Z", Checks: "SUCCESS", MergeCommit: merge, HeadCommit: head, Base: "main"}
	if n, err := c.recordMergedPRs(records, []statusPRDetail{detail}); err != nil || n != 1 {
		t.Fatalf("record: %d %v", n, err)
	}
	records, err = c.ListPRs()
	if err != nil || records[0].MergeHead != head {
		t.Fatalf("round trip: %+v %v", records, err)
	}
	details, err := c.statusEnrichRecords(records, nil)
	if err != nil || details[0].HeadCommit != head {
		t.Fatalf("replay: %+v %v", details, err)
	}
	// Another PR's marker and a marker for another merge cannot supply this head.
	fixtureFile(t, c.PRFile(), "widget 7 topic\n# merged-pr widget 7 2026-10-27T12:00:00Z SUCCESS\n# merged-commit widget 7 "+merge+" main\n# merged-head widget 8 "+merge+" "+head+"\n# merged-head widget 7 "+head+" "+head+"\n", 0644)
	records, err = c.ListPRs()
	if err != nil || records[0].MergeHead != "" {
		t.Fatalf("mixed evidence: %+v %v", records, err)
	}
	if err := c.UnlistPR("widget", "7"); err != nil {
		t.Fatal(err)
	}
	records, err = c.ListPRs()
	if err != nil || len(records) != 0 {
		t.Fatalf("unlist: %+v %v", records, err)
	}
}

func TestStatusAheadBasisTracksExistingUpstream(t *testing.T) {
	_, target, _ := squashFixture(t, false)
	fixtureGit(t, "-C", target.path, "update-ref", "refs/remotes/origin/topic", "HEAD")
	fixtureGit(t, "-C", target.path, "config", "branch.topic.remote", "origin")
	fixtureGit(t, "-C", target.path, "config", "branch.topic.merge", "refs/heads/topic")
	row, err := statusLocalRepo(target)
	if err != nil || row.UpstreamState != "tracking" || row.AheadBasis != "upstream" || row.AheadRef != "origin/topic" || row.Ahead != 0 {
		t.Fatalf("tracking: %+v %v", row, err)
	}
	fixtureGit(t, "-C", target.path, "config", "--remove-section", "branch.topic")
	row, err = statusLocalRepo(target)
	if err != nil || row.UpstreamState != "none" || row.AheadBasis != "development-tip" || row.Ahead != 2 {
		t.Fatalf("no upstream: %+v %v", row, err)
	}
}
