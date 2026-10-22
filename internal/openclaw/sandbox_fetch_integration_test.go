//go:build openclaw_integration

package openclaw

// What a read-only fetch credential for sandboxed sessions (#113) relies on:
// that a sandbox on a network it shares with the gateway can reach a broker
// there, that the broker can tell which sandbox container asks from the
// connection alone, and what becomes of commits made in a sandbox's private
// checkout.
//
//	go test -tags openclaw_integration ./internal/openclaw/ -run SandboxBroker -v

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestSandboxBrokerAndCommits(t *testing.T) {
	g := startGateway(t, gatewayOptions{repos: syntheticRepos, stubModel: true, sandbox: true, sandboxNetwork: true})
	if r := g.call("projects.register", map[string]any{"path": "/home/node/repos/api", "name": "api"}); r.str("id") != "api" {
		t.Fatalf("projects.register: %v", r)
	}
	worktree := fmt.Sprintf("wtc-brk-%d", time.Now().Unix())
	branch := Branch(worktree)
	parent := g.call("sessions.create", map[string]any{"agentId": "dev", "label": worktree}).str("key")
	add, err := AddRepo("api", "api", "", worktree, "", "Work on api")
	if err != nil {
		t.Fatal(err)
	}
	created := g.createFromIntent(parent, add)
	child, path := created.str("key"), created.str("worktree", "path")
	if child == "" || path == "" {
		t.Fatalf("no child session: %v", created)
	}
	const workdir = "/workspace"

	// A stand-in for the broker: it answers with the address it sees.
	g.sh(`nohup node -e 'require("http").createServer((q, s) => s.end(q.socket.remoteAddress.replace(/^::ffff:/, ""))).listen(18790, "0.0.0.0")' > /tmp/broker.log 2>&1 &
sleep 1`)
	ip := func(container string) string {
		out, err := exec.Command("docker", "inspect", "--format", `{{(index .NetworkSettings.Networks "`+g.network+`").IPAddress}}`, container).Output()
		if err != nil {
			t.Fatalf("docker inspect %s: %v", container, err)
		}
		return strings.TrimSpace(string(out))
	}
	gatewayIP := ip(g.container)

	t.Run("a sandbox reaches the gateway, which sees the sandbox container's address", func(t *testing.T) {
		g.turn(child, workdir, "Ask the broker.",
			step{"tool": "exec", "arguments": map[string]any{"command": `echo "hostname=$(hostname)"; echo "peer=$(curl -s --max-time 5 http://` + gatewayIP + `:18790/)"; echo "internet=$(curl -s -o /dev/null -w %{http_code} --max-time 5 https://github.com/ || true)"`}},
			step{"text": "Asked."},
		)
		_, results := g.lastTurn()
		if len(results) == 0 {
			t.Fatalf("no tool result:\n%s", g.modelLog())
		}
		seen := fields(results[len(results)-1])
		t.Logf("in the sandbox: %v", seen)
		if seen["peer"] == "" {
			t.Fatalf("the sandbox did not reach the gateway at %s:18790", gatewayIP)
		}
		ids, err := exec.Command("docker", "ps", "-q", "--no-trunc", "--filter", "name=^"+g.sandboxPrefix).Output()
		if err != nil {
			t.Fatal(err)
		}
		var asking string
		for _, id := range strings.Fields(string(ids)) {
			if ip(id) == seen["peer"] {
				asking = id
			}
		}
		if asking == "" || !strings.HasPrefix(asking, seen["hostname"]) {
			t.Errorf("the address the gateway saw, %s, is not the sandbox container %s (containers %v)", seen["peer"], seen["hostname"], strings.Fields(string(ids)))
		}
		// A bridge network shared with the gateway also routes out to the
		// internet. Restricting that is the operator's, not the broker's.
		t.Logf("the sandbox's internet access on this network: %q", seen["internet"])
	})

	t.Run("commits made in the sandbox", func(t *testing.T) {
		before := g.sh("git -C " + path + " rev-parse HEAD")
		g.turn(child, workdir, "Commit, branch and merge.",
			step{"tool": "exec", "arguments": map[string]any{"command": `set -e
g() { git -c user.name=Sandbox -c user.email=sandbox@example.com "$@"; }
echo a > a.md && g add a.md && g commit -qm "sandbox commit"
g switch -qc side && echo b > b.md && g add b.md && g commit -qm "sandbox side"
g switch -q ` + branch + ` && g merge -q --no-ff side -m "sandbox merge"
echo "head=$(git rev-parse HEAD)"; echo "parents=$(git log -1 --format=%P | wc -w | tr -d ' ')"`}},
			step{"text": "Done."},
		)
		_, results := g.lastTurn()
		seen := fields(results[len(results)-1])
		t.Logf("in the sandbox: %v", seen)
		var log, status string
		for deadline := time.Now().Add(60 * time.Second); time.Now().Before(deadline); time.Sleep(2 * time.Second) {
			log = g.sh("git -C " + path + " log --format='%H %P %s' " + before + "..HEAD")
			status = g.sh("git -C " + path + " status --porcelain")
			if log != "" || strings.Contains(status, "a.md") {
				break
			}
		}
		t.Logf("on the gateway, commits since the turn:\n%s\nstatus:\n%s\nbranches: %s", log, status, g.sh("git -C "+path+" branch --format='%(refname:short)' | tr '\n' ' '"))
		// The sandbox's Git metadata is private: its commits, branches and
		// merges stay there, and only the files reconcile, as changes for the
		// gateway to commit. So a merge in the sandbox would reach the gateway
		// as plain edits, and catch-up merges belong on the gateway.
		if seen["parents"] != "2" {
			t.Fatalf("the sandbox did not make its merge: %v", seen)
		}
		if log != "" || strings.Contains(g.sh("git -C "+path+" branch --list side"), "side") {
			t.Errorf("the sandbox's commits or branches reached the gateway:\n%s", log)
		}
		if !strings.Contains(status, "a.md") || !strings.Contains(status, "b.md") {
			t.Errorf("the sandbox's files did not reach the managed worktree: %q", status)
		}
	})
}

// wtc catch-up in a sandbox, through the plugin's git service: the sandbox
// reads the gateway's clone, merges its base with Git's own conflict state,
// and the gateway takes the branch, the sandbox's commits included. Only
// the collection's branches and origin's are served, and nothing is taken
// by a push.
func TestSandboxCatchUp(t *testing.T) {
	g := startGateway(t, gatewayOptions{repos: syntheticRepos, stubModel: true, sandbox: true, sandboxNetwork: true, plugin: true})
	const who = "-c user.name=t -c user.email=t@example.com"
	upstream := func(script string) {
		t.Helper()
		g.sh(`rm -rf /tmp/up && git clone -q /tmp/origins/api.git /tmp/up && cd /tmp/up && ` + script + ` && git add -A && git ` + who + ` commit -qm "main moves" && git push -q origin main
git -C /home/node/repos/api fetch -q /tmp/origins/api.git +refs/heads/main:refs/remotes/origin/main`)
	}
	upstream(`printf 'base\n' > shared.md && printf 'base\n' > notes.md`)
	// Sessions start from the clone's main: its origin is not reachable here.
	g.sh("git -C /home/node/repos/api merge -q --ff-only origin/main")
	// Another collection's branch, which this one must not see.
	hidden := g.sh(`cd /home/node/repos/api && h=$(git ` + who + ` commit-tree -p origin/main -m "someone else's" 'origin/main^{tree}') && git update-ref refs/heads/openclaw/someone-else "$h" && echo "$h"`)
	if r := g.call("projects.register", map[string]any{"path": "/home/node/repos/api", "name": "api"}); r.str("id") != "api" {
		t.Fatalf("projects.register: %v", r)
	}
	out, err := exec.Command("docker", "inspect", "--format", `{{(index .NetworkSettings.Networks "`+g.network+`").IPAddress}}`, g.container).Output()
	if err != nil {
		t.Fatal(err)
	}
	service := "http://" + strings.TrimSpace(string(out)) + ":18790"
	g.sh(gatewayEnv + `node dist/index.js config set ` + PluginConfigKey + `.sandboxGit '{"port":18790}' --strict-json >/dev/null
node dist/index.js config set agents.defaults.sandbox.docker.env '{"` + GatewayGitEnv + `":"` + service + `"}' --strict-json >/dev/null
node dist/index.js plugins reload wtc >/dev/null 2>&1 || true
for _ in $(seq 1 60); do curl -s localhost:18790/wtc/session | grep -q sandbox && break; sleep 1; done`)
	if answer := g.sh("curl -s localhost:18790/wtc/session"); !strings.Contains(answer, "no sandbox container") {
		t.Fatalf("the gateway itself, which is no sandbox, was answered: %s", answer)
	}

	worktree := fmt.Sprintf("wtc-cu-%d", time.Now().Unix())
	branch := Branch(worktree)
	parent := g.call("sessions.create", map[string]any{"agentId": "dev", "label": worktree}).str("key")
	add, err := AddRepo("api", "api", "", worktree, "", "Work on api")
	if err != nil {
		t.Fatal(err)
	}
	created := g.createFromIntent(parent, add)
	child, path := created.str("key"), created.str("worktree", "path")
	if child == "" || path == "" {
		t.Fatalf("no child session: %v", created)
	}
	// The base moves after the session started, against its work.
	upstream(`printf 'main\n' > shared.md && printf 'new\n' > new.md`)
	base := g.sh("git -C /home/node/repos/api rev-parse origin/main")
	const workdir = "/workspace"
	sandbox := func(message, command string) map[string]string {
		t.Helper()
		g.turn(child, workdir, message, step{"tool": "exec", "arguments": map[string]any{"command": command}}, step{"text": "Done."})
		_, results := g.lastTurn()
		if len(results) == 0 {
			t.Fatalf("no tool result:\n%s", g.modelLog())
		}
		t.Logf("%s, in the sandbox:\n%s", message, results[len(results)-1])
		return fields(results[len(results)-1])
	}

	var first map[string]string
	t.Run("the merge stops on the conflict, with Git's merge state", func(t *testing.T) {
		first = sandbox("Work, then catch up.", `g() { git `+who+` "$@"; }
printf 'session\n' > shared.md && g commit -qam "session work"
printf 'wip\n' > notes.md
echo "doctor=$(wtc doctor 2>&1 | tr '\n' ';')"
wtc catch-up > /tmp/cu.out 2>&1; echo "catchup=$?"; echo "said=$(tr '\n' ' ' < /tmp/cu.out)"
echo "conflicted=$(git diff --name-only --diff-filter=U | tr '\n' ' ')"
echo "mergelog=$(git log --merge --format=%s | tr '\n' ';')"
echo "theirs=$(git show :3:shared.md)"
echo "refs=$(git ls-remote gateway | awk '{print $2}' | tr '\n' ' ')"
echo "hidden=$(git fetch -q gateway `+hidden+` >/dev/null 2>&1 && git cat-file -e `+hidden+` 2>/dev/null && echo fetched || echo refused)"
echo "push=$(git push -q gateway HEAD:refs/heads/pushed >/dev/null 2>&1 && echo pushed || echo refused)"`)
		if first["catchup"] != "0" || first["conflicted"] != "shared.md " || first["theirs"] != "main" {
			t.Fatalf("the catch-up did not stop on shared.md with the base's side: %v", first)
		}
		if !strings.Contains(first["mergelog"], "session work") || !strings.Contains(first["mergelog"], "main moves") {
			t.Errorf("git log --merge does not show both sides: %q", first["mergelog"])
		}
		if !strings.Contains(first["doctor"], "read-only from the gateway") {
			t.Errorf("wtc doctor in the sandbox: %q", first["doctor"])
		}
		if strings.Contains(first["refs"], "someone-else") || !strings.Contains(first["refs"], "refs/heads/"+branch) || !strings.Contains(first["refs"], "refs/remotes/origin/main") {
			t.Errorf("the sandbox was shown %q", first["refs"])
		}
		if first["hidden"] != "refused" {
			t.Errorf("another collection's commit could be fetched: %q", first["hidden"])
		}
		if g.sh("git -C /home/node/repos/api branch --list pushed") != "" || first["push"] != "refused" {
			t.Errorf("a push was taken: %q", first["push"])
		}
	})
	if t.Failed() {
		return
	}

	t.Run("the gateway takes the resolved merge and the sandbox's commits", func(t *testing.T) {
		seen := sandbox("Resolve.", `printf 'resolved\n' > shared.md && git add shared.md
wtc catch-up --continue > /tmp/cu.out 2>&1; echo "catchup=$?"; echo "said=$(tr '\n' ' ' < /tmp/cu.out)"
echo "head=$(git rev-parse HEAD)"; echo "parents=$(git log -1 --format=%P)"; echo "notes=$(cat notes.md)"`)
		if seen["catchup"] != "0" || seen["notes"] != "wip" {
			t.Fatalf("continue: %v", seen)
		}
		if gw := g.sh("git -C " + path + " rev-parse HEAD"); gw != seen["head"] {
			t.Fatalf("the gateway is at %s, the sandbox at %s", gw, seen["head"])
		}
		if parents := strings.Fields(seen["parents"]); len(parents) != 2 || parents[1] != base || g.sh("git -C "+path+" log -1 --format=%s "+parents[0]) != "session work" {
			t.Errorf("the merge's parents %q: the sandbox's commit and origin/main %s", seen["parents"], base)
		}
		var status string
		for deadline := time.Now().Add(60 * time.Second); time.Now().Before(deadline); time.Sleep(2 * time.Second) {
			if status = g.sh("git -C " + path + " status --porcelain -- . ':!.openclaw'"); status == "M notes.md" {
				break
			}
		}
		if status != "M notes.md" {
			t.Errorf("the managed worktree after the turn, want only the uncommitted notes.md: %q", status)
		}
		t.Logf("on the gateway:\n%s", g.sh("git -C "+path+" log --graph --oneline -6"))
	})

	t.Run("the session goes on", func(t *testing.T) {
		seen := sandbox("Look around.", `echo "status=$(git status --porcelain -- . ':!.openclaw' | tr '\n' ';')"; wtc catch-up > /tmp/cu.out 2>&1; echo "catchup=$?"; echo "said=$(tr '\n' ' ' < /tmp/cu.out)"`)
		if seen["catchup"] != "0" || !strings.Contains(seen["said"], "already contains") {
			t.Errorf("a second catch-up: %v", seen)
		}
	})
}
