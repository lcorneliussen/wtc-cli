package main

import (
	"strings"
	"testing"
)

// The gateway's wtc projects, each with the repositories its registry lists,
// so that the agent can match what the person asks for.
func TestProjects(t *testing.T) {
	_, _ = harnessGateway(t)
	withGatewayConfig(t, `{"harnesses": [{"name": "platform", "remote": "https://github.com/example-org/platform-harness"}, {"name": "ops", "remote": "git@github.com:example-org/ops-harness.git"}, {"name": "gone", "remote": "https://github.com/example-org/gone-harness"}], "defaultHarness": "ops"}`)
	projects, err := gatewayProjects()
	if err != nil || len(projects) != 3 {
		t.Fatalf("%+v %v", projects, err)
	}
	platform, ops, gone := projects[0], projects[1], projects[2]
	if platform.Name != "platform" || platform.Default || len(platform.Repos) != 2 || platform.Repos[0].Name != "api" || platform.Repos[1].Name != "web" {
		t.Errorf("platform: %+v", platform)
	}
	if !ops.Default || len(ops.Repos) != 1 || ops.Repos[0].Name != "infra" {
		t.Errorf("ops: %+v", ops)
	}
	if !strings.Contains(gone.Error, "not on the gateway yet") {
		t.Errorf("a project whose harness the gateway lacks: %+v", gone)
	}
	summary := projectsSummary(projects)
	for _, want := range []string{"platform: harness https://github.com/example-org/platform-harness @ main", "ops (default)", "  api  git@github.com:example-org/api.git", "--project none"} {
		if !strings.Contains(summary, want) {
			t.Errorf("summary lacks %q:\n%s", want, summary)
		}
	}
	withGatewayConfig(t, "")
	if projects, err := gatewayProjects(); err != nil || len(projects) != 0 || !strings.Contains(projectsSummary(projects), "no wtc projects") {
		t.Errorf("none: %+v %v", projects, err)
	}
}

// --project is --harness under its new name.
func TestAddRepoTakesProject(t *testing.T) {
	_, _ = harnessGateway(t)
	withGatewayConfig(t, `{"harnesses": [{"name": "platform", "remote": "https://github.com/example-org/platform-harness"}, {"name": "ops", "remote": "git@github.com:example-org/ops-harness.git"}]}`)
	if intents, err := addRepoPlan(t, "api", "--worktree", "40-x", "--project", "platform"); err != nil || !strings.HasPrefix(summary(intents), "platform-harness@") {
		t.Errorf("--project platform: %s %v", summary(intents), err)
	}
	if intents, err := addRepoPlan(t, "api", "--worktree", "41-x", "--project", "none"); err != nil || summary(intents) != "api@api:" {
		t.Errorf("--project none: %s %v", summary(intents), err)
	}
}

// A repository another wtc project lists belongs in a conversation of its
// own in that project: add-repo suggests that beside the usual choices.
func TestAddRepoSuggestsTheProjectThatListsIt(t *testing.T) {
	_, _ = harnessGateway(t)
	withGatewayConfig(t, `{"harnesses": [{"name": "platform", "remote": "https://github.com/example-org/platform-harness"}, {"name": "ops", "remote": "git@github.com:example-org/ops-harness.git"}]}`)
	_, err := addRepoPlan(t, "api", "infra", "--worktree", "50-x", "--project", "platform")
	if err == nil || !strings.Contains(err.Error(), "  4. usually the right one for work on that project: in a new conversation for wtc project ops, which lists infra: the person starts it, and there wtc add-repo infra --worktree <name>") {
		t.Fatalf("%v", err)
	}
	// Listed nowhere else: the three choices only.
	_, err = addRepoPlan(t, "tools", "--worktree", "51-x", "--project", "platform")
	if err == nil || strings.Contains(err.Error(), "  4. ") || !strings.Contains(err.Error(), "  3. ") {
		t.Fatalf("listed nowhere else: %v", err)
	}
}
