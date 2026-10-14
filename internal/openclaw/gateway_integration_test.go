//go:build openclaw_integration

package openclaw

// Helpers for the tests that drive a throwaway OpenClaw gateway in Docker.
//
//	go test -tags openclaw_integration ./internal/openclaw/ -v
//
// The gateway runs from testdata/gateway.Dockerfile: the OpenClaw image named
// by WTC_OPENCLAW_IMAGE (or the default below) plus a Git and a GitHub CLI
// recent enough to publish, and the wtc binary built from this checkout.
// OpenClaw refuses to allocate a worktree with less than about 4 GiB free, so
// the gateway's state lives on memory-backed filesystems.

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const defaultImage = "ghcr.io/openclaw/openclaw:2026.10.2-browser"

const gatewayEnv = `cd /app
export HOME=/home/node OPENCLAW_HOME=/home/node OPENCLAW_STATE_DIR=/home/node/.openclaw
export OPENCLAW_CONFIG_PATH=/home/node/.openclaw/openclaw.json OPENCLAW_CONFIG_DIR=/home/node/.openclaw
`

const gatewayPrepare = gatewayEnv + `set -eu
git config --global user.email test@example.com
git config --global user.name Test
git config --global init.defaultBranch main
mkdir -p /tmp/origins /home/node/repos "$OPENCLAW_STATE_DIR/workspaces/dev"
`

// Without a model account the gateway's agents cannot run a turn. stubModel
// gives agent dev a model served by testdata/stub-model.mjs, and offers every
// tool to it directly instead of behind tool search.
const stubModel = `,
  "models": { "mode": "merge", "providers": { "stub": {
    "baseUrl": "http://127.0.0.1:18999/v1", "apiKey": "stub-local", "api": "openai-completions",
    "agentRuntime": { "id": "openclaw" },
    "models": [ { "id": "stub-model", "name": "Stub Model", "reasoning": false, "input": ["text"],
                  "cost": { "input": 0, "output": 0, "cacheRead": 0, "cacheWrite": 0 },
                  "contextWindow": 200000, "maxTokens": 8192 } ] } } },
  "tools": { "toolSearch": false }`

func gatewayStart(stub bool) string {
	model, defaults, start := "", "", ""
	if stub {
		model, defaults = stubModel, `"defaults": { "model": { "primary": "stub/stub-model" } }, `
		start = "mkdir -p /tmp/stub && nohup node /tmp/stub-model.mjs > /tmp/stub/stub.log 2>&1 &\n"
	}
	return `
cd /app
cat > "$OPENCLAW_CONFIG_PATH" <<JSON
{ "gateway": { "mode": "local", "bind": "loopback", "port": 18789,
               "auth": { "mode": "token", "token": "t" },
               "tools": { "allow": ["sessions_spawn", "github_publish"] } },
  "agents": { ` + defaults + `"entries": { "dev": { "workspace": "$OPENCLAW_STATE_DIR/workspaces/dev" } } }` + model + ` }
JSON
` + start + gatewayBoot
}

const gatewayBoot = `for _ in 1 2 3; do node dist/index.js doctor --fix --non-interactive > /tmp/doctor.log 2>&1 && break; done
nohup node dist/index.js gateway run --allow-unconfigured > /tmp/gateway.log 2>&1 &
for _ in $(seq 1 120); do
  node dist/index.js gateway call health --token t --json >/dev/null 2>&1 && exit 0
  sleep 1
done
tail -50 /tmp/gateway.log >&2
exit 1
`

var (
	imageOnce sync.Once
	imageTag  string
	imageErr  string
)

// testImage builds the gateway image once per test run. Docker's layer cache
// keeps the Git build from being repeated.
func testImage(t *testing.T) string {
	t.Helper()
	imageOnce.Do(func() {
		base := os.Getenv("WTC_OPENCLAW_IMAGE")
		if base == "" {
			base = defaultImage
		}
		fail := func(what string, err error, out []byte) { imageErr = what + ": " + err.Error() + "\n" + string(out) }
		arch, err := exec.Command("docker", "version", "--format", "{{.Server.Arch}}").Output()
		if err != nil {
			fail("docker version", err, nil)
			return
		}
		dir, err := os.MkdirTemp("", "wtc-openclaw-image")
		if err != nil {
			fail("temp dir", err, nil)
			return
		}
		defer os.RemoveAll(dir)
		dockerfile, err := os.ReadFile(filepath.Join("testdata", "gateway.Dockerfile"))
		if err == nil {
			err = os.WriteFile(filepath.Join(dir, "Dockerfile"), dockerfile, 0o644)
		}
		if err != nil {
			fail("Dockerfile", err, nil)
			return
		}
		build := exec.Command("go", "build", "-o", filepath.Join(dir, "wtc"), "../../cmd/wtc")
		build.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH="+strings.TrimSpace(string(arch)))
		if out, err := build.CombinedOutput(); err != nil {
			fail("go build wtc", err, out)
			return
		}
		tag := "wtc-openclaw-test:" + strings.NewReplacer("/", "-", ":", "-").Replace(base)
		if out, err := exec.Command("docker", "build", "-q", "--build-arg", "OPENCLAW_IMAGE="+base, "-t", tag, dir).CombinedOutput(); err != nil {
			fail("docker build", err, out)
			return
		}
		imageTag = tag
	})
	if imageErr != "" {
		t.Fatal(imageErr)
	}
	return imageTag
}

type gateway struct {
	t         *testing.T
	container string
	// secrets are passed into the container's environment by name and kept
	// out of command lines and failure output.
	secrets []string
	// hidden are further values that must not show up in failure output.
	hidden []string
}

type gatewayOptions struct {
	// repos is a script that prepares the repositories under /home/node/repos.
	repos string
	// secrets are NAME=value pairs the gateway process and every later
	// command see in their environment.
	secrets []string
	// files are written into the container before repos runs, readable by
	// the gateway's user only. The values in a *.login file are kept out of
	// failure output.
	files map[string][]byte
	// stubModel lets agent dev run turns against the scripted model.
	stubModel bool
	// plugin installs the collection plugin from ../../openclaw-plugin.
	plugin bool
}

func startGateway(t *testing.T, opt gatewayOptions) *gateway {
	t.Helper()
	out, err := exec.Command("docker", "run", "-d",
		"--tmpfs", "/home/node:size=9g,uid=1000,gid=1000,exec", "--tmpfs", "/tmp:size=6g,exec",
		"--entrypoint", "sleep", testImage(t), "infinity").CombinedOutput()
	if err != nil {
		t.Fatalf("docker run: %v\n%s", err, out)
	}
	g := &gateway{t: t, container: strings.TrimSpace(string(out)), secrets: opt.secrets}
	t.Cleanup(func() {
		// WTC_OPENCLAW_KEEP leaves the gateway running for a look inside.
		if os.Getenv("WTC_OPENCLAW_KEEP") != "" {
			t.Logf("kept container %s", g.container)
			return
		}
		_ = exec.Command("docker", "rm", "-f", g.container).Run()
	})
	for path, content := range opt.files {
		if strings.HasSuffix(path, ".login") {
			for _, line := range strings.Fields(string(content)) {
				_, value, _ := strings.Cut(line, "=")
				g.hidden = append(g.hidden, value)
			}
		}
		g.put(path, content)
		g.sh("chmod 600 " + path)
	}
	if opt.stubModel {
		stub, err := os.ReadFile(filepath.Join("testdata", "stub-model.mjs"))
		if err != nil {
			t.Fatal(err)
		}
		g.put("/tmp/stub-model.mjs", stub)
	}
	if out, err := g.bash(gatewayPrepare + opt.repos + gatewayStart(opt.stubModel)); err != nil {
		t.Fatalf("gateway setup: %v\n%s", err, g.redact(out))
	}
	if opt.plugin {
		source := filepath.Join("..", "..", "openclaw-plugin")
		err := filepath.WalkDir(source, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(source, path)
			g.put("/home/node/wtc-plugin/"+filepath.ToSlash(rel), content)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		// A plugin that brings its own Control UI code is behind a switch.
		g.sh(gatewayEnv + `node dist/index.js config set gateway.controlUi.experimental.customPlugins true >/dev/null 2>&1
node dist/index.js plugins install -l /home/node/wtc-plugin --force 2>&1 | tail -3`)
	}
	return g
}

func (g *gateway) exec(args ...string) *exec.Cmd {
	full := []string{"exec", "-i"}
	for _, s := range g.secrets {
		name, _, _ := strings.Cut(s, "=")
		full = append(full, "-e", name)
	}
	cmd := exec.Command("docker", append(append(full, g.container), args...)...)
	cmd.Env = append(os.Environ(), g.secrets...)
	return cmd
}

func (g *gateway) redact(s string) string {
	for _, secret := range g.secrets {
		if _, v, _ := strings.Cut(secret, "="); v != "" {
			s = strings.ReplaceAll(s, v, "<redacted>")
		}
	}
	for _, v := range g.hidden {
		if len(v) >= 8 {
			s = strings.ReplaceAll(s, v, "<redacted>")
		}
	}
	return s
}

// put writes a file in the container. docker cp cannot: the paths that
// matter are memory-backed mounts.
func (g *gateway) put(path string, content []byte) {
	g.t.Helper()
	cmd := g.exec("sh", "-c", `mkdir -p "$(dirname "$1")" && cat > "$1"`, "put", path)
	cmd.Stdin = bytes.NewReader(content)
	if out, err := cmd.CombinedOutput(); err != nil {
		g.t.Fatalf("write %s: %v\n%s", path, err, out)
	}
}

// step is one answer of the scripted model; see testdata/stub-model.mjs.
type step map[string]any

// turn sends message to a session and waits for the agent's turn, in which
// the model answers with steps. cwd is the session's working directory: the
// steps are only served to turns running there, because other sessions take
// turns of their own meanwhile — a parent is woken when a child finishes.
// It returns what agent.wait reports.
func (g *gateway) turn(sessionKey, cwd, message string, steps ...step) reply {
	g.t.Helper()
	for _, s := range steps {
		if _, routed := s["match"]; !routed {
			s["cwd"] = cwd
		}
	}
	script, err := json.Marshal(map[string]any{"steps": steps})
	if err != nil {
		g.t.Fatal(err)
	}
	g.put("/tmp/stub/steps.json", script)
	sent := g.call("sessions.send", map[string]any{"key": sessionKey, "message": message})
	if sent.str("runId") == "" {
		g.t.Fatalf("sessions.send: %v", sent)
	}
	done := g.call("agent.wait", map[string]any{"runId": sent.str("runId"), "timeoutMs": 120000})
	if done.str("status") != "ok" {
		g.t.Fatalf("agent turn did not finish: %v\n%s", done, g.modelLog())
	}
	return done
}

const agentWorkspace = "/home/node/.openclaw/workspaces/dev"

// modelLog is what the scripted model was asked and answered, one line per
// request: the step it served and the latest tool result it was shown.
func (g *gateway) modelLog() string {
	out, _ := g.bash(`node -e '
const fs = require("fs");
for (const line of fs.readFileSync("/tmp/stub/requests.jsonl", "utf8").split("\n").filter(Boolean)) {
  const r = JSON.parse(line); if (!r.served) continue;
  const tools = (r.body.messages || []).filter((m) => m.role === "tool");
  const last = tools.length ? JSON.stringify(tools[tools.length - 1].content).slice(0, 700) : "-";
  console.log("#" + r.seq, "tools=" + (r.body.tools || []).length, "served=" + JSON.stringify(r.served.step).slice(0, 300), "\n   last tool result:", last);
}'`)
	return g.redact(out)
}

func (g *gateway) bash(script string) (string, error) {
	cmd := g.exec("bash", "-s")
	cmd.Stdin = strings.NewReader(script)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// sh runs a script that has to succeed and returns its trimmed output.
func (g *gateway) sh(script string) string {
	g.t.Helper()
	out, err := g.bash("set -eu\n" + script)
	if err != nil {
		g.t.Fatalf("%s: %v\n%s", script, err, g.redact(out))
	}
	return strings.TrimSpace(out)
}

type reply map[string]any

func (r reply) str(path ...string) string {
	var v any = map[string]any(r)
	for _, k := range path {
		m, _ := v.(map[string]any)
		v = m[k]
	}
	s, _ := v.(string)
	return s
}

func (r reply) failed() bool { ok, set := r["ok"].(bool); return set && !ok }

// call is one gateway method call. A refusal comes back as a reply with
// ok=false, not as a test failure.
func (g *gateway) call(method string, params any) reply {
	g.t.Helper()
	p, err := json.Marshal(params)
	if err != nil {
		g.t.Fatal(err)
	}
	cmd := g.exec("bash", "-c", gatewayEnv+`node dist/index.js gateway call `+method+` --token t --json --timeout 150000 --params "$(cat)" 2>&1`)
	cmd.Stdin = bytes.NewReader(p)
	out, _ := cmd.Output()
	return g.decode(method, out)
}

// invoke is one tool call made as sessionKey, through POST /tools/invoke.
func (g *gateway) invoke(tool, sessionKey string, args any) reply {
	g.t.Helper()
	body, err := json.Marshal(map[string]any{"tool": tool, "sessionKey": sessionKey, "args": args})
	if err != nil {
		g.t.Fatal(err)
	}
	cmd := g.exec("curl", "-s", "-X", "POST",
		"http://127.0.0.1:18789/tools/invoke", "-H", "Authorization: Bearer t",
		"-H", "Content-Type: application/json", "--data-binary", "@-")
	cmd.Stdin = bytes.NewReader(body)
	out, _ := cmd.Output()
	return g.decode(tool, out)
}

func (g *gateway) decode(what string, out []byte) reply {
	g.t.Helper()
	start := bytes.IndexByte(out, '{')
	var r reply
	if start < 0 || json.Unmarshal(out[start:], &r) != nil {
		g.t.Fatalf("%s: no JSON reply:\n%s", what, g.redact(string(out)))
	}
	return r
}

// createFromIntent does what sessions_spawn does with an add-repo intent,
// minus the agent turn: the same project and worktree arguments, given to
// sessions.create under the parent.
func (g *gateway) createFromIntent(parent string, in Intent) reply {
	g.t.Helper()
	raw, err := json.Marshal(in.Args)
	if err != nil {
		g.t.Fatal(err)
	}
	params := map[string]any{}
	if err := json.Unmarshal(raw, &params); err != nil {
		g.t.Fatal(err)
	}
	delete(params, "task")
	delete(params, "visible")
	params["agentId"] = "dev"
	params["parentSessionKey"] = parent
	return g.call("sessions.create", params)
}

// worktreePath is where the gateway put repo's managed worktree named name.
func (g *gateway) worktreePath(repo, name string) string {
	g.t.Helper()
	return g.sh("git -C /home/node/repos/" + repo + " worktree list --porcelain | sed -n 's/^worktree //p' | grep /" + name + "$")
}
