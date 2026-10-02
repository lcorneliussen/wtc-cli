package wtc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCatchUpInventoryUsesTargetIdentityAndRejectsUnknownSelector(t *testing.T) {
	c := newWorkspaceFixture(t)
	result, err := c.NewCollection(NewOptions{Slug: "other", Repos: []string{"widget"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Collection == "" {
		t.Fatal("collection was not created")
	}
	t.Setenv("WTC_HARNESS_REPO", "widget") // Inherited identity must not redirect harness.
	report, targets, err := c.CatchUpInventory(CatchUpOptions{Collections: []string{"missing", "other"}, Repos: []string{"harness"}, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].repo != "agent-harness" || !targets[0].harness || targets[0].collection != "other" {
		t.Fatalf("wrong selected target: %+v", targets)
	}
	if report.ExitStatus != 1 || len(report.Outcomes) != 1 || report.Outcomes[0].Kind != "collection" {
		t.Fatalf("missing collection was not reported independently: %+v", report)
	}
	before := fixtureGit(t, "-C", targets[0].path, "rev-parse", "HEAD")
	report, targets, err = c.CatchUpInventory(CatchUpOptions{Collections: []string{"other"}, Repos: []string{"typo"}})
	if err == nil || len(targets) != 0 || len(report.Outcomes) != 1 || report.Outcomes[0].Kind != "selection" {
		t.Fatalf("selector typo did not stop the sweep: %+v %+v %v", report, targets, err)
	}
	if after := fixtureGit(t, "-C", filepath.Join(result.Collection, "harness"), "rev-parse", "HEAD"); after != before {
		t.Fatal("preflight changed a worktree")
	}
	reportPath := filepath.Join(c.Collection, "preflight.json")
	report, err = c.CatchUp(CatchUpOptions{Collections: []string{"other"}, Repos: []string{"typo"}, DryRun: true, Report: reportPath})
	if err == nil || report.ExitStatus != 1 {
		t.Fatalf("preflight failure was hidden: %+v %v", report, err)
	}
	if _, err := os.Stat(reportPath); err != nil {
		t.Fatalf("preflight report was not saved: %v", err)
	}
}

func TestCatchUpUsesOpenPRMergeTarget(t *testing.T) {
	c := newWorkspaceFixture(t)
	for _, tc := range []struct {
		pr   catchUpPR
		want string
		err  bool
	}{
		{catchUpPR{state: "NONE"}, "origin/main", false},
		{catchUpPR{state: "MERGED"}, "origin/main", false},
		{catchUpPR{state: "OPEN", base: "release/next"}, "origin/release/next", false},
		{catchUpPR{state: "DRAFT", base: "develop"}, "origin/develop", false},
		{catchUpPR{state: "OPEN"}, "", true},
		{catchUpPR{state: "OPEN", base: "../../bad"}, "", true},
	} {
		got, err := catchUpRefForPR(c.Harness, "origin/main", tc.pr)
		if got != tc.want || (err != nil) != tc.err {
			t.Fatalf("PR %+v: ref %q, error %v", tc.pr, got, err)
		}
	}
}

func TestCatchUpHandsOffMissingOpenPRTarget(t *testing.T) {
	c := newWorkspaceFixture(t)
	fixtureGit(t, "-C", c.Harness, "switch", "-qc", "topic")
	before := fixtureGit(t, "-C", c.Harness, "rev-parse", "HEAD")
	owner := filepath.Join(c.Workspace, ".bare", "agent-harness.git")
	fixtureGit(t, "--git-dir="+owner, "remote", "set-url", "origin", "git@github.com:example/agent-harness.git")
	fixtureFile(t, filepath.Join(c.Collection, ".wtc-prs"), "agent-harness 7 topic - fixture\n", 0644)
	bin := filepath.Join(c.Workspace, "bin")
	fixtureFile(t, filepath.Join(bin, "gh"), "#!/bin/sh\nprintf '%s\\n' '{\"state\":\"OPEN\",\"baseRefName\":\"release/missing\"}'\n", 0755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	report, _ := c.CatchUp(CatchUpOptions{Repos: []string{"harness"}, DryRun: true, NoSkills: true, NoMCP: true, NoEnv: true, NoSecrets: true})
	for _, row := range report.Outcomes {
		if row.Kind == "repo" {
			if row.Outcome != "needs-owner" || row.TargetRef != "origin/release/missing" || row.NextAction == "" ||
				!strings.Contains(row.Reason, "merge target") {
				t.Fatalf("missing PR target not handed off: %+v", row)
			}
			if after := fixtureGit(t, "-C", c.Harness, "rev-parse", "HEAD"); after != before {
				t.Fatal("missing PR target moved the branch")
			}
			return
		}
	}
	t.Fatalf("missing repo outcome: %+v", report)
}

func TestCatchUpInventorySelectsSymlinkedWorktreeDirectory(t *testing.T) {
	c := newWorkspaceFixture(t)
	owner := filepath.Join(c.Workspace, ".bare", "widget.git")
	fixtureGit(t, "--git-dir="+owner, "worktree", "add", "--detach", filepath.Join(c.Collection, "widget-real"), "origin/main")
	if err := os.Symlink("widget-real", filepath.Join(c.Collection, "widget-link")); err != nil {
		t.Fatal(err)
	}
	report, targets, err := c.CatchUpInventory(CatchUpOptions{Repos: []string{"widget-link"}, DryRun: true})
	if err != nil || report.ExitStatus != 0 || len(targets) != 1 || filepath.Base(targets[0].path) != "widget-link" {
		t.Fatalf("symlinked worktree was not selected: %+v %+v %v", report, targets, err)
	}
}

func TestCatchUpReportKeepsRowsWhenPersistenceFails(t *testing.T) {
	c := newWorkspaceFixture(t)
	report := CatchUpReport{SchemaVersion: 1, Initiator: c.Collection, Outcomes: []CatchUpRow{}}
	report.add("repo", "main", "widget", "needs-owner", "café | 日本", "old", "new", "old")
	report.add("repo", "main", "widget", "needs-owner", "merge conflict; aborted to original tree; paths: README.md", "old", "new", "old")
	if report.ExitStatus != 1 || !strings.Contains(report.Markdown(), "café \\| 日本") {
		t.Fatalf("incomplete readable report: %+v", report)
	}
	if !strings.Contains(report.Markdown(), "resolve conflicts") || !strings.Contains(report.Outcomes[1].NextAction, "push only if its PR is open or draft") {
		t.Fatalf("conflict handoff missing: %+v", report)
	}
	path := filepath.Join(c.Collection, "report.json")
	if err := SaveCatchUpReport(&report, path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var persisted CatchUpReport
	if err := json.Unmarshal(data, &persisted); err != nil || len(persisted.Outcomes) != 2 {
		t.Fatalf("unreadable persisted report: %v", err)
	}
	blockedPath := filepath.Join(c.Collection, "blocked.json")
	if err := os.Mkdir(blockedPath+".md", 0755); err != nil {
		t.Fatal(err)
	}
	if err := SaveCatchUpReport(&report, blockedPath); err == nil {
		t.Fatal("failed Markdown write was hidden")
	}
	data, err = os.ReadFile(blockedPath)
	if err != nil || json.Unmarshal(data, &persisted) != nil || persisted.ReportError == "" || persisted.ExitStatus != 1 {
		t.Fatalf("saved JSON did not record Markdown failure: %q %v", data, err)
	}
	if err := SaveCatchUpReport(&report, filepath.Join(c.Collection, "missing", "report.json")); err == nil || report.ReportError == "" {
		t.Fatal("report persistence failure was hidden")
	}
}

func TestCatchUpUpdatesDirtyLocalTreeAndReportsPartialFailure(t *testing.T) {
	c := newWorkspaceFixture(t)
	source := filepath.Join(c.Workspace, "source-agent-harness")
	fixtureFile(t, filepath.Join(source, "README.md"), "new upstream content\n", 0644)
	fixtureGit(t, "-C", source, "add", "README.md")
	fixtureGit(t, "-C", source, "-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "upstream")
	want := fixtureGit(t, "-C", source, "rev-parse", "HEAD")
	fixtureFile(t, filepath.Join(c.Harness, "local-note"), "keep me\n", 0644)
	report, err := c.CatchUp(CatchUpOptions{Repos: []string{"harness"}, NoSkills: true, NoMCP: true, NoEnv: true, NoSecrets: true})
	if err != nil || report.ExitStatus != 0 {
		t.Fatalf("local catch-up failed: %+v %v", report, err)
	}
	if got := fixtureGit(t, "-C", c.Harness, "rev-parse", "HEAD"); got != want {
		t.Fatalf("harness stayed at %s, want %s", got, want)
	}
	if got, err := os.ReadFile(filepath.Join(c.Harness, "local-note")); err != nil || string(got) != "keep me\n" {
		t.Fatalf("dirty change was lost: %q %v", got, err)
	}
	if got := fixtureGit(t, "-C", c.Harness, "stash", "list"); got != "" {
		t.Fatalf("stash was not restored: %s", got)
	}
	if _, err := os.Stat(filepath.Join(c.Collection, ".wtc-catch-up.json.md")); err != nil {
		t.Fatalf("missing readable report: %v", err)
	}

	// A failed owner fetch must not consume stale refs, and the next selected
	// repository still receives a result row.
	widgetOwner := filepath.Join(c.Workspace, ".bare", "widget.git")
	fixtureGit(t, "--git-dir="+widgetOwner, "remote", "set-url", "origin", filepath.Join(c.Workspace, "missing"))
	fixtureGit(t, "--git-dir="+widgetOwner, "worktree", "add", "--detach", filepath.Join(c.Collection, "widget"), "origin/main")
	report, err = c.CatchUp(CatchUpOptions{Repos: []string{"harness", "widget"}, NoSkills: true, NoMCP: true, NoEnv: true, NoSecrets: true})
	if err == nil || report.ExitStatus != 1 {
		t.Fatalf("partial failure was hidden: %+v %v", report, err)
	}
	seenHarness, seenWidget := false, false
	for _, row := range report.Outcomes {
		if row.Kind == "repo" && row.Repo == "agent-harness" && row.Outcome == "current" {
			seenHarness = true
		}
		if row.Kind == "repo" && row.Repo == "widget" && row.Outcome == "failed" && strings.Contains(row.Reason, "fetch failed") {
			seenWidget = true
		}
	}
	if !seenHarness || !seenWidget {
		t.Fatalf("missing independent outcomes: %+v", report.Outcomes)
	}
}

func TestCatchUpStatusReloadOnlyTouchesVerifiedStatusPane(t *testing.T) {
	c := newWorkspaceFixture(t)
	bin := filepath.Join(c.Workspace, "bin")
	log := filepath.Join(c.Workspace, "herdr.log")
	state := filepath.Join(c.Workspace, "pane-state")
	fixtureFile(t, filepath.Join(c.Harness, "tools", "wtc-status-tui.sh"), "#!/bin/sh\n", 0755)
	fixtureFile(t, filepath.Join(bin, "herdr"), `#!/bin/sh
printf '%s\n' "$*" >> "$MOCK_HERDR_LOG"
shift 2
case "$1 $2" in
  'workspace list') printf '%s\n' '{"result":{"workspaces":[{"label":"main","workspace_id":"w1"}]}}' ;;
  'pane list') printf '%s\n' '{"result":{"panes":[{"label":"status","pane_id":"w1:p3"},{"label":"agent","pane_id":"w1:p1","agent":"codex"}]}}' ;;
  'pane process-info')
    if [ -f "$MOCK_HERDR_STATE/restarted" ]; then args='["bash","./harness/tools/wtc-status-tui.sh"]'
    elif [ -f "$MOCK_HERDR_STATE/interrupted" ]; then args='["zsh"]'
    elif [ -f "$MOCK_HERDR_STATE/unrelated" ]; then args='["nvim"]'
    else args='["bash","./harness/tools/wtc-status-tui.sh"]'; fi
    printf '{"result":{"process_info":{"foreground_process_group_id":42,"foreground_processes":[{"pid":43,"argv":["renderer"]},{"pid":42,"argv":%s}]}}}\n' "$args" ;;
  'pane send-keys') touch "$MOCK_HERDR_STATE/interrupted" ;;
  'pane run') touch "$MOCK_HERDR_STATE/restarted" ;;
esac
`, 0755)
	if err := os.Mkdir(state, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	t.Setenv("HERDR_SESSION", "fixture")
	t.Setenv("MOCK_HERDR_LOG", log)
	t.Setenv("MOCK_HERDR_STATE", state)
	target := catchUpTarget{collection: "main", path: c.Harness, repo: "agent-harness", harness: true}
	report := CatchUpReport{Initiator: c.Collection}
	fixtureFile(t, filepath.Join(state, "unrelated"), "", 0644)
	c.catchUpReloadStatus(&report, target, CatchUpOptions{ReloadStatus: true})
	if len(report.Outcomes) != 1 || report.Outcomes[0].Outcome != "needs-owner" {
		t.Fatalf("unrelated process was accepted: %+v", report.Outcomes)
	}
	data, _ := os.ReadFile(log)
	if strings.Contains(string(data), "pane send-keys") || strings.Contains(string(data), "pane run") {
		t.Fatal("unrelated process received pane keys")
	}
	if err := os.Remove(filepath.Join(state, "unrelated")); err != nil {
		t.Fatal(err)
	}
	report.Outcomes = nil
	c.catchUpReloadStatus(&report, target, CatchUpOptions{ReloadStatus: true})
	if len(report.Outcomes) != 1 || report.Outcomes[0].Outcome != "restarted" {
		t.Fatalf("verified status pane was not restarted: %+v", report.Outcomes)
	}
	data, _ = os.ReadFile(log)
	if strings.Contains(string(data), "w1:p1") || !strings.Contains(string(data), "pane send-keys w1:p3 ctrl+c") || !strings.Contains(string(data), "pane run w1:p3") {
		t.Fatalf("wrong pane activity: %s", data)
	}
}

func TestCatchUpReadsEnlistedBitbucketStateWithoutGuessing(t *testing.T) {
	c := newWorkspaceFixture(t)
	owner := filepath.Join(c.Workspace, ".bare", "agent-harness.git")
	fixtureGit(t, "--git-dir="+owner, "remote", "set-url", "origin", "git@bitbucket.org:example/agent-harness.git")
	fixtureFile(t, filepath.Join(c.Collection, ".wtc-prs"), "agent-harness 7 topic - fixture\n", 0644)
	bin := filepath.Join(c.Workspace, "bin")
	fixtureFile(t, filepath.Join(bin, "bb"), "#!/bin/sh\nprintf '%s\\n' '{\"state\":\"OPEN\",\"draft\":true,\"source\":{\"commit\":{\"hash\":\"abc\"}},\"destination\":{\"branch\":{\"name\":\"release/next\"}}}'\n", 0755)
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	target := catchUpTarget{collection: "main", path: c.Harness, repo: "agent-harness", harness: true}
	if got := c.catchUpPRState(target, "topic"); got.state != "DRAFT" || got.base != "release/next" {
		t.Fatalf("draft was not protected: %+v", got)
	}
	if got := c.catchUpPRState(target, "other"); got.state != "UNKNOWN" {
		t.Fatalf("unenlisted branch was guessed: %+v", got)
	}
}

func TestCatchUpRejectsUnrecognizedForgeState(t *testing.T) {
	c := newWorkspaceFixture(t)
	owner := filepath.Join(c.Workspace, ".bare", "agent-harness.git")
	fixtureGit(t, "--git-dir="+owner, "remote", "set-url", "origin", "git@github.com:example/agent-harness.git")
	fixtureFile(t, filepath.Join(c.Collection, ".wtc-prs"), "agent-harness 7 topic - fixture\n", 0644)
	bin := filepath.Join(c.Workspace, "bin")
	fixtureFile(t, filepath.Join(bin, "gh"), "#!/bin/sh\nprintf '%s\\n' '{\"state\":\"FUTURE_STATE\",\"headRefOid\":\"abc\"}'\n", 0755)
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	target := catchUpTarget{collection: "main", path: c.Harness, repo: "agent-harness", harness: true}
	if got := c.catchUpPRState(target, "topic"); got.state != "UNKNOWN" {
		t.Fatalf("unrecognized PR state was trusted: %+v", got)
	}
}

func TestCatchUpReadsAllEnlistedPRsBeforeMovingBranch(t *testing.T) {
	c := newWorkspaceFixture(t)
	owner := filepath.Join(c.Workspace, ".bare", "agent-harness.git")
	fixtureGit(t, "--git-dir="+owner, "remote", "set-url", "origin", "git@github.com:example/agent-harness.git")
	fixtureFile(t, filepath.Join(c.Collection, ".wtc-prs"), "agent-harness 7 topic - first\nagent-harness 8 topic - second\n", 0644)
	bin := filepath.Join(c.Workspace, "bin")
	fixtureFile(t, filepath.Join(bin, "gh"), `#!/bin/sh
case " $* " in
  *' view 7 '*) printf '%s\n' '{"state":"OPEN"}' ;;
  *' view 8 '*) exit 1 ;;
esac
`, 0755)
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	target := catchUpTarget{collection: "main", path: c.Harness, repo: "agent-harness", harness: true}
	if got := c.catchUpPRState(target, "topic"); got.state != "UNKNOWN" {
		t.Fatalf("later failed lookup was ignored: %+v", got)
	}
}

func TestCatchUpReloadsRegistryAfterHarnessSelfUpdate(t *testing.T) {
	c := newWorkspaceFixture(t)
	created, err := c.NewCollection(NewOptions{Slug: "other", Repos: []string{"widget"}})
	if err != nil {
		t.Fatal(err)
	}
	harnessSource := filepath.Join(c.Workspace, "source-agent-harness")
	registryPath := filepath.Join(harnessSource, ".harness-repos.yml")
	registry, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	oldWidget := "name: widget\n    remote: " + filepath.Join(c.Workspace, "source-widget") + "\n    default_ref: origin/main"
	newWidget := strings.Replace(oldWidget, "origin/main", "origin/develop", 1)
	if !strings.Contains(string(registry), oldWidget) {
		t.Fatal("fixture registry shape changed")
	}
	fixtureFile(t, registryPath, strings.Replace(string(registry), oldWidget, newWidget, 1), 0644)
	fixtureGit(t, "-C", harnessSource, "add", ".harness-repos.yml")
	fixtureGit(t, "-C", harnessSource, "-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "change development ref")
	widgetSource := filepath.Join(c.Workspace, "source-widget")
	fixtureGit(t, "-C", widgetSource, "switch", "-qc", "develop")
	fixtureFile(t, filepath.Join(widgetSource, "README.md"), "develop content\n", 0644)
	fixtureGit(t, "-C", widgetSource, "add", "README.md")
	fixtureGit(t, "-C", widgetSource, "-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "develop")
	want := fixtureGit(t, "-C", widgetSource, "rev-parse", "HEAD")
	report, err := c.CatchUp(CatchUpOptions{Collections: []string{"other"}, Repos: []string{"harness", "widget"}, NoSkills: true, NoMCP: true, NoEnv: true, NoSecrets: true})
	if err != nil || report.ExitStatus != 0 {
		t.Fatalf("self-update failed: %+v %v", report, err)
	}
	if got := fixtureGit(t, "-C", filepath.Join(created.Collection, "widget"), "rev-parse", "HEAD"); got != want {
		t.Fatalf("widget did not follow new registry ref: %s, want %s", got, want)
	}
}

func TestCatchUpLegacyDefaultBranchOnlyFastForwards(t *testing.T) {
	c := newWorkspaceFixture(t)
	source := filepath.Join(c.Workspace, "source-agent-harness")
	fixtureGit(t, "-C", c.Harness, "switch", "-qc", "main")
	fixtureFile(t, filepath.Join(source, "README.md"), "upstream\n", 0644)
	fixtureGit(t, "-C", source, "add", "README.md")
	fixtureGit(t, "-C", source, "-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "upstream")
	want := fixtureGit(t, "-C", source, "rev-parse", "HEAD")
	opt := CatchUpOptions{Repos: []string{"harness"}, NoSkills: true, NoMCP: true, NoEnv: true, NoSecrets: true}
	if report, err := c.CatchUp(opt); err != nil || report.ExitStatus != 0 {
		t.Fatalf("fast-forward failed: %+v %v", report, err)
	}
	if got := fixtureGit(t, "-C", c.Harness, "rev-parse", "HEAD"); got != want {
		t.Fatalf("default branch did not fast-forward: %s", got)
	}
	fixtureFile(t, filepath.Join(c.Harness, "local-commit"), "local\n", 0644)
	fixtureGit(t, "-C", c.Harness, "add", "local-commit")
	fixtureGit(t, "-C", c.Harness, "-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "local")
	local := fixtureGit(t, "-C", c.Harness, "rev-parse", "HEAD")
	fixtureFile(t, filepath.Join(source, "README.md"), "more upstream\n", 0644)
	fixtureGit(t, "-C", source, "add", "README.md")
	fixtureGit(t, "-C", source, "-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "more upstream")
	report, err := c.CatchUp(opt)
	if err == nil || report.ExitStatus != 1 {
		t.Fatalf("divergent default branch was merged: %+v %v", report, err)
	}
	if got := fixtureGit(t, "-C", c.Harness, "rev-parse", "HEAD"); got != local {
		t.Fatalf("divergent local commit was moved: %s", got)
	}
	if catchUpGitOK(c.Harness, "rev-parse", "-q", "--verify", "MERGE_HEAD") {
		t.Fatal("divergent default branch left a merge in progress")
	}
}

func TestCatchUpPinsStashWhenRestoreConflicts(t *testing.T) {
	c := newWorkspaceFixture(t)
	source := filepath.Join(c.Workspace, "source-agent-harness")
	fixtureFile(t, filepath.Join(source, "README.md"), "upstream edit\n", 0644)
	fixtureGit(t, "-C", source, "add", "README.md")
	fixtureGit(t, "-C", source, "-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "upstream")
	want := fixtureGit(t, "-C", source, "rev-parse", "HEAD")
	fixtureFile(t, filepath.Join(c.Harness, "README.md"), "overlapping local edit\n", 0644)
	report, err := c.CatchUp(CatchUpOptions{Repos: []string{"harness"}, NoSkills: true, NoMCP: true, NoEnv: true, NoSecrets: true})
	if err == nil || report.ExitStatus != 1 {
		t.Fatalf("stash conflict was hidden: %+v %v", report, err)
	}
	if got := fixtureGit(t, "-C", c.Harness, "rev-parse", "HEAD"); got != want {
		t.Fatalf("updated head was lost: %s", got)
	}
	stash := fixtureGit(t, "-C", c.Harness, "rev-parse", "refs/stash")
	pin := fixtureGit(t, "-C", c.Harness, "rev-parse", "refs/wtc-catch-up/main/agent-harness")
	if stash != pin {
		t.Fatalf("recovery stash was not pinned: %s != %s", stash, pin)
	}
	found := false
	for _, row := range report.Outcomes {
		if row.Kind == "repo" && row.Outcome == "needs-owner" && strings.Contains(row.Reason, "stash restore conflicted") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing stash recovery row: %+v", report.Outcomes)
	}
}

func TestCatchUpSkipsSecretsHookForUnmanagedSibling(t *testing.T) {
	c := newWorkspaceFixture(t)
	owner := filepath.Join(c.Workspace, ".bare", "widget.git")
	fixtureGit(t, "--git-dir="+owner, "worktree", "add", "--detach", filepath.Join(c.Collection, "widget"), "origin/main")
	fixtureGit(t, "--git-dir="+owner, "worktree", "add", "--detach", filepath.Join(c.Collection, "ext.thing"), "origin/main")
	calls := filepath.Join(c.Workspace, "secrets-calls")
	t.Setenv("MOCK_SECRETS_LOG", calls)
	fixtureFile(t, filepath.Join(c.Harness, "tools", "link-secrets.sh"), "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$MOCK_SECRETS_LOG\"\n", 0755)
	report, err := c.CatchUp(CatchUpOptions{Repos: []string{"widget", "ext.thing"}, NoSkills: true, NoMCP: true, NoEnv: true})
	if err != nil || report.ExitStatus != 0 {
		t.Fatalf("catch-up with an unmanaged sibling failed: %+v %v", report, err)
	}
	logged, _ := os.ReadFile(calls)
	if !strings.Contains(string(logged), "--repo widget") || strings.Contains(string(logged), "ext.thing") {
		t.Fatalf("secrets hook calls: %q", logged)
	}
	seen := false
	for _, row := range report.Outcomes {
		if row.Kind == "hook" && row.Repo == "secrets:ext.thing" {
			if row.Outcome != "skipped" || !strings.Contains(row.Reason, "unmanaged") {
				t.Fatalf("unmanaged sibling secrets row: %+v", row)
			}
			seen = true
		}
	}
	if !seen {
		t.Fatalf("missing skipped secrets row: %+v", report.Outcomes)
	}
}

func TestCatchUpRunsSecretsHookForSiblingRegisteredByHarnessUpdate(t *testing.T) {
	c := newWorkspaceFixture(t)
	created, err := c.NewCollection(NewOptions{Slug: "other", Repos: []string{"widget"}})
	if err != nil {
		t.Fatal(err)
	}
	// gadget is a sibling the registry does not know yet; the harness update
	// that catch-up applies first registers it and ships the secrets hook.
	widgetOwner := filepath.Join(c.Workspace, ".bare", "widget.git")
	fixtureGit(t, "--git-dir="+widgetOwner, "worktree", "add", "--detach", filepath.Join(created.Collection, "gadget"), "origin/main")
	harnessSource := filepath.Join(c.Workspace, "source-agent-harness")
	registryPath := filepath.Join(harnessSource, ".harness-repos.yml")
	registry, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	fixtureFile(t, registryPath, string(registry)+"  - name: gadget\n    remote: "+filepath.Join(c.Workspace, "source-widget")+"\n    default_ref: origin/main\n    port_offset: 2\n", 0644)
	fixtureFile(t, filepath.Join(harnessSource, "tools", "link-secrets.sh"), "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$MOCK_SECRETS_LOG\"\n", 0755)
	fixtureGit(t, "-C", harnessSource, "add", "-A")
	fixtureGit(t, "-C", harnessSource, "-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "register gadget")
	calls := filepath.Join(c.Workspace, "secrets-calls")
	t.Setenv("MOCK_SECRETS_LOG", calls)
	report, err := c.CatchUp(CatchUpOptions{Collections: []string{"other"}, Repos: []string{"harness", "gadget"}, NoSkills: true, NoMCP: true, NoEnv: true})
	if err != nil || report.ExitStatus != 0 {
		t.Fatalf("catch-up across a registering harness update failed: %+v %v", report, err)
	}
	logged, _ := os.ReadFile(calls)
	if !strings.Contains(string(logged), "--repo gadget") {
		t.Fatalf("newly registered sibling did not receive the secrets hook: %q", logged)
	}
	for _, row := range report.Outcomes {
		if row.Kind == "hook" && row.Repo == "secrets:gadget" && row.Outcome != "ok" {
			t.Fatalf("stale membership decided the hook: %+v", row)
		}
	}
}
