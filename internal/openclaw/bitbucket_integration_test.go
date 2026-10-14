//go:build openclaw_integration

package openclaw

// A publication off GitHub, where the gateway brokers nothing: an agent turn
// commits, runs wtc publish and hands the plan to the Bitbucket skill's
// publish-intent script, which pushes from the session's worktree and opens
// the pull request.
//
//	WTC_OPENCLAW_BITBUCKET_REPO=<workspace>/<throwaway-repo> \
//	WTC_OPENCLAW_BITBUCKET_SKILL=<checkout of the Bitbucket skill> \
//	TWG_BBC_USER=<account e-mail> TWG_BBC_TOKEN=<api token> \
//	  go test -tags openclaw_integration ./internal/openclaw/ -run Bitbucket -v
//
// The gateway is given what the Bitbucket plugin gives a session: a login
// file named by BITBUCKET_LOGIN_FILE. It exists in the test container only. The
// test leaves the default branch alone, declines the pull request it opened
// and deletes its branch.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type bitbucket struct {
	t                 *testing.T
	repo, user, token string
}

func (b bitbucket) do(method, path string, body any, into any) int {
	b.t.Helper()
	var payload io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		payload = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, "https://api.bitbucket.org/2.0/repositories/"+b.repo+path, payload)
	if err != nil {
		b.t.Fatal(err)
	}
	req.SetBasicAuth(b.user, b.token)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		b.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if into != nil && res.StatusCode < 300 {
		if err := json.Unmarshal(raw, into); err != nil {
			b.t.Fatalf("%s %s: %v\n%s", method, path, err, raw)
		}
	}
	return res.StatusCode
}

type bitbucketPR struct {
	ID     int    `json:"id"`
	Title  string `json:"title"`
	Draft  bool   `json:"draft"`
	State  string `json:"state"`
	Source struct {
		Branch struct {
			Name string `json:"name"`
		} `json:"branch"`
		Commit struct {
			Hash string `json:"hash"`
		} `json:"commit"`
	} `json:"source"`
	Destination struct {
		Branch struct {
			Name string `json:"name"`
		} `json:"branch"`
	} `json:"destination"`
}

func (b bitbucket) open(branch string) []bitbucketPR {
	var page struct {
		Values []bitbucketPR `json:"values"`
	}
	b.do("GET", "/pullrequests?q="+url.QueryEscape(`source.branch.name="`+branch+`" AND state="OPEN"`), nil, &page)
	return page.Values
}

func TestBitbucketConversation(t *testing.T) {
	hub := bitbucket{t, os.Getenv("WTC_OPENCLAW_BITBUCKET_REPO"), os.Getenv("TWG_BBC_USER"), os.Getenv("TWG_BBC_TOKEN")}
	skill := os.Getenv("WTC_OPENCLAW_BITBUCKET_SKILL")
	if hub.repo == "" || hub.user == "" || hub.token == "" || skill == "" {
		t.Skip("needs WTC_OPENCLAW_BITBUCKET_REPO=<workspace>/<throwaway-repo>, WTC_OPENCLAW_BITBUCKET_SKILL=<skill checkout>, TWG_BBC_USER and TWG_BBC_TOKEN")
	}
	const scripts, login = "/home/node/skills/bitbucket/scripts/", "/home/node/bitbucket.login"
	files := map[string][]byte{login: []byte("user=" + hub.user + "\ntoken=" + hub.token + "\n")}
	for _, name := range []string{"git-forge", "git-credential-bitbucket", "bb", "publish-intent.mjs"} {
		script, err := os.ReadFile(filepath.Join(skill, "skills", "bitbucket", "scripts", name))
		if err != nil {
			t.Fatal(err)
		}
		files[scripts+name] = script
	}
	g := startGateway(t, gatewayOptions{
		files: files,
		repos: "chmod 700 " + scripts + "*\ngit -c credential.https://bitbucket.org.helper=" + scripts + `git-credential-bitbucket clone -q "https://bitbucket.org/` + hub.repo + `.git" /home/node/repos/target` + "\n",
		// Not a secret: the path of the login, which is what a session gets.
		secrets:   []string{"BITBUCKET_LOGIN_FILE=" + login},
		stubModel: true,
	})
	const carrier = "node " + scripts + "publish-intent.mjs"

	worktree := fmt.Sprintf("wtc-talk-%d", time.Now().Unix())
	branch := Branch(worktree)
	t.Cleanup(func() {
		for _, pr := range hub.open(branch) {
			hub.do("POST", fmt.Sprintf("/pullrequests/%d/decline", pr.ID), nil, nil)
		}
		hub.do("DELETE", "/refs/branches/"+url.PathEscape(branch), nil, nil)
	})

	project := g.call("projects.register", map[string]any{"path": "/home/node/repos/target", "name": "target"})
	origin := ParseOrigin(project.str("originUrl"))
	if origin.Forge != ForgeBitbucket || origin.Owner+"/"+origin.Repo != hub.repo {
		t.Fatalf("origin %+v from %v", origin, project)
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
	dest := g.sh("git -C /home/node/repos/target branch --show-current")

	publish := func(t *testing.T, change, out string) map[string]any {
		t.Helper()
		g.turn(child.str("key"), path, "Commit and publish.",
			step{"tool": "exec", "arguments": map[string]any{"command": change + ` && git add . && git commit -qm "Note from the wtc integration test" && wtc publish --title "wtc conversation ` + worktree + `" --body "Opened from an agent turn by the wtc integration test." 2> ` + out + `.err | ` + carrier + ` > ` + out + ` 2>> ` + out + `.err; echo "exit $?"`}},
			step{"text": "Published."},
		)
		var result map[string]any
		if err := json.Unmarshal([]byte(g.sh("cat "+out)), &result); err != nil || result["status"] != "published" {
			t.Fatalf("publish-intent: %v %v\n%s", result, err, g.redact(g.sh("cat "+out+".err")))
		}
		return result
	}

	var first bitbucketPR
	t.Run("the agent commits, plans with wtc and the skill opens a draft pull request", func(t *testing.T) {
		result := publish(t, "echo note > "+worktree+".md", "/tmp/bb1.json")
		prs := hub.open(branch)
		if len(prs) != 1 {
			t.Fatalf("%d open pull requests for %s, want 1", len(prs), branch)
		}
		first = prs[0]
		if !first.Draft || first.Title != "wtc conversation "+worktree || first.Destination.Branch.Name != dest || result["created"] != true {
			t.Errorf("pull request %+v, result %v", first, result)
		}
		if head := g.sh("git -C " + path + " rev-parse HEAD"); !strings.HasPrefix(head, first.Source.Commit.Hash) {
			t.Errorf("pull request head %s, worktree %s", first.Source.Commit.Hash, head)
		}
	})
	if first.ID == 0 {
		t.Fatal("no pull request to go on with")
	}

	t.Run("a second publication goes to the same pull request", func(t *testing.T) {
		result := publish(t, "echo more >> "+worktree+".md", "/tmp/bb2.json")
		prs := hub.open(branch)
		if len(prs) != 1 || prs[0].ID != first.ID || result["created"] != false {
			t.Fatalf("pull requests %+v, result %v", prs, result)
		}
		if head := g.sh("git -C " + path + " rev-parse HEAD"); !strings.HasPrefix(head, prs[0].Source.Commit.Hash) {
			t.Errorf("pull request head %s, worktree %s", prs[0].Source.Commit.Hash, head)
		}
	})

	t.Run("rewritten history is refused and nothing is forced", func(t *testing.T) {
		before := hub.open(branch)[0].Source.Commit.Hash
		g.turn(child.str("key"), path, "Tidy the history and publish.",
			step{"tool": "exec", "arguments": map[string]any{"command": `git commit -q --amend -m "Tidied" && wtc publish 2> /tmp/bb3.err | ` + carrier + ` > /tmp/bb3.json 2>> /tmp/bb3.err; echo $? > /tmp/bb3.rc`}},
			step{"text": "Refused."},
		)
		if rc, msg := g.sh("cat /tmp/bb3.rc"), g.sh("cat /tmp/bb3.err"); rc != "1" || !(strings.Contains(msg, "rewritten") || strings.Contains(msg, "do not rebase")) {
			t.Errorf("publishing an amended branch: exit %s\n%s", rc, g.redact(msg))
		}
		if prs := hub.open(branch); len(prs) != 1 || prs[0].Source.Commit.Hash != before {
			t.Errorf("the pull request moved: %+v", prs)
		}
	})
}
