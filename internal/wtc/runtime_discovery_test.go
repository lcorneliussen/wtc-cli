package wtc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRuntimeDiscoveryBeforeFirstStart(t *testing.T) {
	c := runtimeFixture(t)
	discoveryCommitShared(t, c)
	before := discoverySnapshot(t, c.Collection)
	for _, items := range [][]RuntimeItem{c.RuntimeEnlisted(), c.RuntimeStatus()} {
		if len(items) != 2 {
			t.Fatalf("want two declared tasks before first start, got %+v", items)
		}
		for _, item := range items {
			if item.State != "unconfigured" {
				t.Fatalf("declaration state: %+v", item)
			}
		}
	}
	if after := discoverySnapshot(t, c.Collection); !reflect.DeepEqual(before, after) {
		t.Fatal("listing changed collection files")
	}
}

func TestRuntimeDiscoveryNewSiblingPreservesRecordedFacts(t *testing.T) {
	c := runtimeFixture(t)
	discoveryCommitShared(t, c)
	task := runtimeTask{RuntimeItem: RuntimeItem{Collection: filepath.Base(c.Collection), Dir: "existing", Repo: "existing", Path: "existing/web/main", Group: "existing/web", Kind: "service", URL: "http://127.0.0.1:43100", State: "ready"}}
	task.Log = filepath.Join(c.runtimeRoot(), "logs", "existing/web/main.log")
	m := runtimeManifest{Schema: 1, Root: c.runtimeRoot(), Binary: c.Config.Runtime.Binary, Tasks: []runtimeTask{task}}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	fixtureFile(t, filepath.Join(c.runtimeRoot(), "manifest.json"), string(data), 0600)
	before := discoverySnapshot(t, c.Collection)
	got := c.RuntimeEnlisted()
	if len(got) != 3 {
		t.Fatalf("want recorded service plus new sibling tasks, got %+v", got)
	}
	if got[0].Path != task.Path || got[0].URL != task.URL || got[0].State != "checking" {
		t.Fatalf("lost recorded facts: %+v", got)
	}
	if after := discoverySnapshot(t, c.Collection); !reflect.DeepEqual(before, after) {
		t.Fatal("listing changed collection files")
	}
}

func discoverySnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			out[path] = entry.Type().String()
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			out[path] = "symlink:" + target
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[path] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestRuntimeDiscoveryDoesNotExecuteOrRewrite(t *testing.T) {
	c := runtimeFixture(t)
	marker := filepath.Join(c.Collection, "forbidden")
	command := "touch " + marker
	fixtureFile(t, filepath.Join(c.Collection, ".env.collection"), command+"\n", 0600)
	fixtureFile(t, filepath.Join(c.Collection, ".env.collection.local"), command+"\n", 0600)
	fixtureFile(t, c.Config.Runtime.Binary, "#!/bin/sh\n"+command+"\nexit 1\n", 0755)
	fixtureFile(t, filepath.Join(c.Collection, "widget/.harness/dekit.tasks.yaml"), "tasks:\n web/main:\n  cmd: '"+command+"'\n  x-wtc: {port_env: PORT, url_env: WEB_URL}\n", 0644)
	for _, hook := range []string{"runtime-provision.sh", "runtime-teardown.sh"} {
		fixtureFile(t, filepath.Join(c.Collection, "widget/.harness", hook), "#!/bin/sh\n"+command+"\n", 0755)
	}
	fixtureFile(t, filepath.Join(c.Collection, "widget/.harness/init.sh"), "#!/bin/sh\n"+command+"\n", 0755)
	fixtureFile(t, c.runtimeLocalFragment("widget"), "tasks: {worker/main: {cmd: '"+command+"'}}\n", 0600)
	login, answer := runtimeLoginCheck, runtimeAnswerCheck
	t.Cleanup(func() { runtimeLoginCheck, runtimeAnswerCheck = login, answer })
	runtimeLoginCheck = func(string) string { t.Fatal("discovery queried a provider"); return "" }
	runtimeAnswerCheck = func(string) (bool, string) { t.Fatal("discovery probed a URL"); return false, "" }
	before := discoverySnapshot(t, c.Collection)
	for _, got := range [][]RuntimeItem{c.RuntimeEnlisted(), c.RuntimeStatus()} {
		if len(got) != 2 {
			t.Fatalf("discovery: %+v", got)
		}
		for _, item := range got {
			if item.State != "unconfigured" || item.URL != "" {
				t.Fatalf("resolved runtime configuration: %+v", item)
			}
		}
	}
	if after := discoverySnapshot(t, c.Collection); !reflect.DeepEqual(before, after) {
		t.Fatal("listing executed code or changed collection files")
	}
}

func TestRuntimeDiscoveryPreservesLiveItems(t *testing.T) {
	c := runtimeFixture(t)
	code := 17
	live := []RuntimeItem{
		{Path: "widget/web/server", State: "failed", ExitCode: &code, Reason: "exit", Error: "build failed", URL: "http://127.0.0.1:43100"},
		{Path: "widget/web/tunnel", Kind: "endpoint", State: "ready", URL: "https://example.invalid", Exposure: ExposurePublic, Provider: "cloudflare", TunnelIntent: "on", Lifecycle: "chosen", Answering: "yes"},
		{Path: "widget/@resources/database", Kind: "resources", State: "provisioned"},
	}
	before := append([]RuntimeItem(nil), live...)
	got := c.runtimeWithDeclarations(live)
	if !reflect.DeepEqual(got, before) {
		t.Fatalf("declarations overwrote live facts: %+v", got)
	}
}

func TestRuntimeDiscoveryUnsafeAndAmbiguousDefinitions(t *testing.T) {
	for _, name := range []string{"malformed", "escaping name", "duplicate YAML", "duplicate local", "symlink file", "symlink parent", "symlink local parent"} {
		t.Run(name, func(t *testing.T) {
			c := runtimeFixture(t)
			shared := runtimeSharedFragment(filepath.Join(c.Collection, "widget"))
			switch name {
			case "malformed":
				fixtureFile(t, shared, "tasks: [", 0644)
			case "escaping name":
				fixtureFile(t, shared, "tasks: {'../escape': {cmd: true}}\n", 0644)
			case "duplicate YAML":
				fixtureFile(t, shared, "tasks: {web: {cmd: true}, web: {cmd: false}}\n", 0644)
			case "duplicate local":
				fixtureFile(t, c.runtimeLocalFragment("widget"), "tasks: {web/server: {cmd: true}}\n", 0644)
			default:
				target := shared
				if name == "symlink parent" {
					target = filepath.Dir(shared)
				}
				if name == "symlink local parent" {
					target = filepath.Dir(c.runtimeLocalFragment("widget"))
				}
				if err := os.RemoveAll(target); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
					t.Fatal(err)
				}
				outside := t.TempDir()
				if name == "symlink file" {
					outside = filepath.Join(outside, "tasks.yaml")
					fixtureFile(t, outside, "tasks: {outside: {cmd: true}}\n", 0644)
				}
				if err := os.Symlink(outside, target); err != nil {
					t.Fatal(err)
				}
			}
			got := c.RuntimeEnlisted()
			want := 1
			// The local directory is shared by both repositories.
			if name == "symlink local parent" {
				want = 2
			}
			if len(got) != want {
				t.Fatalf("configuration diagnostics: %+v", got)
			}
			for _, item := range got {
				if item.Kind != "configuration" || item.State != "unknown" || item.Error == "" {
					t.Fatalf("unsafe definition presented as a service: %+v", got)
				}
			}
		})
	}
}

func TestRuntimeDiscoveryStatusMergesReadOnlyObservations(t *testing.T) {
	c := runtimeFixture(t)
	fixtureFile(t, c.Config.Runtime.Binary, `#!/bin/sh
shift 3
case "$*" in
 'runner status') echo '{"status":"running"}' ;;
 'ls') echo '{"tasks":[{"path":"existing/web/main","state":"failed","exit_code":17,"reason":"exit"},{"path":"existing/web/tunnel","state":"ready"}]}' ;;
 *) exit 99 ;;
esac
`, 0755)
	tasks := []runtimeTask{
		{RuntimeItem: RuntimeItem{Collection: filepath.Base(c.Collection), Dir: "existing", Repo: "existing", Path: "existing/web/main", Group: "existing/web", Kind: "service"}},
		{RuntimeItem: RuntimeItem{Collection: filepath.Base(c.Collection), Dir: "existing", Repo: "existing", Path: "existing/web/tunnel", Group: "existing/web", Kind: "endpoint", Provider: "cloudflare", URL: "https://example.invalid", Exposure: ExposurePublic, TunnelIntent: "on"}},
	}
	for i := range tasks {
		tasks[i].Log = filepath.Join(c.runtimeRoot(), "logs", filepath.FromSlash(tasks[i].Path)+".log")
	}
	m := runtimeManifest{Schema: 1, Root: c.runtimeRoot(), Binary: c.Config.Runtime.Binary, Tasks: tasks}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	fixtureFile(t, filepath.Join(c.runtimeRoot(), "manifest.json"), string(data), 0600)
	before := discoverySnapshot(t, c.Collection)
	got := c.RuntimeStatus()
	if len(got) != 4 || got[0].State != "failed" || got[0].ExitCode == nil || *got[0].ExitCode != 17 || got[0].Reason != "exit" || got[1].State != "ready" || got[1].URL != tasks[1].URL || got[1].TunnelIntent != "on" || got[2].State != "unconfigured" {
		t.Fatalf("lost status facts: %+v", got)
	}
	if after := discoverySnapshot(t, c.Collection); !reflect.DeepEqual(before, after) {
		t.Fatal("status wrote collection files")
	}
}

func TestRuntimeDiscoverySameNameInDifferentRepositories(t *testing.T) {
	c := runtimeFixture(t)
	other := filepath.Join(c.Collection, "other")
	fixtureFile(t, runtimeSharedFragment(other), "tasks: {web/server: {cmd: true}}\n", 0644)
	fixtureGit(t, "-C", other, "init", "-q", "-b", "main")
	got := c.RuntimeEnlisted()
	paths := map[string]bool{}
	for _, item := range got {
		paths[item.Path] = true
	}
	if len(got) != 3 || !paths["other/web/server"] || !paths["widget/web/server"] {
		t.Fatalf("lost qualified names: %+v", got)
	}
}

func TestRuntimeDiscoveryIncludesHarnessDeclarations(t *testing.T) {
	c := runtimeFixture(t)
	fixtureFile(t, runtimeSharedFragment(c.Harness), "tasks: {tools/main: {cmd: true}}\n", 0644)
	fixtureFile(t, c.runtimeLocalFragment("harness"), "tasks: {worker/main: {cmd: true}}\n", 0600)
	for _, got := range [][]RuntimeItem{c.RuntimeEnlisted(), c.RuntimeStatus()} {
		paths := map[string]RuntimeItem{}
		for _, item := range got {
			paths[item.Path] = item
		}
		for _, path := range []string{"harness/tools/main", "harness/worker/main"} {
			item := paths[path]
			if item.Repo != "harness-example" || item.Dir != "harness" || item.State != "unconfigured" {
				t.Fatalf("harness declaration missing: %+v", got)
			}
		}
	}
	// A newly added harness task must also appear beside persisted observations.
	recorded := []RuntimeItem{{Path: "harness/tools/main", State: "ready", URL: "http://127.0.0.1:43100"}}
	got := c.runtimeWithDeclarations(recorded)
	if len(got) != 4 || !reflect.DeepEqual(got[0], recorded[0]) {
		t.Fatalf("lost harness observations or new tasks: %+v", got)
	}
}

// Exercise shared definitions as tracked repository content, without real hooks.
func discoveryCommitShared(t *testing.T, c *Context) {
	t.Helper()
	repo := filepath.Join(c.Collection, "widget")
	fixtureGit(t, "-C", repo, "-c", "core.hooksPath=/dev/null", "add", ".harness/dekit.tasks.yaml")
	fixtureGit(t, "-C", repo, "-c", "core.hooksPath=/dev/null", "-c", "user.name=Synthetic", "-c", "user.email=synthetic@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "Declare synthetic services")
}

func TestRuntimeDiscoveryWorkspaceSweep(t *testing.T) {
	for _, scenario := range []string{"fresh", "manifest error", "existing", "unavailable binary"} {
		t.Run(scenario, func(t *testing.T) {
			c := runtimeFixture(t)
			discoveryCommitShared(t, c)
			marker := filepath.Join(c.Collection, "forbidden")
			fixtureFile(t, filepath.Join(c.Collection, ".env.collection.local"), "touch "+marker+"\n", 0600)
			fixtureFile(t, c.Config.Runtime.Binary, `#!/bin/sh
if [ "$1" = --version ]; then echo 'dekit 0.10.0'; exit; fi
shift 3
case "$*" in
 'runner status') echo '{"status":"running"}' ;;
 'ls') echo '{"tasks":[{"path":"existing/web/main","state":"failed","exit_code":17},{"path":"existing/web/tunnel","state":"ready"}]}' ;;
 *) exit 99 ;;
esac
`, 0755)
			switch scenario {
			case "manifest error":
				fixtureFile(t, filepath.Join(c.runtimeRoot(), "manifest.json"), "{", 0600)
			case "existing", "unavailable binary":
				// The sweep must never execute the foreign manifest's binary.
				poison := filepath.Join(t.TempDir(), "foreign-runtime")
				fixtureFile(t, poison, "#!/bin/sh\ntouch "+marker+"\nexit 99\n", 0755)
				tasks := []runtimeTask{
					{RuntimeItem: RuntimeItem{Collection: filepath.Base(c.Collection), Dir: "existing", Repo: "existing", Path: "existing/web/main", Group: "existing/web", Kind: "service"}},
					{RuntimeItem: RuntimeItem{Collection: filepath.Base(c.Collection), Dir: "existing", Repo: "existing", Path: "existing/web/tunnel", Group: "existing/web", Kind: "endpoint", URL: "https://example.invalid", Provider: "cloudflare", Exposure: ExposurePublic, TunnelIntent: "on"}},
				}
				for i := range tasks {
					tasks[i].Log = filepath.Join(c.runtimeRoot(), "logs", filepath.FromSlash(tasks[i].Path)+".log")
				}
				m := runtimeManifest{Schema: 1, Root: c.runtimeRoot(), Binary: poison, Tasks: tasks, Resources: []runtimeResource{{Dir: "existing", Repo: "existing", State: "provision-hook-completed"}}}
				data, err := json.Marshal(m)
				if err != nil {
					t.Fatal(err)
				}
				fixtureFile(t, filepath.Join(c.runtimeRoot(), "manifest.json"), string(data), 0600)
				if scenario == "unavailable binary" {
					c.Config.Runtime.Binary = filepath.Join(t.TempDir(), "missing")
				}
			}
			before := discoverySnapshot(t, c.Collection)
			var snapshot StatusSnapshot
			c.statusRuntime(&snapshot, true)
			byPath := map[string]RuntimeItem{}
			for _, item := range snapshot.Runtime {
				byPath[item.Path] = item
			}
			for _, path := range []string{"widget/web/server", "widget/web/tunnel"} {
				if byPath[path].State != "unconfigured" {
					t.Fatalf("sweep omitted declaration: %+v", snapshot.Runtime)
				}
			}
			if scenario == "manifest error" && (byPath["runtime"].State != "unknown" || byPath["runtime"].Error == "") {
				t.Fatalf("lost manifest diagnostic: %+v", snapshot.Runtime)
			}
			if scenario == "existing" || scenario == "unavailable binary" {
				main, tunnel, resource := byPath["existing/web/main"], byPath["existing/web/tunnel"], byPath["existing/@resources"]
				if tunnel.URL != "https://example.invalid" || tunnel.TunnelIntent != "on" || resource.State != "provision-hook-completed" {
					t.Fatalf("lost persisted facts: %+v", snapshot.Runtime)
				}
				if scenario == "existing" && (main.State != "failed" || main.ExitCode == nil || *main.ExitCode != 17 || tunnel.State != "ready") {
					t.Fatalf("lost live facts: %+v", snapshot.Runtime)
				}
				if scenario == "unavailable binary" && (main.State != "unknown" || main.Error == "") {
					t.Fatalf("lost binary diagnostic: %+v", snapshot.Runtime)
				}
			}
			if after := discoverySnapshot(t, c.Collection); !reflect.DeepEqual(before, after) {
				t.Fatal("sweep executed foreign code or wrote collection files")
			}
		})
	}
}

func TestRuntimeDiscoveryLinkedRepository(t *testing.T) {
	for _, scenario := range []string{"shared", "local only", "absent", "existing manifest", "escaping definition directory"} {
		t.Run(scenario, func(t *testing.T) {
			c := runtimeFixture(t)
			discoveryCommitShared(t, c)
			link := filepath.Join(c.Collection, "widget")
			checkout := filepath.Join(t.TempDir(), "checkout")
			if err := os.Rename(link, checkout); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(checkout, link); err != nil {
				t.Fatal(err)
			}
			if scenario == "local only" || scenario == "absent" {
				if err := os.Remove(runtimeSharedFragment(checkout)); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "local only" || scenario == "existing manifest" {
				fixtureFile(t, c.runtimeLocalFragment("widget"), "tasks: {worker/main: {cmd: true}}\n", 0600)
			}
			if scenario == "escaping definition directory" {
				target := filepath.Join(checkout, ".harness")
				if err := os.RemoveAll(target); err != nil {
					t.Fatal(err)
				}
				outside := t.TempDir()
				fixtureFile(t, filepath.Join(outside, "dekit.tasks.yaml"), "tasks: {outside: {cmd: true}}\n", 0644)
				if err := os.Symlink(outside, target); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "existing manifest" {
				fixtureFile(t, c.Config.Runtime.Binary, "#!/bin/sh\necho '{\"status\":\"absent\"}'\n", 0755)
				task := runtimeTask{RuntimeItem: RuntimeItem{Collection: filepath.Base(c.Collection), Dir: "widget", Repo: "widget", Path: "widget/web/server", Group: "widget/web", Kind: "service", URL: "http://127.0.0.1:43100"}, Log: filepath.Join(c.runtimeRoot(), "logs/widget/web/server.log")}
				m := runtimeManifest{Schema: 1, Root: c.runtimeRoot(), Binary: c.Config.Runtime.Binary, Tasks: []runtimeTask{task}}
				data, err := json.Marshal(m)
				if err != nil {
					t.Fatal(err)
				}
				fixtureFile(t, filepath.Join(c.runtimeRoot(), "manifest.json"), string(data), 0600)
			}
			beforeCollection, beforeCheckout := discoverySnapshot(t, c.Collection), discoverySnapshot(t, checkout)
			for _, got := range [][]RuntimeItem{c.RuntimeEnlisted(), c.RuntimeStatus()} {
				if scenario == "escaping definition directory" {
					if len(got) != 1 || got[0].Kind != "configuration" || got[0].State != "unknown" {
						t.Fatalf("accepted escaping definition: %+v", got)
					}
					continue
				}
				want := 2
				if scenario == "local only" {
					want = 1
				}
				if scenario == "absent" {
					want = 0
				}
				if scenario == "existing manifest" {
					want = 3
				}
				if len(got) != want {
					t.Fatalf("linked repository discovery: %+v", got)
				}
				for _, item := range got {
					if scenario == "existing manifest" && item.Path == "widget/web/server" {
						if item.URL != "http://127.0.0.1:43100" {
							t.Fatalf("lost recorded URL: %+v", item)
						}
					} else if item.State != "unconfigured" {
						t.Fatalf("linked declaration: %+v", item)
					}
				}
			}
			if !reflect.DeepEqual(beforeCollection, discoverySnapshot(t, c.Collection)) || !reflect.DeepEqual(beforeCheckout, discoverySnapshot(t, checkout)) {
				t.Fatal("linked discovery changed collection or checkout files")
			}
		})
	}
}
