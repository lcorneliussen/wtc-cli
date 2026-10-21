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
