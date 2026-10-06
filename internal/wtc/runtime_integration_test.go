package wtc

import (
	"fmt"
	"golang.org/x/sys/unix"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func runtimeFreePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	return port
}
func runtimeGet(t *testing.T, port int) string {
	t.Helper()
	client := http.Client{Timeout: time.Second}
	response, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
func runtimePortClosed(t *testing.T, port int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 100*time.Millisecond)
		if err != nil {
			return
		}
		conn.Close()
		if time.Now().After(deadline) {
			t.Fatalf("port %d survived shutdown", port)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// CI sets WTC_TEST_REQUIRE_RUNTIME so a missing tool fails instead of skipping.
func runtimeToolsSkip(t *testing.T, reason string) {
	t.Helper()
	if os.Getenv("WTC_TEST_REQUIRE_RUNTIME") != "" {
		t.Fatal(reason)
	}
	t.Skip(reason)
}

// Run explicitly against the pinned upstream release, without replacing a host
// installation: WTC_TEST_DEKIT=/absolute/path/to/dekit go test ./internal/wtc -run TestDekitRuntimeIntegration -v
func TestDekitRuntimeIntegration(t *testing.T) {
	binary := os.Getenv("WTC_TEST_DEKIT")
	if binary == "" {
		runtimeToolsSkip(t, "set WTC_TEST_DEKIT to the pinned dekit 0.10.0 binary")
	}
	binary, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal(err)
	}
	owner := newWorkspaceFixture(t)
	shared := filepath.Join(owner.Workspace, "shared-resource")
	fixtureFile(t, shared, "external synthetic resource", 0600)
	var collections []*Context
	var ports [][2]int
	for _, name := range []string{"runtime-one", "runtime-two"} {
		result, err := owner.NewCollection(NewOptions{Slug: name, Repos: []string{"widget"}})
		if err != nil {
			t.Fatal(err)
		}
		c, err := OpenCollection(result.Collection)
		if err != nil {
			t.Fatal(err)
		}
		c.Config.Runtime.Backend = "dekit"
		c.Config.Runtime.Binary = binary
		collections = append(collections, c)
		t.Cleanup(func() {
			if err := c.RetireRuntime(); err != nil {
				t.Errorf("cleanup: %v", err)
			}
		})
		port, relay := runtimeFreePort(t), runtimeFreePort(t)
		ports = append(ports, [2]int{port, relay})
		app := filepath.Join(c.Collection, "widget")
		for _, file := range []string{"dev.py", "server.py", "relay.py"} {
			data, err := os.ReadFile(filepath.Join("testdata/runtime", file))
			if err != nil {
				t.Fatal(err)
			}
			fixtureFile(t, filepath.Join(app, file), string(data), 0644)
		}
		for _, file := range []string{"runtime-provision.sh", "runtime-teardown.sh"} {
			data, err := os.ReadFile(filepath.Join("testdata/runtime", file))
			if err != nil {
				t.Fatal(err)
			}
			fixtureFile(t, filepath.Join(app, ".harness", file), string(data), 0755)
		}
		// These values represent generated collection ports and endpoint metadata.
		fixtureFile(t, filepath.Join(c.Collection, ".env.collection.local"), fmt.Sprintf("PORT=%d\nRELAY_PORT=%d\nWEB_URL='http://127.0.0.1:%d'\nRELAY_URL='http://127.0.0.1:%d'\n", port, relay, port, relay), 0600)
		fragment := fmt.Sprintf(`tasks:
 web/server:
  cmd: [%q, dev.py]
  autostart: true
  ready: {log: WEB_READY, timeout: 3s}
  x-wtc: {url_env: WEB_URL}
 web/tunnel:
  cmd: [%q, relay.py]
  deps: [web/server]
  autostart: true
  tags: [endpoint]
  ready: {log: ENDPOINT_READY, timeout: 3s}
  x-wtc: {url_env: RELAY_URL}
`, python, python)
		fixtureFile(t, filepath.Join(app, ".harness/dekit.tasks.yaml"), fragment, 0644)
		if items := c.RuntimeStatus(); len(items) != 0 {
			t.Fatal("status unexpectedly created runtime", items)
		}
		if _, err := c.RuntimeAction("up", "", 5*time.Second); err != nil {
			t.Fatal(err)
		}
		if got := runtimeGet(t, port); got != name {
			t.Fatal(got, name)
		}
		if got := runtimeGet(t, relay); got != name {
			t.Fatal(got, name)
		}
		items := c.RuntimeStatus()
		if len(items) != 3 || items[0].State != "ready" || items[1].Kind != "endpoint" || items[1].URL != fmt.Sprintf("http://127.0.0.1:%d", relay) {
			t.Fatal(items)
		}
	}
	one, two := collections[0], collections[1]
	fixtureFile(t, filepath.Join(one.Collection, "widget/.runtime-data/database"), "persistent fixture data", 0600)
	// CLI launch processes have already exited; both detached runners still serve.
	if _, err := one.RuntimeAction("restart", "widget/web", 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if got := runtimeGet(t, ports[1][1]); got != "runtime-two" {
		t.Fatal(got)
	}
	logPath, err := one.RuntimeLogPath("widget/web/server")
	if err != nil {
		t.Fatal(err)
	}
	log, err := os.ReadFile(logPath)
	if err != nil || !strings.Contains(string(log), "BUILD cached") {
		t.Fatalf("cached build and persistent log missing: %q %v", log, err)
	}
	childData, err := os.ReadFile(filepath.Join(one.Collection, "widget/child.pid"))
	if err != nil {
		t.Fatal(err)
	}
	child, err := strconv.Atoi(string(childData))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := one.RuntimeAction("down", "widget/web", 0); err != nil {
		t.Fatal(err)
	}
	runtimePortClosed(t, ports[0][0])
	runtimePortClosed(t, ports[0][1])
	deadline := time.Now().Add(3 * time.Second)
	for unix.Kill(child, 0) == nil {
		if time.Now().After(deadline) {
			t.Fatal("nested child survived group stop")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if got := runtimeGet(t, ports[1][0]); got != "runtime-two" {
		t.Fatal(got)
	}
	if _, err := one.RuntimeAction("up", "widget/web", 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if _, err := one.RuntimeAction("down", "", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := one.RuntimeLogPath("widget/web/server"); err != nil {
		t.Fatal("logs lost after down", err)
	}
	if _, err := os.Stat(filepath.Join(one.Collection, "widget/.runtime-data/database")); err != nil {
		t.Fatal("down removed data", err)
	}
	persisted, _ := os.ReadFile(filepath.Join(one.Collection, "widget/.runtime-data/database"))
	if string(persisted) != "persistent fixture data" {
		t.Fatal("idempotent provision lost data")
	}
	// Changed definitions after down must replace dekit's saved snapshot.
	fragment := filepath.Join(one.Collection, "widget/.harness/dekit.tasks.yaml")
	data, _ := os.ReadFile(fragment)
	fixtureFile(t, fragment, strings.ReplaceAll(string(data), "ENDPOINT_READY", "NEVER_READY"), 0644)
	if _, err := one.RuntimeAction("up", "", 5*time.Second); err == nil {
		t.Fatal("readiness failure reported success")
	}
	found := false
	for _, item := range one.RuntimeStatus() {
		if item.Path == "widget/web/tunnel" && item.State == "exited" {
			found = true
		}
	}
	if !found {
		t.Fatal("failed startup is not visible", one.RuntimeStatus())
	}
	// A teardown failure must preserve real worktrees and its ownership receipt.
	local := filepath.Join(one.Collection, ".env.collection.local")
	baseEnv, _ := os.ReadFile(local)
	fixtureFile(t, local, string(baseEnv)+"RUNTIME_TEARDOWN_FAIL=1\n", 0600)
	if _, err := owner.RetireCollection(RetireOptions{Name: "runtime-one", Force: true}); err == nil {
		t.Fatal("retirement ignored resource teardown failure")
	}
	if _, err := os.Stat(filepath.Join(one.Collection, "widget/.runtime-data/owner")); err != nil {
		t.Fatal("retirement removed receipt on failure", err)
	}
	fixtureFile(t, local, string(baseEnv), 0600)
	// Retirement is scoped, including saved runner state; second stays alive.
	if err := one.RetireRuntime(); err != nil {
		t.Fatal(err)
	}
	runtimePortClosed(t, ports[0][0])
	runtimePortClosed(t, ports[0][1])
	if got := runtimeGet(t, ports[1][1]); got != "runtime-two" {
		t.Fatal(got)
	}
	if err := two.RetireRuntime(); err != nil {
		t.Fatal(err)
	}
	runtimePortClosed(t, ports[1][0])
	runtimePortClosed(t, ports[1][1])
	// Existing local command has a useful fallback with no WTC/dekit environment.
	cmd := exec.Command(python, "server.py")
	cmd.Dir = filepath.Join(one.Collection, "widget")
	cmd.Env = []string{"PATH=/usr/bin:/bin", "PORT=0"}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cmd.Process.Kill(); cmd.Wait() }()
	line := make([]byte, 100)
	n, err := stdout.Read(line)
	if err != nil || !strings.Contains(string(line[:n]), "WEB_READY port=") {
		t.Fatalf("standalone failed: %q %v", line[:n], err)
	}
	standalonePort, err := strconv.Atoi(strings.TrimSpace(strings.Split(string(line[:n]), "port=")[1]))
	if err != nil {
		t.Fatal(err)
	}
	if got := runtimeGet(t, standalonePort); got != "standalone" {
		t.Fatal(got)
	}
	cmd.Process.Kill()
	cmd.Wait()
	for _, name := range []string{"runtime-one", "runtime-two"} {
		result, err := owner.RetireCollection(RetireOptions{Name: name, Force: true})
		if err != nil || !result.FolderRemoved {
			t.Fatalf("verified retirement failed: %+v %v", result, err)
		}
	}
	if _, err := os.Stat(shared); err != nil {
		t.Fatal("removed shared resource", err)
	}
}
