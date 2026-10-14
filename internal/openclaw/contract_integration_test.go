//go:build openclaw_integration

package openclaw

// The intents in this package against a gateway with synthetic repositories,
// no GitHub account and no model.

import (
	"fmt"
	"strings"
	"testing"
)

// Three repositories with a forge-shaped fetch URL, so the gateway can name a
// publication target, and a local push URL: api and web on GitHub, docs on
// Bitbucket. web also has a develop branch one commit ahead of main.
const syntheticRepos = `for r in api web docs; do
  git init -q --bare "/tmp/origins/$r.git"
  git clone -q "/tmp/origins/$r.git" "/home/node/repos/$r" 2>/dev/null
  cd "/home/node/repos/$r"
  echo "# $r" > README.md && git add . && git commit -qm "init $r" && git push -q origin main
  git remote set-url origin "https://github.com/example-org/$r.git"
  [ "$r" = docs ] && git remote set-url origin "git@bitbucket.org:example-ws/docs.git"
  git remote set-url --push origin "/tmp/origins/$r.git"
done
cd /home/node/repos/web
git switch -qc develop && echo dev > DEV && git add . && git commit -qm "develop" && git switch -q main
`

func TestGatewayContract(t *testing.T) {
	g := startGateway(t, gatewayOptions{repos: syntheticRepos})
	const worktree = "72-openclaw-host"

	origins := map[string]Origin{}
	for _, repo := range []string{"api", "web", "docs"} {
		r := g.call("projects.register", map[string]any{"path": "/home/node/repos/" + repo, "name": repo})
		if r.str("id") != repo {
			t.Fatalf("projects.register %s: %v", repo, r)
		}
		origins[repo] = ParseOrigin(r.str("originUrl"))
	}
	if origins["api"] != (Origin{ForgeGitHub, "example-org", "api"}) || origins["docs"] != (Origin{ForgeBitbucket, "example-ws", "docs"}) {
		t.Fatalf("origins as the gateway reports them: %+v", origins)
	}
	parent := g.call("sessions.create", map[string]any{"agentId": "dev", "label": worktree}).str("key")
	if parent == "" {
		t.Fatal("no parent session")
	}

	children := map[string]string{}
	t.Run("add-repo creates a child that owns the worktree", func(t *testing.T) {
		for repo, base := range map[string]string{"api": "", "web": "develop", "docs": ""} {
			in, err := AddRepo(repo, repo, worktree, base, "Work on "+repo)
			if err != nil {
				t.Fatal(err)
			}
			r := g.createFromIntent(parent, in)
			if r.failed() {
				t.Fatalf("%s: %v", repo, r)
			}
			children[repo] = r.str("key")
			if got := r.str("worktree", "branch"); got != Branch(worktree) {
				t.Errorf("%s: branch %q, want %q", repo, got, Branch(worktree))
			}
			if got := r.str("entry", "parentSessionKey"); got != parent {
				t.Errorf("%s: parent %q, want %q", repo, got, parent)
			}
			if got := r.str("entry", "projectId"); got != repo {
				t.Errorf("%s: project %q", repo, got)
			}
			path := r.str("worktree", "path")
			if got := g.sh("git -C " + path + " branch --show-current"); got != Branch(worktree) {
				t.Errorf("%s: checkout is on %q", repo, got)
			}
			if base == "" {
				base = "main"
			}
			want := g.sh("git -C /home/node/repos/" + repo + " rev-parse " + base)
			if got := g.sh("git -C " + path + " rev-parse HEAD"); got != want {
				t.Errorf("%s: worktree starts at %s, want %s (%s)", repo, got, want, base)
			}
		}
	})
	if len(children) != 3 {
		t.Fatal("no child sessions to go on with")
	}

	t.Run("a worktree name is taken once per repository", func(t *testing.T) {
		in, _ := AddRepo("api", "api", worktree, "", "again")
		r := g.createFromIntent(parent, in)
		if !r.failed() || !strings.Contains(r.str("error", "message"), "already in use") {
			t.Fatalf("second api worktree of the same name: %v", r)
		}
	})

	t.Run("worktree names: the gateway agrees with ValidWorktreeName", func(t *testing.T) {
		names := []string{"72-openclaw-host", "a", strings.Repeat("a", 64), "Feat-X", "feat/x", "feat_x", "feat.x", "-x", strings.Repeat("a", 65)}
		for _, name := range names {
			// An unknown project keeps accepted names from allocating worktrees:
			// the name is checked first, the project second.
			r := g.call("sessions.create", map[string]any{"agentId": "dev", "parentSessionKey": parent,
				"projectId": "no-such-project", "worktree": true, "worktreeName": name})
			msg := r.str("error", "message")
			accepted := strings.Contains(msg, "unknown project id")
			if !accepted && !strings.Contains(msg, "worktreeName") {
				t.Fatalf("%q: unexpected reply %v", name, r)
			}
			if accepted != ValidWorktreeName(name) {
				t.Errorf("%q: gateway accepted=%v, ValidWorktreeName=%v", name, accepted, ValidWorktreeName(name))
			}
		}
	})

	t.Run("status lists the collection's children", func(t *testing.T) {
		in := Status()
		r := g.invoke(in.Tool, parent, in.Args)
		details, _ := r["result"].(map[string]any)["details"].(map[string]any)
		sessions, _ := details["sessions"].([]any)
		found := map[string]string{}
		for _, s := range sessions {
			s := reply(s.(map[string]any))
			if s.str("parentSessionKey") == parent {
				found[s.str("projectId")] = s.str("worktree", "branch")
			}
		}
		for repo := range children {
			if found[repo] != Branch(worktree) {
				t.Errorf("%s: listed branch %q, want %q (reply %v)", repo, found[repo], Branch(worktree), r)
			}
		}
		if len(found) != len(children) {
			t.Errorf("listed %d children of the parent, want %d", len(found), len(children))
		}
	})

	// Without a GitHub identity on the gateway a publication stops at the
	// credential, after the gateway has accepted the session as the owner of
	// a publishable repository.
	admitted := func(t *testing.T, key string) {
		t.Helper()
		r := g.call("sessions.github.publish", map[string]any{"sessionKey": key, "idempotencyKey": fmt.Sprint(t.Name(), key), "title": "Add thing"})
		if msg := r.str("error", "message"); !strings.Contains(msg, "GitHub credential") {
			t.Errorf("publish: %q, want it to reach the GitHub credential", msg)
		}
	}

	t.Run("publish belongs to the child, not the parent", func(t *testing.T) {
		for _, key := range children {
			admitted(t, key)
		}
		r := g.call("sessions.github.publish", map[string]any{"sessionKey": parent, "idempotencyKey": "parent"})
		if msg := r.str("error", "message"); !r.failed() || strings.Contains(msg, "GitHub credential") {
			t.Errorf("parent publish was not refused before the credential: %v", r)
		}
	})

	t.Run("catch-up by merge keeps the child publishable", func(t *testing.T) {
		path := g.worktreePath("api", worktree)
		g.sh(`git clone -q /tmp/origins/api.git /tmp/elsewhere && cd /tmp/elsewhere
echo upstream > UPSTREAM && git add . && git commit -qm "upstream moves" && git push -q origin main`)
		g.sh("cd " + path + ` && echo work > WORK && git add . && git commit -qm "work"
git fetch -q /tmp/origins/api.git main && git merge -q --no-edit FETCH_HEAD`)
		if got := g.sh("cd " + path + " && git rev-list --count --merges HEAD"); got != "1" {
			t.Fatalf("expected one merge commit, got %s", got)
		}
		admitted(t, children["api"])
	})

	// The gateway owns the worktree of a Bitbucket repository like any other,
	// but brokers no publication for it: the intent names no tool, and the
	// push it describes is the session's own, from its checkout.
	t.Run("publish off GitHub is a push from the child's checkout", func(t *testing.T) {
		in := Publish("docs", origins["docs"], worktree, "main", "Add thing", "")
		args, ok := in.Args.(PullRequestArgs)
		if in.Tool != "" || in.Forge != ForgeBitbucket || !ok {
			t.Fatalf("intent %+v", in)
		}
		path := g.worktreePath("docs", worktree)
		g.sh("cd " + path + ` && echo work > WORK && git add . && git commit -qm "work" && git push -q origin ` + args.Source)
		want := g.sh("git -C " + path + " rev-parse HEAD")
		if got := g.sh("git -C /tmp/origins/docs.git rev-parse " + args.Source); got != want {
			t.Errorf("origin has %s at %s, want %s", args.Source, got, want)
		}
		if got := g.sh("git -C /tmp/origins/docs.git merge-base " + args.Dest + " " + args.Source); got != g.sh("git -C /tmp/origins/docs.git rev-parse "+args.Dest) {
			t.Errorf("%s is not based on %s", args.Source, args.Dest)
		}
	})

	t.Run("sessions_spawn needs a model before it looks at the intent", func(t *testing.T) {
		in, _ := AddRepo("api", "api", worktree+"-tool", "", "Work on api")
		r := g.invoke(in.Tool, parent, in.Args)
		details, _ := r["result"].(map[string]any)["details"].(map[string]any)
		if msg, _ := details["error"].(string); !strings.Contains(msg, "is not usable") {
			t.Fatalf("sessions_spawn without a model: %v", r)
		}
	})

	t.Run("github_publish is not offered outside an agent run", func(t *testing.T) {
		in := Publish("api", origins["api"], worktree, "main", "Add thing", "")
		r := g.invoke(in.Tool, children["api"], in.Args)
		if !strings.Contains(r.str("error", "message"), "Tool not available") {
			t.Fatalf("github_publish through /tools/invoke: %v", r)
		}
	})
}

// What these tests do not reach yet. Each is still unverified.
func TestNotYetCovered(t *testing.T) {
	for name, needs := range map[string]string{
		"co-author credit on a child prompted via the parent": "verified GitHub identities on people's profiles",
	} {
		t.Run(name, func(t *testing.T) { t.Skip("needs " + needs) })
	}
}
