//go:build openclaw_integration

package openclaw

// A real publication: the gateway commits a child session's work, pushes its
// branch and opens a draft pull request on GitHub.
//
//	WTC_OPENCLAW_GITHUB_REPO=<owner>/<throwaway-repo> GH_TOKEN=$(gh auth token) \
//	  go test -tags openclaw_integration ./internal/openclaw/ -run GitHub -v
//
// The token is the gateway's GitHub identity for the run: it lives in the
// test container's environment and nowhere else. The test leaves the
// repository's default branch alone and removes the branches and the pull
// request it created.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

const gitHubHelper = `-c credential.helper='!gh auth git-credential'`

type gitHub struct {
	t           *testing.T
	repo, token string
}

func (h gitHub) do(method, path string, body any, into any) int {
	h.t.Helper()
	var payload io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			h.t.Fatal(err)
		}
		payload = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, "https://api.github.com/repos/"+h.repo+path, payload)
	if err != nil {
		h.t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+h.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if into != nil && res.StatusCode < 300 {
		if err := json.Unmarshal(raw, into); err != nil {
			h.t.Fatalf("%s %s: %v\n%s", method, path, err, raw)
		}
	}
	return res.StatusCode
}

// clone is the gateway setup that makes the repository project "target".
func (h gitHub) clone() string {
	return "git " + gitHubHelper + ` clone -q "https://github.com/` + h.repo + `.git" /home/node/repos/target` + "\n"
}

func gitHubTarget(t *testing.T) gitHub {
	t.Helper()
	repo := os.Getenv("WTC_OPENCLAW_GITHUB_REPO")
	token := os.Getenv("GH_TOKEN")
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}
	if repo == "" || token == "" {
		t.Skip("needs WTC_OPENCLAW_GITHUB_REPO=<owner>/<throwaway-repo> and GH_TOKEN")
	}
	return gitHub{t, repo, token}
}

type pullRequest struct {
	Number int    `json:"number"`
	State  string `json:"state"`
	Draft  bool   `json:"draft"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	Head   struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
}

func TestGitHubPublication(t *testing.T) {
	hub := gitHubTarget(t)
	g := startGateway(t, gatewayOptions{repos: hub.clone(), secrets: []string{"GH_TOKEN=" + hub.token}})

	worktree := fmt.Sprintf("wtc-e2e-%d", time.Now().Unix())
	branch := Branch(worktree)
	side := "wtc-e2e-base/" + worktree
	t.Cleanup(func() {
		var open []pullRequest
		hub.do("GET", "/pulls?state=open&head="+strings.Split(hub.repo, "/")[0]+":"+branch, nil, &open)
		for _, pr := range open {
			hub.do("PATCH", fmt.Sprintf("/pulls/%d", pr.Number), map[string]string{"state": "closed"}, nil)
		}
		hub.do("DELETE", "/git/refs/heads/"+branch, nil, nil)
		hub.do("DELETE", "/git/refs/heads/"+side, nil, nil)
	})

	project := g.call("projects.register", map[string]any{"path": "/home/node/repos/target", "name": "target"})
	origin := ParseOrigin(project.str("originUrl"))
	if origin.Forge != ForgeGitHub || origin.Owner+"/"+origin.Repo != hub.repo {
		t.Fatalf("origin %+v from %v", origin, project)
	}
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
	defaultBranch := g.sh("git -C /home/node/repos/target branch --show-current")

	// publish is what github_publish does with the intent's arguments: the
	// gateway is told which session and nothing about repository or branch.
	n := 0
	publish := func(t *testing.T, title string) reply {
		t.Helper()
		n++
		in := Publish("target", origin, worktree, defaultBranch, title, "Opened by the wtc integration test.")
		if in.Tool != ToolGitHubPublish {
			t.Fatalf("intent %+v", in)
		}
		args := in.Args.(PublishArgs)
		params := map[string]any{"sessionKey": child, "idempotencyKey": fmt.Sprintf("%s-%d", worktree, n)}
		if args.Title != "" {
			params["title"] = args.Title
		}
		if args.Body != "" {
			params["body"] = args.Body
		}
		return g.call("sessions.github.publish", params)
	}
	published := func(t *testing.T, r reply) pullRequest {
		t.Helper()
		if r.str("status") != "published" || r.str("branch") != branch || r.str("repository") != hub.repo {
			t.Fatalf("publication: %v", r)
		}
		var prs []pullRequest
		hub.do("GET", "/pulls?state=open&head="+origin.Owner+":"+branch, nil, &prs)
		if len(prs) != 1 {
			t.Fatalf("%d open pull requests for %s, want 1", len(prs), branch)
		}
		if want := g.sh("git -C " + path + " rev-parse HEAD"); prs[0].Head.SHA != want || r.str("headCommit") != want {
			t.Errorf("pull request head %s, reply %s, checkout %s", prs[0].Head.SHA, r.str("headCommit"), want)
		}
		if !strings.HasSuffix(r.str("url"), fmt.Sprintf("/pull/%d", prs[0].Number)) {
			t.Errorf("reply url %q is not pull request %d", r.str("url"), prs[0].Number)
		}
		return prs[0]
	}

	var first pullRequest
	t.Run("uncommitted work becomes a commit, a branch and a draft pull request", func(t *testing.T) {
		g.sh("echo one > " + path + "/" + worktree + ".txt")
		first = published(t, publish(t, "wtc e2e "+worktree))
		if !first.Draft || first.Base.Ref != defaultBranch || first.Title != "wtc e2e "+worktree {
			t.Errorf("pull request %+v", first)
		}
		if got := g.sh("git -C " + path + " status --porcelain"); got != "" {
			t.Errorf("checkout is not clean after publication:\n%s", got)
		}
		if got := g.sh("git -C " + path + " log -1 --format=%B"); !strings.Contains(got, "OpenClaw-Publication:") {
			t.Errorf("publication commit has no marker:\n%s", got)
		}
	})
	if first.Number == 0 {
		t.Fatal("no pull request to go on with")
	}

	t.Run("the agent's own commits go to the same pull request", func(t *testing.T) {
		g.sh("cd " + path + " && echo two >> " + worktree + `.txt && git add . && git commit -qm "Agent commit"`)
		if pr := published(t, publish(t, "")); pr.Number != first.Number {
			t.Errorf("pull request %d, want %d", pr.Number, first.Number)
		}
	})

	t.Run("a merge catch-up is published", func(t *testing.T) {
		g.sh(`git ` + gitHubHelper + ` clone -q "https://github.com/` + hub.repo + `.git" /tmp/elsewhere && cd /tmp/elsewhere
git switch -qc ` + side + ` && echo upstream > ` + worktree + `.upstream && git add . && git commit -qm "Upstream moves"
git ` + gitHubHelper + ` push -q origin ` + side)
		g.sh("cd " + path + " && git " + gitHubHelper + " fetch -q origin " + side + " && git merge -q --no-edit FETCH_HEAD")
		if pr := published(t, publish(t, "")); pr.Number != first.Number {
			t.Errorf("pull request %d, want %d", pr.Number, first.Number)
		}
		if got := g.sh("git -C " + path + " rev-list --count --merges HEAD"); got != "1" {
			t.Errorf("%s merge commits on the branch, want 1", got)
		}
	})

	t.Run("rewritten history is refused and nothing is pushed", func(t *testing.T) {
		before := published(t, publish(t, "")).Head.SHA
		g.sh("cd " + path + ` && git commit -q --amend --allow-empty -m "Rewritten" && echo three >> ` + worktree + ".txt")
		r := publish(t, "")
		if r.str("status") != "failed" || r.str("code") != "push_rejected" {
			t.Fatalf("publication of rewritten history: %v", r)
		}
		var prs []pullRequest
		hub.do("GET", "/pulls?state=open&head="+origin.Owner+":"+branch, nil, &prs)
		if len(prs) != 1 || prs[0].Head.SHA != before {
			t.Errorf("pull request moved after a refused publication: %+v", prs)
		}
	})

	t.Run("the parent owns no repository and cannot publish", func(t *testing.T) {
		r := g.call("sessions.github.publish", map[string]any{"sessionKey": parent, "idempotencyKey": worktree + "-parent"})
		if !r.failed() {
			t.Fatalf("parent publication: %v", r)
		}
	})
}
