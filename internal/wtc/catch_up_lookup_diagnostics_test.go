package wtc

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCatchUpLookupDiagnostics(t *testing.T) {
	for _, tc := range []struct{ name, remote, record, cli, script, want string }{
		{"github-auth", "git@github.com:example/agent-harness.git", "agent-harness 7 topic - fixture\n", "gh", "echo 'HTTP 401 Authorization: Bearer secret-value' >&2; exit 1", "github.com/example/agent-harness PR #7: gh pr view failed: authentication or authorization failed"},
		{"github-transport", "git@github.com:example/agent-harness.git", "", "gh", "echo 'dial tcp private.invalid:443: connection refused secret-value' >&2; exit 1", "github.com/example/agent-harness branch \"topic\": gh pr list failed: transport failure"},
		{"bitbucket-auth", "git@bitbucket.org:example/agent-harness.git", "agent-harness 7 topic - fixture\n", "bb", "echo 'HTTP 403 secret-value' >&2; exit 1", "bitbucket.org/example/agent-harness PR #7: bb pr view failed: authentication or authorization failed"},
		{"bitbucket-no-id", "git@bitbucket.org:example/agent-harness.git", "", "bb", "exit 99", "bitbucket.org/example/agent-harness branch \"topic\": branch PR lookup unsupported; enlist the PR number"},
		{"unsupported", "https://user:secret-value@forge.invalid/example/repo.git?token=secret-value", "", "gh", "exit 99", "identify origin forge: unsupported or invalid remote"},
		{"invalid-json", "git@github.com:example/agent-harness.git", "agent-harness 7 topic - fixture\n", "gh", "echo 'secret-value not-json'", "gh pr view failed: invalid JSON response"},
		{"invalid-state", "git@github.com:example/agent-harness.git", "agent-harness 7 topic - fixture\n", "gh", "echo '{\"state\":\"secret-value\"}'", "gh pr view failed: unrecognized PR state"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newWorkspaceFixture(t)
			fixtureGit(t, "-C", c.Harness, "switch", "-qc", "topic")
			owner := filepath.Join(c.Workspace, ".bare", "agent-harness.git")
			fixtureGit(t, "--git-dir="+owner, "remote", "set-url", "origin", tc.remote)
			fixtureFile(t, filepath.Join(c.Collection, ".wtc-prs"), tc.record, 0644)
			bin := filepath.Join(c.Workspace, "bin")
			fixtureFile(t, filepath.Join(bin, tc.cli), "#!/bin/sh\n"+tc.script+"\n", 0755)
			t.Setenv("PATH", bin+":/usr/bin:/bin")
			before := fixtureGit(t, "-C", c.Harness, "rev-parse", "HEAD")
			report, err := c.CatchUp(CatchUpOptions{Repos: []string{"harness"}, DryRun: true, NoSkills: true, NoMCP: true, NoEnv: true, NoSecrets: true})
			if err == nil || report.ExitStatus != 1 {
				t.Fatalf("lookup failure hidden: %+v %v", report, err)
			}
			found := false
			for _, row := range report.Outcomes {
				if row.Kind != "repo" {
					continue
				}
				found = true
				if row.Outcome != "needs-owner" || !strings.Contains(row.Reason, tc.want) || !strings.Contains(row.Reason, "branch left untouched") || row.NextAction == "" {
					t.Errorf("missing diagnostic: %+v; want %q", row, tc.want)
				}
				var shape map[string]json.RawMessage
				rowJSON, _ := json.Marshal(row)
				if json.Unmarshal(rowJSON, &shape) != nil {
					t.Fatal("invalid row JSON")
				}
				expected := []string{"kind", "collection", "repo", "outcome", "reason", "source_sha", "target_sha", "target_ref", "result_sha", "next_actor", "handoff", "collection_path", "next_action"}
				if report.SchemaVersion != 1 || len(shape) != len(expected) {
					t.Fatalf("failure JSON shape changed: %s", rowJSON)
				}
				for _, key := range expected {
					if _, ok := shape[key]; !ok {
						t.Errorf("missing failure field %s", key)
					}
				}
				if row.SourceSHA == nil || row.ResultSHA == nil || *row.SourceSHA != before || *row.ResultSHA != before {
					t.Errorf("failure SHA contract changed: %+v", row)
				}
			}
			if !found {
				t.Fatal("missing repo outcome")
			}
			data, _ := json.Marshal(report)
			for _, forbidden := range []string{"secret-value", "private.invalid", "user:", "Bearer"} {
				if strings.Contains(string(data), forbidden) || strings.Contains(report.Markdown(), forbidden) {
					t.Errorf("unsafe diagnostic contains %q", forbidden)
				}
			}
			if after := fixtureGit(t, "-C", c.Harness, "rev-parse", "HEAD"); after != before {
				t.Fatal("lookup failure moved branch")
			}
			if status := fixtureGit(t, "-C", c.Harness, "status", "--porcelain"); status != "" {
				t.Fatalf("lookup failure changed worktree: %s", status)
			}
			if stash := fixtureGit(t, "-C", c.Harness, "stash", "list"); stash != "" {
				t.Fatal("lookup failure stashed work")
			}
		})
	}
}

func TestCatchUpLookupCauseDoesNotEchoUntrustedOutput(t *testing.T) {
	for _, tc := range []struct{ stderr, want string }{
		{"HTTP 401 token=secret-value\x1b[31m", "authentication or authorization failed"},
		{"dial tcp private.invalid:443: secret-value", "transport failure"},
		{strings.Repeat("secret-value", 10000), "forge CLI exited with status 17"},
	} {
		err := &exec.ExitError{ProcessState: lookupExitState(t), Stderr: []byte(tc.stderr)}
		got := catchUpLookupCause(err)
		if !strings.HasPrefix(got, tc.want) || strings.Contains(got, "secret-value") || strings.ContainsAny(got, "\x1b\n\r") {
			t.Fatalf("unsafe or unhelpful cause: %q", got)
		}
	}
}

func lookupExitState(t *testing.T) *os.ProcessState {
	t.Helper()
	cmd := exec.Command("sh", "-c", "exit 17")
	_ = cmd.Run()
	return cmd.ProcessState
}

func TestCatchUpLookupMissingCLIAndTimeout(t *testing.T) {
	_, err := catchUpJSON("wtc-synthetic-missing-forge-cli")
	if err == nil || !strings.Contains(catchUpLookupCause(err), "CLI not installed") {
		t.Fatalf("missing CLI: %v", err)
	}
	old := catchUpForgeTimeout
	catchUpForgeTimeout = 20 * time.Millisecond
	defer func() { catchUpForgeTimeout = old }()
	_, err = catchUpJSON("sh", "-c", "sleep 2")
	if err == nil || !strings.Contains(catchUpLookupCause(err), "lookup timed out") {
		t.Fatalf("timeout: %v", err)
	}
}

func TestCatchUpLookupLabelsEscapeAndBound(t *testing.T) {
	got := catchUpLookupLabel("topic\x1b[31m\n" + strings.Repeat("界", 200))
	if len(got) > 1000 || strings.ContainsAny(got, "\x1b\n\r") || !strings.Contains(got, `\x1b`) || !strings.Contains(got, "...") {
		t.Fatalf("unsafe label: %q", got)
	}
}
