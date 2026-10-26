package wtc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompatibilityRangeAndRewrite(t *testing.T) {
	for version, want := range map[string]string{"0.1.54": ">=0.1.54,<0.2", "v1.9.0": ">=1.9.0,<1.10", "2.0.3": ">=2.0.3,<2.1"} {
		if got, err := CompatibilityRange(version); err != nil || got != want {
			t.Fatalf("%s: %q %v", version, got, err)
		}
	}
	if _, err := CompatibilityRange("0.1"); err == nil {
		t.Fatal("partial version accepted")
	}
	config := "[other]\nrequires = \"keep\"\n\n[compatibility] # pin range\nrequires = \">=0.1.41,<0.2\" # moved by wtc upgrade\r\n\n[status]\nreturn_merged = false\n"
	got, err := replaceCompatibility(config, ">=0.1.54,<0.2")
	if err != nil {
		t.Fatal(err)
	}
	want := "[other]\nrequires = \"keep\"\n\n[compatibility] # pin range\nrequires = \">=0.1.54,<0.2\" # moved by wtc upgrade\r\n\n[status]\nreturn_merged = false\n"
	if got != want {
		t.Fatalf("rewrite changed more than the requires line:\n%q", got)
	}
	if _, err := replaceCompatibility("[other]\nrequires = \"x\"\n", ">=0.1.54,<0.2"); err == nil {
		t.Fatal("missing [compatibility] requires was not reported")
	}
}

// upgradeFixture commits a wtc.toml into the fixture harness and puts a fake
// gh on PATH that publishes 0.1.9 to 0.1.12 (plus a draft 0.1.13).
func upgradeFixture(t *testing.T) (*Context, string) {
	t.Helper()
	c := newWorkspaceFixture(t)
	fixtureFile(t, filepath.Join(c.Harness, "wtc.toml"), "[compatibility]\nrequires = \">=0.1.9,<0.2\"\n", 0644)
	fixtureGit(t, "-C", c.Harness, "add", "wtc.toml")
	fixtureGit(t, "-C", c.Harness, "-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "config")
	bin := filepath.Join(c.Workspace, "bin")
	fixtureFile(t, filepath.Join(bin, "gh"), `#!/bin/sh
case "$*" in
  *releases/latest*) echo v0.1.12 ;;
  *releases/tags/v0.1.1[0-2]*) echo ok ;;
  *releases/tags/*) exit 1 ;;
  *releases\?per_page=100*)
    for v in 9 10 11 12; do printf '{"tag_name":"v0.1.%s","body":"# wtc 0.1.%s\\n","draft":false,"prerelease":false}\n' $v $v; done
    printf '%s\n' '{"tag_name":"v0.1.13","body":"draft","draft":true,"prerelease":false}' ;;
  *) exit 2 ;;
esac
`, 0755)
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	return c, bin
}

func TestUpgradeBumpsPinAndRangeWithPublishedNotes(t *testing.T) {
	c, _ := upgradeFixture(t)
	pin := filepath.Join(c.Harness, ".wtc-cli-version")
	config := filepath.Join(c.Harness, "wtc.toml")

	dry, err := c.Upgrade(UpgradeOptions{DryRun: true})
	if err != nil || dry.To != "0.1.12" || dry.Requires != ">=0.1.12,<0.2" || len(dry.Notes) != 3 || dry.Notes[0].Version != "0.1.10" {
		t.Fatalf("dry run: %+v %v", dry, err)
	}
	if data, _ := os.ReadFile(pin); string(data) != "0.1.9\n" {
		t.Fatal("dry run wrote the pin")
	}

	if _, err := c.Upgrade(UpgradeOptions{Version: "0.1.99"}); err == nil || !strings.Contains(err.Error(), "not a published release") {
		t.Fatalf("unpublished version accepted: %v", err)
	}
	if data, _ := os.ReadFile(pin); string(data) != "0.1.9\n" {
		t.Fatal("unpublished version wrote the pin")
	}

	result, err := c.Upgrade(UpgradeOptions{Version: "v0.1.11"})
	if err != nil {
		t.Fatal(err)
	}
	if result.From != "0.1.9" || result.To != "0.1.11" || result.NotesSource != "GitHub releases" ||
		len(result.Notes) != 2 || result.Notes[1].Content != "# wtc 0.1.11\n" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if data, _ := os.ReadFile(pin); string(data) != "0.1.11\n" {
		t.Fatalf("pin not written: %q", data)
	}
	if data, _ := os.ReadFile(config); !strings.Contains(string(data), `requires = ">=0.1.11,<0.2"`) {
		t.Fatalf("range not written: %s", data)
	}
	if data, _ := os.ReadFile(filepath.Join(c.Collection, "mise.toml")); !strings.Contains(string(data), `"`+cliMiseTool+`" = "0.1.11"`) {
		t.Fatalf("collection mise config not refreshed: %s", data)
	}

	// The bump is left uncommitted for review, so a second run refuses it.
	if _, err := c.Upgrade(UpgradeOptions{}); err == nil || !strings.Contains(err.Error(), "uncommitted") {
		t.Fatalf("dirty pin not refused: %v", err)
	}
	fixtureGit(t, "-C", c.Harness, "checkout", "--", ".wtc-cli-version", "wtc.toml")
	if _, err := c.Upgrade(UpgradeOptions{Version: "0.1.9"}); err == nil || !strings.Contains(err.Error(), "not newer") {
		t.Fatalf("same version accepted: %v", err)
	}
}

func TestUpgradeReadsTargetBinaryNotesThroughMise(t *testing.T) {
	c, bin := upgradeFixture(t)
	log := filepath.Join(c.Workspace, "mise.log")
	t.Setenv("MOCK_MISE_LOG", log)
	fixtureFile(t, filepath.Join(bin, "mise"), `#!/bin/sh
printf '%s|%s\n' "$*" "$WTC_DEV_BUILD" >> "$MOCK_MISE_LOG"
case "$1" in
  exec) printf '%s\n' '{"ok":true,"data":[{"version":"0.1.10","content":"corrected 0.1.10\n"},{"version":"0.1.12","content":"0.1.12\n"}]}' ;;
esac
`, 0755)
	result, err := c.Upgrade(UpgradeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if result.NotesSource != "wtc 0.1.12 release-notes" || len(result.Notes) != 2 || result.Notes[0].Content != "corrected 0.1.10\n" {
		t.Fatalf("target binary notes not used: %+v", result)
	}
	data, _ := os.ReadFile(log)
	for _, want := range []string{"install|", "exec " + cliMiseTool + "@0.1.12 -- wtc release-notes --since 0.1.9 --json|off"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("mise not called with %q:\n%s", want, data)
		}
	}
}

func TestUpgradeRefusesAdHocCollection(t *testing.T) {
	c := &Context{AdHoc: true}
	if _, err := c.Upgrade(UpgradeOptions{}); err == nil || !strings.Contains(err.Error(), "harness collection") {
		t.Fatalf("ad-hoc collection not refused: %v", err)
	}
}

func TestCatchUpIdleOnlyLeavesBusyAgentsAndInstallsTools(t *testing.T) {
	c := newWorkspaceFixture(t)
	for _, slug := range []string{"busy", "quiet"} {
		if _, err := c.NewCollection(NewOptions{Slug: slug}); err != nil {
			t.Fatal(err)
		}
	}
	bin := filepath.Join(c.Workspace, "bin")
	log := filepath.Join(c.Workspace, "tools.log")
	fixtureFile(t, filepath.Join(bin, "herdr"), `#!/bin/sh
shift 2
case "$1 $2" in
  'workspace list')
    if [ -n "$MOCK_HERDR_DOWN" ]; then printf '%s\n' '{"error":{"code":"server_not_running"}}'; exit 1; fi
    printf '%s\n' '{"result":{"workspaces":[{"label":"main","workspace_id":"w1"},{"label":"busy","workspace_id":"w2"},{"label":"quiet","workspace_id":"w3"}]}}' ;;
  'pane list')
    case "$4" in
      w2) printf '%s\n' '{"result":{"panes":[{"label":"agent","pane_id":"w2:p1","agent":"claude","agent_status":"working"}]}}' ;;
      *) printf '%s\n' '{"result":{"panes":[{"label":"agent","pane_id":"x:p1","agent":"claude","agent_status":"idle"}]}}' ;;
    esac ;;
esac
`, 0755)
	fixtureFile(t, filepath.Join(bin, "mise"), "#!/bin/sh\nprintf '%s %s\\n' \"$1\" \"$PWD\" >> \""+log+"\"\n", 0755)
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	t.Setenv("HERDR_SESSION", "fixture")
	opt := CatchUpOptions{All: true, Repos: []string{"harness"}, CleanOnly: true, IdleOnly: true, InstallTools: true, NoSkills: true, NoMCP: true, NoSecrets: true}
	report, _ := c.CatchUp(opt)
	outcomes := map[string]string{}
	for _, row := range report.Outcomes {
		if row.Kind == "repo" {
			outcomes[row.Collection] = row.Outcome + ": " + row.Reason
		}
		if row.Kind == "hook" && row.Repo == "tools" && row.Outcome != "ok" {
			t.Fatalf("tools hook failed: %+v", row)
		}
	}
	if !strings.HasPrefix(outcomes["busy"], "needs-owner: agent working") {
		t.Fatalf("busy agent's collection was not left alone: %v", outcomes)
	}
	for _, name := range []string{"main", "quiet"} {
		if strings.HasPrefix(outcomes[name], "needs-owner") || outcomes[name] == "" {
			t.Fatalf("%s should have been caught up: %v", name, outcomes)
		}
	}
	data, _ := os.ReadFile(log)
	if !strings.Contains(string(data), "install "+filepath.Join(c.Workspace, "quiet")) || strings.Contains(string(data), "install "+filepath.Join(c.Workspace, "busy")) {
		t.Fatalf("mise install ran in the wrong collections:\n%s", data)
	}

	// Without a running herdr session no agent can be busy.
	t.Setenv("MOCK_HERDR_DOWN", "1")
	report, _ = c.CatchUp(CatchUpOptions{Collections: []string{"busy"}, Repos: []string{"harness"}, IdleOnly: true, DryRun: true, NoSkills: true, NoMCP: true, NoSecrets: true, NoEnv: true})
	for _, row := range report.Outcomes {
		if row.Kind == "repo" && row.Outcome == "needs-owner" {
			t.Fatalf("stopped herdr session blocked the sweep: %+v", row)
		}
	}
}
