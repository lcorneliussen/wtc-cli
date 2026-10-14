package wtc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func runtimeDekitCollection(t *testing.T, slug string) *Context {
	t.Helper()
	binary := os.Getenv("WTC_TEST_DEKIT")
	if binary == "" {
		runtimeToolsSkip(t, "set WTC_TEST_DEKIT to the pinned dekit 0.10.0 binary")
	}
	binary, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	owner := newWorkspaceFixture(t)
	result, err := owner.NewCollection(NewOptions{Slug: slug, Repos: []string{"widget"}})
	if err != nil {
		t.Fatal(err)
	}
	c, err := OpenCollection(result.Collection)
	if err != nil {
		t.Fatal(err)
	}
	c.Config.Runtime.Backend, c.Config.Runtime.Binary = "dekit", binary
	t.Cleanup(func() {
		if err := c.RetireRuntime(); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})
	return c
}

func TestDekitStartupFailureCarriesTheTaskOutput(t *testing.T) {
	c := runtimeDekitCollection(t, "startup-failure")
	fixtureFile(t, filepath.Join(c.Collection, "widget/.harness/dekit.tasks.yaml"), `tasks:
 web/server:
  cmd: [sh, -c, 'echo compiling; echo "main.go:3:1: syntax error: unexpected }" >&2; exit 2']
  autostart: true
  ready: {log: WEB_READY, timeout: 5s}
`, 0644)
	result, err := c.RuntimeAction("up", "", 10*time.Second)
	if err == nil {
		t.Fatal("a failing build reported success")
	}
	if len(result.Logs) != 1 || result.Logs[0].Path != "widget/web/server" || !strings.Contains(result.Logs[0].Tail, "main.go:3:1: syntax error") {
		t.Fatalf("failure without the build error: %v / %+v", err, result.Logs)
	}
}

func TestDekitAppliesChangedServicesLive(t *testing.T) {
	c := runtimeDekitCollection(t, "live-apply")
	fragment := filepath.Join(c.Collection, "widget/.harness/dekit.tasks.yaml")
	write := func(tasks string) {
		fixtureFile(t, fragment, "tasks:\n"+tasks, 0644)
	}
	task := func(name, word string) string {
		return " " + name + ":\n  cmd: [sh, -c, 'echo " + word + "_READY; exec sleep 300']\n  autostart: true\n  ready: {log: " + word + "_READY, timeout: 5s}\n"
	}
	states := func() map[string]string {
		got := map[string]string{}
		for _, item := range c.RuntimeStatus() {
			got[item.Path] = item.State
		}
		return got
	}
	write(task("web/server", "A") + task("worker/main", "B"))
	if _, err := c.RuntimeAction("up", "", 10*time.Second); err != nil {
		t.Fatal(err)
	}
	// Add one service, remove another, redefine the third: no wtc down.
	write(task("web/server", "A2") + task("api/main", "C"))
	if _, err := c.RuntimeAction("up", "", 10*time.Second); err != nil {
		t.Fatal(err)
	}
	got := states()
	if got["widget/web/server"] != "ready" || got["widget/api/main"] != "ready" {
		t.Fatalf("states after live apply: %v", got)
	}
	if _, present := got["widget/worker/main"]; present {
		t.Fatalf("removed service still listed: %v", got)
	}
	m, _ := c.runtimeManifest()
	var data []byte
	for _, task := range m.Tasks {
		if task.Path == "widget/web/server" {
			data, _ = os.ReadFile(task.Log)
		}
	}
	if !strings.Contains(string(data), "A2_READY") {
		t.Fatalf("redefined service kept its old command:\n%s", data)
	}
	runner := runtimeCommandOutput(t, m, "ls")
	if strings.Contains(runner, "worker/main") {
		t.Fatalf("removed service still known to the runner: %s", runner)
	}
}

func runtimeCommandOutput(t *testing.T, m *runtimeManifest, args ...string) string {
	t.Helper()
	data, err := runtimeCommand(m, false, args...)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
