//go:build openclaw_integration

package openclaw

// A conversation whose commands run in an OpenClaw sandbox: the gateway runs
// each exec in a Docker container of its own (testdata/sandbox.Dockerfile),
// on the Docker daemon the tests use, and the session's managed worktree
// reaches that container as a private checkout at /workspace. Publication is
// still the gateway's: github_publish needs no credential in the sandbox.
//
//	WTC_OPENCLAW_GITHUB_REPO=<owner>/<throwaway-repo> GH_TOKEN=$(gh auth token) \
//	  go test -tags openclaw_integration ./internal/openclaw/ -run Sandbox -v
//
// The gateway's state is a directory on the Docker host under sandboxRoot,
// removed with the sandbox containers when the test ends.

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"
)

// lastTurn is what the scripted model was last asked: the tools it was
// offered and the tool results so far, oldest first.
func (g *gateway) lastTurn() (tools []string, results []string) {
	g.t.Helper()
	out := g.sh(`node -e '
const fs = require("fs");
let last = null;
for (const line of fs.readFileSync("/tmp/stub/requests.jsonl", "utf8").split("\n").filter(Boolean)) {
  const r = JSON.parse(line); if (r.served) last = r;
}
const text = (c) => typeof c === "string" ? c : Array.isArray(c) ? c.map((p) => p.text ?? "").join("") : JSON.stringify(c ?? "");
console.log(JSON.stringify({
  tools: (last?.body.tools || []).map((t) => t.function?.name),
  results: (last?.body.messages || []).filter((m) => m.role === "tool").map((m) => text(m.content)),
}));'`)
	var seen struct{ Tools, Results []string }
	if err := json.Unmarshal([]byte(out), &seen); err != nil {
		g.t.Fatalf("model log: %v\n%s", err, g.redact(out))
	}
	return seen.Tools, seen.Results
}

// fields reads the key=value lines a probe command printed.
func fields(output string) map[string]string {
	f := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		if k, v, ok := strings.Cut(line, "="); ok && !strings.ContainsAny(k, " {\"") {
			f[k] = v
		}
	}
	return f
}

func TestSandboxedConversation(t *testing.T) {
	hub := gitHubTarget(t)
	g := startGateway(t, gatewayOptions{repos: hub.clone(), secrets: []string{"GH_TOKEN=" + hub.token}, stubModel: true, sandbox: true})
	worktree := fmt.Sprintf("wtc-sbx-%d", time.Now().Unix())
	branch := Branch(worktree)
	owner := strings.Split(hub.repo, "/")[0]
	openPRs := func() []pullRequest {
		var prs []pullRequest
		hub.do("GET", "/pulls?state=open&head="+owner+":"+branch, nil, &prs)
		return prs
	}
	t.Cleanup(func() {
		for _, pr := range openPRs() {
			hub.do("PATCH", fmt.Sprintf("/pulls/%d", pr.Number), map[string]string{"state": "closed"}, nil)
		}
		hub.do("DELETE", "/git/refs/heads/"+branch, nil, nil)
	})
	if r := g.call("projects.register", map[string]any{"path": "/home/node/repos/target", "name": "target"}); r.str("id") != "target" {
		t.Fatalf("projects.register: %v", r)
	}
	defaultBranch := g.sh("git -C /home/node/repos/target branch --show-current")
	parent := g.call("sessions.create", map[string]any{"agentId": "dev", "label": worktree}).str("key")
	add, err := AddRepo("target", "target", "", worktree, "", "Work on target")
	if err != nil {
		t.Fatal(err)
	}
	created := g.createFromIntent(parent, add)
	child, path := created.str("key"), created.str("worktree", "path")
	if child == "" || path == "" {
		t.Fatalf("no child session: %v", created)
	}
	// A sandboxed turn's working directory is the container's, and the
	// scripted model tells turns apart by it.
	const workdir = "/workspace"

	t.Run("the child's commands run in a sandbox container that holds its worktree", func(t *testing.T) {
		g.turn(child, workdir, "Look around.",
			step{"tool": "exec", "arguments": map[string]any{"command": `echo "hostname=$(hostname)"; echo "pwd=$(pwd)"
[ -e /.dockerenv ] && echo dockerenv=yes
echo "openclaw_shell=${OPENCLAW_SHELL:-}"; echo "gh_token=${GH_TOKEN:+present}"; echo "github_token=${GITHUB_TOKEN:+present}"
echo "host=$(wtc host)"; echo "branch=$(git branch --show-current)"
echo "origin=$(git remote get-url origin 2>&1 | head -1)"; echo "refs=$(git for-each-ref --format='%(refname)' | tr '\n' ' ')"
echo "mount=$(awk '$2 == "/workspace" { print $4 }' /proc/self/mounts | cut -d, -f1)"
echo TOKEN=x > .env; wtc publish > /tmp/env.out 2> /tmp/env.err; echo "env_publish=$? $(head -c 300 /tmp/env.err)"; wc -c < /tmp/env.out | sed 's/^/env_plan_bytes=/'; rm .env`}},
			step{"text": "Looked around."},
		)
		tools, results := g.lastTurn()
		if len(results) == 0 {
			t.Fatalf("no tool result:\n%s", g.modelLog())
		}
		seen := fields(results[len(results)-1])
		t.Logf("in the sandbox: %v", seen)

		// The command ran in a container the gateway started for this session,
		// not in the gateway's own.
		ids, err := exec.Command("docker", "ps", "-q", "--no-trunc", "--filter", "name=^"+g.sandboxPrefix).Output()
		if err != nil {
			t.Fatal(err)
		}
		host := seen["hostname"]
		inSandbox := slices.ContainsFunc(strings.Fields(string(ids)), func(id string) bool { return host != "" && strings.HasPrefix(id, host) })
		if !inSandbox || host == g.sh("hostname") {
			t.Errorf("the command ran on host %q; the sandbox containers are %q", host, strings.Fields(string(ids)))
		}
		if seen["dockerenv"] != "yes" {
			t.Errorf("no /.dockerenv in the command's container")
		}
		if seen["pwd"] != workdir || seen["mount"] != "rw" || seen["branch"] != branch {
			t.Errorf("the worktree in the sandbox: pwd %q, /workspace mounted %q, branch %q", seen["pwd"], seen["mount"], seen["branch"])
		}
		if seen["openclaw_shell"] != "exec" || seen["host"] != string(HostOpenClaw) {
			t.Errorf("OPENCLAW_SHELL=%q, wtc host %q", seen["openclaw_shell"], seen["host"])
		}
		// Without tools.github.allowInSandbox the sandbox has no GitHub token.
		if seen["gh_token"] != "" || seen["github_token"] != "" {
			t.Errorf("a GitHub token reached the sandbox: GH_TOKEN %q, GITHUB_TOKEN %q", seen["gh_token"], seen["github_token"])
		}
		// The private checkout carries the session's branch and nothing of
		// the repository's configuration: no origin, no other refs.
		if strings.TrimSpace(seen["refs"]) != "refs/heads/"+branch {
			t.Errorf("refs in the private checkout: %q", seen["refs"])
		}
		if !strings.HasPrefix(seen["env_publish"], "1 ") || !strings.Contains(seen["env_publish"], ".env") || seen["env_plan_bytes"] != "0" {
			t.Errorf("wtc publish with a .env in the sandbox: %q, %s bytes of plan", seen["env_publish"], seen["env_plan_bytes"])
		}
		// OpenClaw's default sandbox tool policy leaves github_publish out.
		if slices.Contains(tools, "github_publish") {
			t.Errorf("github_publish is offered in the sandbox by default: %v", tools)
		}
	})

	// github_publish runs on the gateway, so allowing it in the sandbox gives
	// the sandbox no credential; tools.github.allowInSandbox would.
	g.sh(gatewayEnv + `node dist/index.js config set tools.sandbox.tools.alsoAllow '["github_publish"]' --strict-json >/dev/null`)
	for deadline := time.Now().Add(30 * time.Second); ; time.Sleep(time.Second) {
		if g.sh("grep -c 'hot reload applied (tools.sandbox)' /tmp/gateway.log || true") != "0" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the gateway did not reload the sandbox tool policy:\n%s", g.redact(g.sh("grep reload /tmp/gateway.log | tail -5 | cut -c1-300")))
		}
	}

	t.Run("the sandboxed agent publishes with wtc publish and github_publish", func(t *testing.T) {
		g.turn(child, workdir, "Add a note and publish it.",
			step{"tool": "exec", "arguments": map[string]any{"command": "echo sandboxed > " + worktree + ".md && " +
				`wtc publish --title "wtc sandbox ` + worktree + `" --body "Opened from a sandboxed agent turn by the wtc integration test." 2> /tmp/publish.err; cat /tmp/publish.err`}},
			step{"carryOut": 0},
			step{"text": "Published."},
		)
		tools, results := g.lastTurn()
		if !slices.Contains(tools, "github_publish") || len(results) < 2 {
			t.Fatalf("tools %v, %d tool results\n%s", tools, len(results), g.modelLog())
		}
		var plan Plan
		output := results[len(results)-2]
		if start := strings.Index(output, "{"); start < 0 || json.NewDecoder(strings.NewReader(output[start:])).Decode(&plan) != nil {
			t.Fatalf("no plan from wtc publish in the sandbox:\n%s", g.redact(output))
		}
		// Without an origin wtc leaves the repository to the gateway, and the
		// skills OpenClaw mirrors into the copy are not the agent's work.
		if len(plan.Intents) != 1 || plan.Intents[0].Tool != ToolGitHubPublish || plan.Intents[0].Forge != ForgeUnknown {
			t.Fatalf("plan %+v", plan)
		}
		if strings.Join(plan.Pending, " ") != worktree+".md" {
			t.Errorf("pending %v, want only %s.md", plan.Pending, worktree)
		}
		t.Logf("github_publish answered: %s", g.redact(results[len(results)-1]))

		var pr pullRequest
		for deadline := time.Now().Add(90 * time.Second); pr.Number == 0 && time.Now().Before(deadline); time.Sleep(3 * time.Second) {
			if prs := openPRs(); len(prs) == 1 {
				pr = prs[0]
			}
		}
		if pr.Number == 0 {
			t.Fatalf("no publication arrived on %s:\n%s", branch, g.redact(g.sh("tail -4 /tmp/gateway.log | cut -c1-500")))
		}
		if !pr.Draft || pr.Title != "wtc sandbox "+worktree || pr.Base.Ref != defaultBranch {
			t.Errorf("pull request %+v", pr)
		}
		var files []struct {
			Filename string `json:"filename"`
		}
		hub.do("GET", fmt.Sprintf("/pulls/%d/files", pr.Number), nil, &files)
		if len(files) != 1 || files[0].Filename != worktree+".md" {
			t.Errorf("pull request files %+v, want only %s.md", files, worktree)
		}
		// The worktree on the gateway is what was published.
		if got := g.sh("git -C " + path + " rev-parse HEAD"); got != pr.Head.SHA {
			t.Errorf("pull request head %s, worktree %s", pr.Head.SHA, got)
		}
		if got := g.sh("cat " + path + "/" + worktree + ".md"); got != "sandboxed" {
			t.Errorf("the worktree's %s.md holds %q", worktree, got)
		}
	})
}
