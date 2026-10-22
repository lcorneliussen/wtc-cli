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

// A catch-up resolved in the sandbox, with Git's own merge state: the
// sandbox fetches its session branch and the base from the gateway's clone,
// which needs no forge credential, merges there, and only the files come
// back. The gateway then records the merge commit the sandbox made: the same
// parents and the same tree.
func TestSandboxMergesFromGateway(t *testing.T) {
	g := startGateway(t, gatewayOptions{repos: syntheticRepos, stubModel: true, sandbox: true, sandboxNetwork: true})
	const who = "-c user.name=t -c user.email=t@example.com"
	g.sh(`cd /home/node/repos/api && printf 'base\n' > shared.md && printf 'base\n' > other.md && git add . && git ` + who + ` commit -qm base`)
	if r := g.call("projects.register", map[string]any{"path": "/home/node/repos/api", "name": "api"}); r.str("id") != "api" {
		t.Fatalf("projects.register: %v", r)
	}
	worktree := fmt.Sprintf("wtc-mrg-%d", time.Now().Unix())
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
	settled := func(want string) string {
		t.Helper()
		var status string
		for deadline := time.Now().Add(60 * time.Second); time.Now().Before(deadline); time.Sleep(2 * time.Second) {
			if status = g.sh("git -C " + path + " status --porcelain"); strings.Contains(status, want) {
				return status
			}
		}
		t.Fatalf("%s did not reach the managed worktree: %q", want, status)
		return ""
	}

	// The session's work, committed on the gateway as wtc does; then the
	// base moves and conflicts with it.
	sandbox("Edit shared.md.", `printf 'session\n' > shared.md`)
	settled("shared.md")
	g.sh("git -C " + path + " " + who + " commit -qam 'session work'")
	g.sh(`cd /home/node/repos/api && printf 'main\n' > shared.md && printf 'main\n' > other.md && printf 'new\n' > new.md && git add . && git ` + who + ` commit -qm 'main moves'`)
	session, base := g.sh("git -C "+path+" rev-parse HEAD"), g.sh("git -C /home/node/repos/api rev-parse main")

	// The gateway's clone, served read-only. Telling sessions apart and
	// limiting each to its own repositories is the broker's, not this test's.
	g.sh(`nohup git daemon --reuseaddr --export-all --base-path=/home/node/repos --listen=0.0.0.0 --port=9418 /home/node/repos > /tmp/git-daemon.log 2>&1 &
sleep 1`)
	out, err := exec.Command("docker", "inspect", "--format", `{{(index .NetworkSettings.Networks "`+g.network+`").IPAddress}}`, g.container).Output()
	if err != nil {
		t.Fatal(err)
	}
	remote := "git://" + strings.TrimSpace(string(out)) + "/api"

	var merged map[string]string
	t.Run("the sandbox merges with Git's merge state", func(t *testing.T) {
		merged = sandbox("Catch up with main.", `g() { git `+who+` "$@"; }
echo "before=$(git rev-parse HEAD) shallow=$(git rev-parse --is-shallow-repository)"
git fetch -q --unshallow `+remote+` `+branch+`:refs/remotes/gateway/session main:refs/remotes/gateway/main 2>&1 || git fetch -q `+remote+` `+branch+`:refs/remotes/gateway/session main:refs/remotes/gateway/main 2>&1
git reset -q gateway/session
echo "afterreset=$(git status --porcelain -- . ':!.openclaw' | tr '\n' ';')"
g merge -q --no-edit gateway/main 2>&1 | tail -1
echo "conflicted=$(git diff --name-only --diff-filter=U | tr '\n' ' ')"
echo "mergelog=$(git log --merge --format=%s | tr '\n' ';')"
echo "base=$(git show :1:shared.md) ours=$(git show :2:shared.md) theirs=$(git show :3:shared.md)"
printf 'resolved\n' > shared.md && g add shared.md && g commit -q --no-edit
echo "head=$(git rev-parse HEAD)"; echo "parents=$(git log -1 --format=%P)"; echo "tree=$(git rev-parse 'HEAD^{tree}')"`)
		if merged["conflicted"] != "shared.md " {
			t.Fatalf("the merge in the sandbox did not stop on shared.md: %v", merged)
		}
		if merged["afterreset"] != "" {
			t.Errorf("the sandbox's files differ from the session branch it fetched: %q", merged["afterreset"])
		}
		if merged["parents"] != session+" "+base {
			t.Errorf("the sandbox merged %q, want %s %s", merged["parents"], session, base)
		}
	})
	if t.Failed() {
		return
	}

	t.Run("the gateway records the same merge", func(t *testing.T) {
		status := settled("new.md")
		t.Logf("the managed worktree after the turn:\n%s", status)
		// The parents are what the gateway served; the tree is what came back.
		commit := g.sh(`cd ` + path + ` && git add -A -- . ':!.openclaw' && git ` + who + ` commit-tree "$(git write-tree)" -p ` + session + ` -p ` + base + ` -m "Merge main" | tee /tmp/merge && git update-ref HEAD "$(cat /tmp/merge)" ` + session + ` >&2`)
		tree := g.sh("git -C " + path + " rev-parse '" + commit + "^{tree}'")
		if tree != merged["tree"] {
			t.Errorf("the gateway's merge has tree %s, the sandbox's %s:\n%s", tree, merged["tree"], g.sh("git -C "+path+" diff --stat "+merged["tree"]+" "+tree+" || true"))
		}
		if left := g.sh("git -C " + path + " status --porcelain -- . ':!.openclaw'"); left != "" {
			t.Errorf("the managed worktree is not clean after the merge: %q", left)
		}
		t.Logf("on the gateway:\n%s", g.sh("git -C "+path+" log --graph --oneline -6"))
	})

	t.Run("the session goes on after the merge", func(t *testing.T) {
		seen := sandbox("Look around.", `git fetch -q `+remote+` `+branch+`:refs/remotes/gateway/session 2>&1; git reset -q gateway/session
echo "head=$(git rev-parse HEAD)"; echo "status=$(git status --porcelain -- . ':!.openclaw' | tr '\n' ';')"; echo "shared=$(cat shared.md)"; printf 'more\n' >> other.md`)
		if seen["shared"] != "resolved" || seen["status"] != "" {
			t.Errorf("the sandbox after the merge: %v", seen)
		}
		if status := settled("other.md"); !strings.Contains(status, "M other.md") {
			t.Errorf("a later edit did not reconcile: %q", status)
		}
	})
}
