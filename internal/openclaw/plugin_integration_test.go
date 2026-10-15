//go:build openclaw_integration

package openclaw

// The collection plugin (../../openclaw-plugin) in a gateway: its session
// action is what the Control UI panel renders, so the test asks for it the
// way the panel does.

import (
	"strings"
	"testing"
	"time"
)

// collectionView is the plugin's answer for a session.
func (g *gateway) collectionView(sessionKey string) reply {
	g.t.Helper()
	var last reply
	// The gateway loads a freshly installed plugin on its own time.
	for deadline := time.Now().Add(60 * time.Second); time.Now().Before(deadline); time.Sleep(2 * time.Second) {
		last = g.call("plugins.sessionAction", map[string]any{"pluginId": "wtc", "actionId": "collection", "sessionKey": sessionKey, "agentId": "dev"})
		if result, ok := last["result"].(map[string]any); ok {
			return reply(result)
		}
	}
	g.t.Fatalf("the collection plugin did not answer: %v\n%s", last, g.sh("tail -5 /tmp/gateway.log | cut -c1-400"))
	return nil
}

func repoRows(view reply) map[string]reply {
	rows := map[string]reply{}
	repos, _ := view["repos"].([]any)
	for _, r := range repos {
		r := reply(r.(map[string]any))
		rows[r.str("project")] = r
	}
	return rows
}

func TestCollectionPlugin(t *testing.T) {
	g := startGateway(t, gatewayOptions{repos: syntheticRepos, plugin: true})
	const worktree = "72-openclaw-host"
	for _, repo := range []string{"api", "docs"} {
		g.call("projects.register", map[string]any{"path": "/home/node/repos/" + repo, "name": repo})
	}
	parent := g.call("sessions.create", map[string]any{"agentId": "dev", "label": worktree}).str("key")
	if view := g.collectionView(parent); view.str("schema") != CollectionSchema || len(repoRows(view)) != 0 {
		t.Fatalf("a session without repositories: %v", view)
	}
	keys := map[string]string{}
	for _, repo := range []string{"api", "docs"} {
		in, _ := AddRepo(repo, repo, "", worktree, "", "Work on "+repo)
		keys[repo] = g.createFromIntent(parent, in).str("key")
	}
	g.sh("cd " + g.worktreePath("api", worktree) + ` && echo one > one.txt && git add . && git commit -qm "One" && echo stray > stray.txt`)

	check := func(t *testing.T, view reply) {
		t.Helper()
		if view.str("collection", "sessionKey") != parent || view.str("collection", "label") != worktree {
			t.Errorf("collection %v", view["collection"])
		}
		rows := repoRows(view)
		if len(rows) != 2 {
			t.Fatalf("%d repositories, want 2: %v", len(rows), view)
		}
		api, docs := rows["api"], rows["docs"]
		if api.str("sessionKey") != keys["api"] || api.str("branch") != Branch(worktree) || api.str("forge") != string(ForgeGitHub) ||
			api["ahead"] != float64(1) || api["dirty"] != float64(1) || api["untracked"] != float64(1) || api["unpublished"] != float64(1) {
			t.Errorf("api row %v", api)
		}
		if docs.str("forge") != string(ForgeBitbucket) || docs["ahead"] != float64(0) || docs["dirty"] != float64(0) || docs["pullRequest"] != nil {
			t.Errorf("docs row %v", docs)
		}
	}
	t.Run("the parent shows every repository of the collection", func(t *testing.T) { check(t, g.collectionView(parent)) })
	t.Run("a child shows its siblings too", func(t *testing.T) {
		view := g.collectionView(keys["docs"])
		if view.str("requestedSessionKey") != keys["docs"] {
			t.Errorf("requested %q", view.str("requestedSessionKey"))
		}
		check(t, view)
	})

	t.Run("catch up merges in the child's worktree", func(t *testing.T) {
		g.sh(`git clone -q /tmp/origins/docs.git /tmp/elsewhere && cd /tmp/elsewhere
echo upstream > UPSTREAM && git add . && git commit -qm "Upstream moves" && git push -q origin main`)
		// The synthetic origin is not reachable by its forge URL.
		path := g.worktreePath("docs", worktree)
		g.sh("git -C " + path + " remote set-url origin /tmp/origins/docs.git")
		r := g.call("plugins.sessionAction", map[string]any{"pluginId": "wtc", "actionId": "catch-up", "sessionKey": keys["docs"], "agentId": "dev"})
		result, _ := r["result"].(map[string]any)
		if result["code"] != float64(0) || !strings.Contains(reply(result).str("json", "summary"), "merged origin/main") {
			t.Fatalf("catch-up: %v", r)
		}
		if got := g.sh("git -C " + path + " cat-file -e HEAD:UPSTREAM && echo present"); got != "present" {
			t.Error("the upstream change did not arrive in the worktree")
		}
	})
}
