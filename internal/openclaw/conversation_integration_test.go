//go:build openclaw_integration

package openclaw

// Whole conversations: an agent turn runs wtc in its shell, reads the plan
// and calls the tool the intent names. The model is scripted (see
// testdata/stub-model.mjs), everything else is the real gateway: the exec
// tool and its environment, sessions_spawn, github_publish, and GitHub.

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// childTurn answers the first turn of a session that sessions_spawn starts,
// which runs while the parent's turn is still going.
func childTurn() step {
	return step{"text": "Ready.", "match": "Wait for instructions", "notMatch": `"role":"tool"`}
}

func (g *gateway) plan(file string) Plan {
	g.t.Helper()
	var plan Plan
	raw := g.sh("cat " + file)
	if err := json.Unmarshal([]byte(raw), &plan); err != nil {
		g.t.Fatalf("%s is not a plan: %v\n%s", file, err, raw)
	}
	return plan
}

// children are the sessions spawned under parent, by project, once their
// first turns are over: a spawned session gets its worktree as its run starts.
func (g *gateway) children(parent string, want int) map[string]reply {
	g.t.Helper()
	var found map[string]reply
	for deadline := time.Now().Add(90 * time.Second); time.Now().Before(deadline); time.Sleep(2 * time.Second) {
		listed := g.call("sessions.list", map[string]any{})
		found = map[string]reply{}
		settled := true
		sessions, _ := listed["sessions"].([]any)
		for _, s := range sessions {
			s := reply(s.(map[string]any))
			if s.str("parentSessionKey") == parent && s.str("projectId") != "" {
				found[s.str("projectId")] = s
				if active, _ := s["hasActiveRun"].(bool); active {
					settled = false
				}
			}
		}
		if settled && len(found) >= want {
			return found
		}
	}
	g.t.Fatalf("the parent has %d settled children, want %d: %v\n%s", len(found), want, found, g.modelLog())
	return nil
}

func TestConversationAddsRepositories(t *testing.T) {
	g := startGateway(t, gatewayOptions{repos: syntheticRepos, stubModel: true})
	for _, repo := range []string{"api", "docs"} {
		if r := g.call("projects.register", map[string]any{"path": "/home/node/repos/" + repo, "name": repo}); r.str("id") != repo {
			t.Fatalf("projects.register %s: %v", repo, r)
		}
	}
	const worktree = "72-openclaw-host"
	parent := g.call("sessions.create", map[string]any{"agentId": "dev", "label": worktree}).str("key")

	g.turn(parent, agentWorkspace, "Add api and docs to this collection.",
		step{"tool": "exec", "arguments": map[string]any{"command": "wtc host > /tmp/host.txt; wtc add-repo api docs --worktree " + worktree + " 2> /tmp/plan.err | tee /tmp/plan.json"}},
		step{"carryOut": 0},
		step{"carryOut": 1, "match": "childSessionKey"},
		childTurn(), childTurn(),
		step{"text": "Added api and docs.", "match": "childSessionKey"},
	)

	if host := g.sh("cat /tmp/host.txt"); host != string(HostOpenClaw) {
		t.Fatalf("wtc host in the agent's shell: %q", host)
	}
	plan := g.plan("/tmp/plan.json")
	if len(plan.Intents) != 2 || plan.Intents[0].Tool != ToolSessionsSpawn || plan.Intents[0].Actor != ActorParent {
		t.Fatalf("plan %+v\n%s", plan, g.sh("cat /tmp/plan.err"))
	}
	children := g.children(parent, 2)
	for _, repo := range []string{"api", "docs"} {
		child, ok := children[repo]
		if !ok {
			t.Fatalf("no child session for %s under the parent: %v\n%s", repo, children, g.modelLog())
		}
		if got := child.str("worktree", "branch"); got != Branch(worktree) {
			t.Fatalf("%s: branch %q, want %q\nchild: %v\n%s", repo, got, Branch(worktree), child, g.modelLog())
		}
		path := child.str("workspaceDir")
		if got := g.sh("git -C " + path + " branch --show-current"); got != Branch(worktree) {
			t.Errorf("%s: checkout %s is on %q", repo, path, got)
		}
	}

	t.Run("a child's shell is in its worktree and wtc refuses what is not safe", func(t *testing.T) {
		child := children["api"].str("key")
		path := children["api"].str("workspaceDir")
		g.turn(child, path, "Publish.",
			step{"tool": "exec", "arguments": map[string]any{"command": "pwd > /tmp/child.pwd; echo TOKEN=x > .env; echo work > WORK; wtc publish --title Work > /tmp/refused.json 2> /tmp/refused.err; echo $? > /tmp/refused.rc; rm .env"}},
			step{"text": "Refused."},
		)
		if got := g.sh("cat /tmp/child.pwd"); got != path {
			t.Errorf("the child's shell runs in %q, want its worktree %q", got, path)
		}
		if rc, msg := g.sh("cat /tmp/refused.rc"), g.sh("cat /tmp/refused.err"); rc != "1" || !strings.Contains(msg, ".env") || g.sh("cat /tmp/refused.json") != "" {
			t.Errorf("wtc publish with a .env in the worktree: exit %s\n%s", rc, msg)
		}
	})

	t.Run("off GitHub the plan names no tool and the checkout must be committed", func(t *testing.T) {
		child := children["docs"].str("key")
		g.turn(child, children["docs"].str("workspaceDir"), "Publish.",
			step{"tool": "exec", "arguments": map[string]any{"command": `echo work > WORK; wtc publish > /dev/null 2> /tmp/dirty.err; echo $? > /tmp/dirty.rc
git add . && git commit -qm Work && wtc publish --title "Add work" > /tmp/docs.json 2> /tmp/docs.err`}},
			step{"text": "Planned."},
		)
		if rc, msg := g.sh("cat /tmp/dirty.rc"), g.sh("cat /tmp/dirty.err"); rc != "1" || !strings.Contains(msg, "commit your work first") {
			t.Errorf("uncommitted work on Bitbucket: exit %s\n%s", rc, msg)
		}
		plan := g.plan("/tmp/docs.json")
		in := plan.Intents[0]
		raw, _ := json.Marshal(in.Args)
		var args PullRequestArgs
		_ = json.Unmarshal(raw, &args)
		want := PullRequestArgs{Owner: "example-ws", Repo: "docs", Source: Branch(worktree), Dest: "main", Title: "Add work", Draft: true}
		if in.Tool != "" || in.Forge != ForgeBitbucket || args != want {
			t.Errorf("intent %+v\n%s", in, g.sh("cat /tmp/docs.err"))
		}
	})
}

func TestGitHubConversation(t *testing.T) {
	hub := gitHubTarget(t)
	g := startGateway(t, gatewayOptions{repos: hub.clone(), secrets: []string{"GH_TOKEN=" + hub.token}, stubModel: true, plugin: true})
	worktree := fmt.Sprintf("wtc-talk-%d", time.Now().Unix())
	branch := Branch(worktree)
	side := "wtc-e2e-base/" + worktree
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
		hub.do("DELETE", "/git/refs/heads/"+side, nil, nil)
	})
	if r := g.call("projects.register", map[string]any{"path": "/home/node/repos/target", "name": "target"}); r.str("id") != "target" {
		t.Fatalf("projects.register: %v", r)
	}
	parent := g.call("sessions.create", map[string]any{"agentId": "dev", "label": worktree}).str("key")

	g.turn(parent, agentWorkspace, "Add target to this collection.",
		step{"tool": "exec", "arguments": map[string]any{"command": "wtc add-repo target --worktree " + worktree}},
		step{"carryOut": 0},
		childTurn(),
		step{"text": "Added target.", "match": "childSessionKey"},
	)
	child, ok := g.children(parent, 1)["target"]
	if !ok {
		t.Fatal("the turn did not create a child session for target")
	}
	path := child.str("workspaceDir")

	// The publication is the gateway's: it commits, pushes and opens the
	// pull request after the turn that asked for it.
	awaitHead := func(t *testing.T, differentFrom string) pullRequest {
		t.Helper()
		for deadline := time.Now().Add(90 * time.Second); time.Now().Before(deadline); time.Sleep(3 * time.Second) {
			if prs := openPRs(); len(prs) == 1 && prs[0].Head.SHA != differentFrom {
				return prs[0]
			}
		}
		t.Fatalf("no publication arrived on %s:\n%s", branch, g.redact(g.sh("tail -4 /tmp/gateway.log | cut -c1-500")))
		return pullRequest{}
	}

	var first pullRequest
	t.Run("the agent publishes with wtc publish and github_publish", func(t *testing.T) {
		g.turn(child.str("key"), path, "Add a note and publish it.",
			step{"tool": "exec", "arguments": map[string]any{"command": "echo note > " + worktree + `.md && wtc publish --title "wtc conversation ` + worktree + `" --body "Opened from an agent turn by the wtc integration test."`}},
			step{"carryOut": 0},
			step{"text": "Published."},
		)
		first = awaitHead(t, "")
		if !first.Draft || first.Title != "wtc conversation "+worktree || first.Head.Ref != branch {
			t.Errorf("pull request %+v", first)
		}
		if got := g.sh("git -C " + path + " status --porcelain"); got != "" {
			t.Errorf("the worktree is not clean after publication:\n%s", got)
		}
		if got := g.sh("git -C " + path + " rev-parse HEAD"); got != first.Head.SHA {
			t.Errorf("pull request head %s, worktree %s", first.Head.SHA, got)
		}
	})
	if first.Number == 0 {
		t.Fatal("no pull request to go on with")
	}

	t.Run("the collection view shows the pull request", func(t *testing.T) {
		row := repoRows(g.collectionView(parent))["target"]
		if row.str("pullRequest", "url") == "" || !strings.HasSuffix(row.str("pullRequest", "url"), fmt.Sprintf("/pull/%d", first.Number)) ||
			row["published"] != true || row["unpublished"] != float64(0) || row["dirty"] != float64(0) {
			t.Errorf("row %v", row)
		}
	})

	t.Run("the agent catches up by merging and publishes the merge", func(t *testing.T) {
		// Upstream moves on a side branch, so that the repository's default
		// branch is left alone; wtc catch-up takes it as the destination.
		var main struct {
			Object struct {
				SHA string `json:"sha"`
			} `json:"object"`
		}
		hub.do("GET", "/git/ref/heads/"+g.sh("git -C /home/node/repos/target branch --show-current"), nil, &main)
		if code := hub.do("POST", "/git/refs", map[string]string{"ref": "refs/heads/" + side, "sha": main.Object.SHA}, nil); code != 201 {
			t.Fatalf("creating %s: HTTP %d", side, code)
		}
		if code := hub.do("PUT", "/contents/"+worktree+".upstream", map[string]string{"message": "Upstream moves", "content": "dXBzdHJlYW0K", "branch": side}, nil); code != 201 {
			t.Fatalf("committing to %s: HTTP %d", side, code)
		}
		g.turn(child.str("key"), path, "Catch up and publish.",
			step{"tool": "exec", "arguments": map[string]any{"command": "wtc catch-up --json --dest " + side + " > /tmp/catchup.json 2> /tmp/catchup.err; wtc publish --dest " + side}},
			step{"carryOut": 0},
			step{"text": "Caught up and published."},
		)
		var caught struct {
			Data CatchUp `json:"data"`
		}
		if err := json.Unmarshal([]byte(g.sh("cat /tmp/catchup.json")), &caught); err != nil || !caught.Data.Merged {
			t.Fatalf("wtc catch-up: %+v %v\n%s", caught, err, g.redact(g.sh("cat /tmp/catchup.json /tmp/catchup.err")))
		}
		merged := awaitHead(t, first.Head.SHA)
		if merged.Number != first.Number {
			t.Errorf("pull request %d, want %d", merged.Number, first.Number)
		}
		if got := g.sh("git -C " + path + " rev-list --count --merges HEAD"); got != "1" {
			t.Errorf("%s merge commits on the branch, want 1", got)
		}
		if got := g.sh("git -C " + path + " cat-file -e HEAD:" + worktree + ".upstream && echo present"); got != "present" {
			t.Errorf("the upstream change is not in the published branch")
		}
		first = merged
	})

	t.Run("after rewritten history wtc refuses and the pull request stays", func(t *testing.T) {
		g.turn(child.str("key"), path, "Tidy the history and publish.",
			step{"tool": "exec", "arguments": map[string]any{"command": `git commit -q --amend --allow-empty -m "Tidied"; echo more >> ` + worktree + `.md; wtc publish > /tmp/rewritten.json 2> /tmp/rewritten.err; echo $? > /tmp/rewritten.rc`}},
			step{"text": "wtc refused."},
		)
		if rc, msg := g.sh("cat /tmp/rewritten.rc"), g.sh("cat /tmp/rewritten.err"); rc != "1" || !strings.Contains(msg, "rewritten") {
			t.Errorf("wtc publish after an amend: exit %s\n%s", rc, msg)
		}
		if prs := openPRs(); len(prs) != 1 || prs[0].Head.SHA != first.Head.SHA {
			t.Errorf("the pull request moved: %+v", prs)
		}
	})
}
