//go:build openclaw_integration

package openclaw

// Credit for the person who asked. Someone signed in to the gateway with a
// GitHub-verified identity prompts only the collection's session; the
// repository's session that wtc add-repo has the agent spawn publishes, and
// its commit has to carry that person's Co-authored-by trailer.
//
// OpenClaw verifies a GitHub account only through the sign-in, here a
// Tailscale login made with GitHub. The test gateway and a test client each
// run their own tailscaled in their container and join a separate test
// tailnet; the machine's own Tailscale is not involved.
//
//	WTC_TS_AUTHKEY=<reusable, ephemeral, untagged auth key of a tailnet whose
//	  user signed in with GitHub> \
//	WTC_OPENCLAW_GITHUB_REPO=<owner>/<throwaway-repo> GH_TOKEN=<publisher> \
//	  go test -tags openclaw_integration ./internal/openclaw/ -run CoAuthor -v
//
// OpenClaw never credits the account it publishes as, so the tailnet's user
// and the owner of GH_TOKEN must be different GitHub accounts.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// client is a container on the test tailnet that talks to the gateway as the
// tailnet's user, with no gateway token.
type client struct {
	g         *gateway
	container string
	url       string
}

func (c client) call(method string, params map[string]any) reply {
	c.g.t.Helper()
	raw := mustJSON(c.g.t, params)
	cmd := exec.Command("docker", "exec", "-i", c.container, "bash", "-c",
		gatewayEnv+`node dist/index.js gateway call `+method+` --url "`+c.url+`" --json --timeout 150000 --params "$(cat)" 2>&1`)
	cmd.Stdin = strings.NewReader(raw)
	out, _ := cmd.Output()
	return c.g.decode(method, out)
}

func TestCoAuthorCredit(t *testing.T) {
	hub := gitHubTarget(t)
	key := os.Getenv("WTC_TS_AUTHKEY")
	if key == "" {
		t.Skip("needs WTC_TS_AUTHKEY for a test tailnet whose user signed in with GitHub")
	}
	g := startGateway(t, gatewayOptions{repos: hub.clone(), secrets: []string{"GH_TOKEN=" + hub.token}, stubModel: true, tailnetKey: key})

	out, err := exec.Command("docker", append(append([]string{"run", "-d"}, tailnetArgs...),
		"--tmpfs", "/home/node:size=1g,uid=1000,gid=1000,exec", "--tmpfs", "/tmp:size=1g,exec",
		"--entrypoint", "sleep", tailscaleImage(t), "infinity")...).CombinedOutput()
	if err != nil {
		t.Fatalf("docker run client: %v\n%s", err, out)
	}
	c := client{g: g, container: strings.TrimSpace(string(out)), url: "wss://" + g.tailnetName}
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", c.container).Run() })
	joinTailnet(t, c.container, "wtc-client-"+c.container[:8], []string{"WTC_TS_AUTHKEY=" + key})

	// The person is whoever the tailnet says the client belongs to.
	login := strings.TrimSpace(mustOutput(t, "docker", "exec", c.container, "sh", "-c",
		`tailscale status --json | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>{const j=JSON.parse(s);console.log(j.User[j.Self.UserID].LoginName)})'`))
	githubLogin, ok := strings.CutSuffix(login, "@github")
	if !ok {
		t.Skipf("the tailnet's user %q did not sign in with GitHub", login)
	}
	var publisher struct {
		Login string `json:"login"`
	}
	if code := hub.user(&publisher); code != 200 || publisher.Login == "" || strings.EqualFold(publisher.Login, githubLogin) {
		t.Skipf("the gateway publishes as %q and OpenClaw does not credit its own account: sign in to the tailnet with another GitHub account", publisher.Login)
	}

	if r := g.call("projects.register", map[string]any{"path": "/home/node/repos/target", "name": "target"}); r.str("id") != "target" {
		t.Fatalf("projects.register: %v", r)
	}
	worktree := fmt.Sprintf("wtc-credit-%d", time.Now().Unix())
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

	// The person creates and prompts the collection's session, and nothing
	// else. The repository's session is the agent's doing.
	parent := c.call("sessions.create", map[string]any{"agentId": "dev", "label": worktree}).str("key")
	if parent == "" {
		t.Fatalf("the signed-in person could not create a session:\n%s", g.redact(g.sh("tail -5 /tmp/gateway.log | cut -c1-400")))
	}
	const childTask = "Wait for instructions"
	steps := []step{
		{"tool": "exec", "arguments": map[string]any{"command": "wtc add-repo target --worktree " + worktree}, "cwd": agentWorkspace},
		{"carryOut": 0, "cwd": agentWorkspace},
		// The spawned session's own turn: write, plan, publish.
		{"tool": "exec", "arguments": map[string]any{"command": "echo credited > " + worktree + ".md && wtc publish --title \"wtc credit " + worktree + "\""},
			"match": childTask, "notMatch": "Working directory: " + agentWorkspace},
		{"carryOut": 0, "match": childTask, "notMatch": "Working directory: " + agentWorkspace},
		{"text": "Published.", "match": childTask, "notMatch": "Working directory: " + agentWorkspace},
		{"text": "Added target and published.", "cwd": agentWorkspace, "match": "childSessionKey"},
	}
	g.put("/tmp/stub/steps.json", []byte(mustJSON(t, map[string]any{"steps": steps})))
	sent := c.call("sessions.send", map[string]any{"key": parent, "message": "Add target to this collection, then write a note there and publish it."})
	if sent.str("runId") == "" {
		t.Fatalf("sessions.send as the signed-in person: %v", sent)
	}
	if done := g.call("agent.wait", map[string]any{"runId": sent.str("runId"), "timeoutMs": 300000}); done.str("status") != "ok" {
		t.Fatalf("the collection's turn: %v\n%s", done, g.modelLog())
	}

	var pr pullRequest
	for deadline := time.Now().Add(3 * time.Minute); time.Now().Before(deadline) && pr.Number == 0; time.Sleep(5 * time.Second) {
		if prs := openPRs(); len(prs) == 1 {
			pr = prs[0]
		}
	}
	if pr.Number == 0 {
		t.Fatalf("no pull request from %s\n%s", branch, g.modelLog())
	}
	var commits []struct {
		Commit struct {
			Message string `json:"message"`
		} `json:"commit"`
	}
	hub.do("GET", fmt.Sprintf("/pulls/%d/commits", pr.Number), nil, &commits)
	trailer := "Co-authored-by: " + githubLogin + " <"
	for _, commit := range commits {
		if strings.Contains(commit.Commit.Message, trailer) {
			t.Logf("credited: %s", commit.Commit.Message[strings.Index(commit.Commit.Message, trailer):])
			return
		}
	}
	var messages []string
	for _, commit := range commits {
		messages = append(messages, commit.Commit.Message)
	}
	t.Errorf("no %q… trailer on pull request #%d; commits:\n%s", trailer, pr.Number, strings.Join(messages, "\n---\n"))
}

func mustOutput(t *testing.T, name string, args ...string) string {
	t.Helper()
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return string(out)
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// user reads the account the token belongs to.
func (h gitHub) user(into any) int {
	h.t.Helper()
	req, err := http.NewRequest("GET", "https://api.github.com/user", nil)
	if err != nil {
		h.t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+h.token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatalf("GET /user: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode == 200 {
		_ = json.NewDecoder(res.Body).Decode(into)
	}
	return res.StatusCode
}
