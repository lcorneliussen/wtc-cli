package wtc

import (
	"encoding/json"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func runtimeFixture(t *testing.T) *Context {
	t.Helper()
	c := fixture(t)
	fixtureGit(t, "-C", c.Harness, "init", "-q", "-b", "main")
	fixtureGit(t, "-C", c.Harness, "remote", "add", "origin", "https://example.invalid/harness-example.git")
	app := filepath.Join(c.Collection, "widget")
	fixtureFile(t, filepath.Join(app, "README.md"), "fixture\n", 0644)
	fixtureGit(t, "-C", app, "init", "-q", "-b", "main")
	c.Config.Runtime.Backend = "dekit"
	binary := filepath.Join(t.TempDir(), "dekit")
	fixtureFile(t, binary, `#!/bin/sh
if [ "$1" = --version ]; then echo 'dekit 0.10.0'; exit; fi
root=$2
shift 3
printf '%s\n' "$*" >> "$root/calls"
case "$1 $2" in
 'runner status') if [ -f "$root/running" ]; then echo '{"status":"running"}'; else echo '{"status":"absent"}'; fi ;;
 'runner stop') if [ -f "$root/running" ] || [ -f "$root/saved" ]; then rm -f "$root/running" "$root/saved"; echo '{}'; else echo 'Runner is not running.' >&2; exit 1; fi ;;
 'ls ') cat "$root/fake-tasks.json" ;;
 'down ') rm -f "$root/running"; touch "$root/saved"; echo '{}' ;;
 *) touch "$root/running"; printf '%s' "$RUNTIME_TEST_VALUE" > "$root/env-seen"; echo '{"matched":1}' ;;
esac
`, 0755)
	c.Config.Runtime.Binary = binary
	fixtureFile(t, filepath.Join(app, ".harness/dekit.tasks.yaml"), `tasks:
 web/server:
  cmd: [sh, -c, 'sleep 60']
  autostart: true
  ready: {log: READY}
 web/tunnel:
  cmd: [sh, -c, 'sleep 60']
  deps: [web/server]
  tags: [endpoint]
`, 0644)
	return c
}

func TestRuntimeOptInPlanAndReadOnlyStatus(t *testing.T) {
	c := runtimeFixture(t)
	fixtureFile(t, filepath.Join(c.Collection, ".env.collection.local"), "touch should-not-run\n", 0600)
	if items := c.RuntimeStatus(); len(items) != 0 {
		t.Fatal(items)
	}
	if _, err := os.Stat(c.runtimeRoot()); !os.IsNotExist(err) {
		t.Fatal("status created runtime")
	}
	m, data, err := c.runtimePlan("dekit")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Tasks) != 2 || m.Tasks[1].Kind != "endpoint" || m.Tasks[1].Deps[0] != "widget/web/server" {
		t.Fatalf("bad ownership: %+v", m.Tasks)
	}
	var native struct {
		Tasks map[string]map[string]any `yaml:"tasks"`
	}
	if err := yaml.Unmarshal(data, &native); err != nil {
		t.Fatal(err)
	}
	cwd, _ := filepath.EvalSymlinks(filepath.Join(c.Collection, "widget"))
	if native.Tasks["widget/web/server"]["cwd"] != cwd {
		t.Fatal("cwd was not rebased")
	}
	if _, err := os.Stat(filepath.Join(c.Collection, "should-not-run")); !os.IsNotExist(err) {
		t.Fatal("read-only operation sourced env")
	}
	c.Config.Runtime.Backend = ""
	if _, _, err := c.runtimePlan("dekit"); err == nil {
		t.Fatal("accepted unconfigured backend")
	}
}

func TestRuntimeRejectsUnsafeDefinitions(t *testing.T) {
	for _, fragment := range []string{
		"tasks: {../bad: {cmd: [true]}}\n",
		"tasks: {web: {cmd: [true], deps: [host::db]}}\n",
		"tasks: {web: {cmd: [true], deps: [missing]}}\n",
		"tasks: {web: {cmd: [true], cwd: ../..}}\n",
		"tasks: {web: {cmd: [true]}}\non_init: up\n",
		"tasks: {}\n---\ntasks: {}\n",
		"tasks: {web: {cmd: [true], x-wtc: {url: 'https://user:password@example.invalid'}}}\n",
	} {
		t.Run(fragment, func(t *testing.T) {
			c := runtimeFixture(t)
			fixtureFile(t, filepath.Join(c.Collection, "widget/.harness/dekit.tasks.yaml"), fragment, 0644)
			if _, _, err := c.runtimePlan("dekit"); err == nil {
				t.Fatal("accepted unsafe config")
			}
		})
	}
	c := runtimeFixture(t)
	if err := os.Symlink(t.TempDir(), c.runtimeRoot()); err == nil {
		t.Fatal("unexpected parent directory")
	}
	if err := os.MkdirAll(filepath.Dir(c.runtimeRoot()), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), c.runtimeRoot()); err != nil {
		t.Fatal(err)
	}
	if _, err := c.RenderRuntime(false); err == nil {
		t.Fatal("accepted symlink runtime")
	}
}

func TestRuntimeLifecycleHooksAndSavedConfig(t *testing.T) {
	c := runtimeFixture(t)
	fixtureFile(t, filepath.Join(c.Collection, ".env.collection"), "RUNTIME_TEST_VALUE='scoped value'\n", 0600)
	app := filepath.Join(c.Collection, "widget/.harness")
	fixtureFile(t, filepath.Join(app, "runtime-provision.sh"), "#!/bin/sh\necho provision\nexit 7\n", 0755)
	fixtureFile(t, filepath.Join(app, "runtime-teardown.sh"), "#!/bin/sh\necho teardown\nexit 9\n", 0755)
	if _, err := c.RuntimeAction("up", "", 0); err == nil {
		t.Fatal("ignored provisioning failure")
	}
	if _, err := os.Stat(filepath.Join(c.runtimeRoot(), "running")); !os.IsNotExist(err) {
		t.Fatal("started after failed provision")
	}
	m, err := c.runtimeManifest()
	if err != nil || m == nil || len(m.Resources) != 1 || m.Resources[0].State != "provision-attempted" {
		t.Fatalf("lost partial resource ownership: %+v %v", m, err)
	}
	if err := c.RetireRuntime(); err == nil {
		t.Fatal("ignored teardown failure")
	}
	if _, err := os.Stat(filepath.Join(c.runtimeRoot(), "manifest.json")); err != nil {
		t.Fatal("removed manifest on failure")
	}
	fixtureFile(t, filepath.Join(app, "runtime-provision.sh"), "#!/bin/sh\necho provision\n", 0755)
	fixtureFile(t, filepath.Join(c.runtimeRoot(), "fake-tasks.json"), `{"tasks":[{"path":"widget/web/server","state":"ready"},{"path":"widget/web/tunnel","state":"ready"}]}`, 0600)
	if _, err := c.RuntimeAction("up", "widget/web", time.Second); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(c.runtimeRoot(), "env-seen"))
	if string(got) != "scoped value" {
		t.Fatalf("wrong env %q", got)
	}
	fragment := filepath.Join(app, "dekit.tasks.yaml")
	data, _ := os.ReadFile(fragment)
	fixtureFile(t, fragment, strings.ReplaceAll(string(data), "sleep 60", "sleep 61"), 0644)
	if _, err := c.RenderRuntime(false); err == nil {
		t.Fatal("changed live runtime config")
	}
	if _, err := c.RuntimeAction("down", "", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := c.RenderRuntime(false); err != nil {
		t.Fatal(err)
	}
	calls, _ := os.ReadFile(filepath.Join(c.runtimeRoot(), "calls"))
	if !strings.Contains(string(calls), "runner stop") {
		t.Fatal("did not discard saved definitions")
	}
	fixtureFile(t, filepath.Join(app, "runtime-teardown.sh"), "#!/bin/sh\necho teardown\n", 0755)
	// Preserve resource ownership across changed task definitions.
	data, _ = os.ReadFile(fragment)
	fixtureFile(t, fragment, strings.ReplaceAll(string(data), "web/tunnel:", "web/endpoint:"), 0644)
	if _, err := c.RenderRuntime(false); err != nil {
		t.Fatal(err)
	}
	m, _ = c.runtimeManifest()
	m.Tasks = nil
	if err := runtimeSaveManifest(m); err != nil {
		t.Fatal(err)
	}
	if err := c.RetireRuntime(); err != nil {
		t.Fatal(err)
	}
	m, err = c.runtimeManifest()
	if err != nil || m == nil || len(m.Resources) != 0 {
		t.Fatalf("did not clear resource ledger: %v %v", m, err)
	}
	log, _ := os.ReadFile(filepath.Join(m.Root, "hook-logs/widget/provision.log"))
	if !strings.Contains(string(log), "provision") {
		t.Fatal("hook log unavailable")
	}
}

func TestRuntimeLogParentsAndSelectors(t *testing.T) {
	c := runtimeFixture(t)
	if _, err := c.RenderRuntime(false); err != nil {
		t.Fatal(err)
	}
	m, _ := c.runtimeManifest()
	for _, target := range []string{"host::db", "../widget", "widget/**", "+all", "unknown"} {
		if _, _, err := runtimeSelect(m, target); err == nil {
			t.Fatal("accepted selector", target)
		}
	}
	if selector, tasks, err := runtimeSelect(m, "widget/web"); err != nil || selector != "widget/web/**" || len(tasks) != 2 {
		t.Fatal(selector, tasks, err)
	}
	dir := filepath.Join(m.Root, "logs/widget/web")
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), dir); err != nil {
		t.Fatal(err)
	}
	if _, err := c.RuntimeLogPath("widget/web/server"); err == nil {
		t.Fatal("followed log parent symlink")
	}
}

func TestRetireRuntimeFailurePreservesWorktrees(t *testing.T) {
	c := newWorkspaceFixture(t)
	result, err := c.NewCollection(NewOptions{Slug: "runtime-failure", Repos: []string{"widget"}})
	if err != nil {
		t.Fatal(err)
	}
	target, err := OpenCollection(result.Collection)
	if err != nil {
		t.Fatal(err)
	}
	fake := runtimeFixture(t)
	target.Config.Runtime = fake.Config.Runtime
	fixtureFile(t, filepath.Join(target.Collection, "widget/.harness/dekit.tasks.yaml"), "tasks: {web: {cmd: [sh, -c, 'sleep 60']}}\n", 0644)
	fixtureFile(t, filepath.Join(target.Collection, "widget/.harness/runtime-teardown.sh"), "#!/bin/sh\nexit 8\n", 0755)
	if _, err := target.RenderRuntime(false); err != nil {
		t.Fatal(err)
	}
	if _, err := c.RetireCollection(RetireOptions{Name: "runtime-failure", Force: true}); err == nil || !strings.Contains(err.Error(), "runtime cleanup") {
		t.Fatal("ignored cleanup failure", err)
	}
	for _, dir := range []string{"harness", "widget", ".wtc/runtime"} {
		if _, err := os.Stat(filepath.Join(result.Collection, dir)); err != nil {
			t.Fatal("lost state", dir, err)
		}
	}
}

func TestRuntimeStatusIncludesResourceHookFacts(t *testing.T) {
	c := runtimeFixture(t)
	if _, err := c.RenderRuntime(false); err != nil {
		t.Fatal(err)
	}
	m, _ := c.runtimeManifest()
	m.Resources = []runtimeResource{{Dir: "widget", Repo: "widget", State: "provision-hook-completed"}}
	if err := runtimeSaveManifest(m); err != nil {
		t.Fatal(err)
	}
	items := c.RuntimeStatus()
	if len(items) != 3 || items[2].State != "provision-hook-completed" {
		t.Fatal(items)
	}
	data, _ := json.Marshal(items)
	if strings.Contains(string(data), "scoped value") {
		t.Fatal("leaked env")
	}
}

func TestRuntimeWaitsForFiniteJobs(t *testing.T) {
	c := runtimeFixture(t)
	fixtureFile(t, filepath.Join(c.Collection, "widget/.harness/dekit.tasks.yaml"), "tasks:\n migrate:\n  cmd: [true]\n  type: job\n  autostart: true\n", 0644)
	if _, err := c.RenderRuntime(false); err != nil {
		t.Fatal(err)
	}
	response := filepath.Join(c.runtimeRoot(), "fake-tasks.json")
	fixtureFile(t, response, `{"tasks":[{"path":"widget/migrate","state":"running"}]}`, 0600)
	if _, err := c.RuntimeAction("up", "widget/migrate", 25*time.Millisecond); err == nil {
		t.Fatal("running job counted as complete")
	}
	fixtureFile(t, response, `{"tasks":[{"path":"widget/migrate","state":"done"}]}`, 0600)
	if _, err := c.RuntimeAction("up", "widget/migrate", time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeEndpointMetadataRerendersWithoutConfigChange(t *testing.T) {
	c := runtimeFixture(t)
	fragment := filepath.Join(c.Collection, "widget/.harness/dekit.tasks.yaml")
	base, _ := os.ReadFile(fragment)
	withURL := func(url string) {
		fixtureFile(t, fragment, string(base)+"  x-wtc: {url: '"+url+"'}\n", 0644)
	}
	withURL("https://one.example.invalid")
	first, err := c.RenderRuntime(false)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := c.runtimeManifest()
	withURL("https://two.example.invalid")
	second, err := c.RenderRuntime(false)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := c.runtimeManifest()
	if first.Items[1].URL != "https://one.example.invalid" || second.Items[1].URL != "https://two.example.invalid" || after.Tasks[1].URL != "https://two.example.invalid" {
		t.Fatalf("stale endpoint: %+v %+v", second.Items, after.Tasks)
	}
	if before.Digest != after.Digest {
		t.Fatal("metadata changed the generated config")
	}
}

func TestRuntimeReadinessProbeURLIsShownOnlyWhenPlain(t *testing.T) {
	for probe, want := range map[string]string{
		"http://127.0.0.1:3000/health":                 "http://127.0.0.1:3000/health",
		"http://user:password@127.0.0.1:3000/health":   "",
		"http://127.0.0.1:3000/health?token=synthetic": "",
	} {
		c := runtimeFixture(t)
		fixtureFile(t, filepath.Join(c.Collection, "widget/.harness/dekit.tasks.yaml"), "tasks: {web: {cmd: [true], ready: {http: '"+probe+"'}}}\n", 0644)
		m, data, err := c.runtimePlan("dekit")
		if err != nil {
			t.Fatal(err)
		}
		if m.Tasks[0].URL != want || !m.Tasks[0].Ready || !strings.Contains(string(data), probe) {
			t.Fatalf("probe %s shown as %q", probe, m.Tasks[0].URL)
		}
	}
}
