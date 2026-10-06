package wtc

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Explicitly enabled: creates one disposable Postgres container, without host
// ports, and two databases owned by separate synthetic collections. It does not
// inspect or modify existing containers. The caller supplies a cached image.
func TestDekitSharedDockerPostgres(t *testing.T) {
	image, binary := os.Getenv("WTC_TEST_DOCKER_IMAGE"), os.Getenv("WTC_TEST_DEKIT")
	if image == "" || binary == "" {
		runtimeToolsSkip(t, "set WTC_TEST_DOCKER_IMAGE and WTC_TEST_DEKIT for the isolated Docker trial")
	}
	docker, err := exec.LookPath("docker")
	if err != nil {
		t.Fatal(err)
	}
	binary, err = filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("wtc-dekit-fixture-%d-%d", os.Getpid(), time.Now().UnixNano())
	call := func(args ...string) string {
		t.Helper()
		out, err := exec.Command(docker, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("docker %v: %s: %v", args, out, err)
		}
		return strings.TrimSpace(string(out))
	}
	// Registered first, so each collection's cleanup runs before shared removal.
	t.Cleanup(func() {
		out, err := exec.Command(docker, "rm", "-fv", name).CombinedOutput()
		if err != nil && !strings.Contains(string(out), "No such container") {
			t.Errorf("remove fixture container: %s %v", out, err)
		}
	})
	call("run", "-d", "--name", name, "--label", "wtc.synthetic-runtime-test=true", "--tmpfs", "/var/lib/postgresql/data:rw,size=192m", "-e", "POSTGRES_PASSWORD=fixture-only", image)
	deadline := time.Now().Add(45 * time.Second)
	for {
		if err := exec.Command(docker, "exec", name, "pg_isready", "-h", "127.0.0.1", "-U", "postgres").Run(); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("fixture Postgres did not become ready")
		}
		time.Sleep(200 * time.Millisecond)
	}
	owner := newWorkspaceFixture(t)
	var collections []*Context
	for _, slug := range []string{"pg-one", "pg-two"} {
		result, err := owner.NewCollection(NewOptions{Slug: slug, Repos: []string{"widget"}})
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
				t.Errorf("resource cleanup: %v", err)
			}
		})
		db := strings.ReplaceAll(slug, "-", "_")
		fixtureFile(t, filepath.Join(c.Collection, ".env.collection.local"), fmt.Sprintf("FIXTURE_DOCKER='%s'\nFIXTURE_CONTAINER='%s'\nFIXTURE_DB='%s'\n", docker, name, db), 0600)
		app := filepath.Join(c.Collection, "widget/.harness")
		fixtureFile(t, filepath.Join(app, "runtime-provision.sh"), `#!/bin/sh
set -eu
: "${WTC_COLLECTION:?}"
case "$FIXTURE_DB" in pg_one|pg_two) ;; *) exit 8;; esac
exists=$("$FIXTURE_DOCKER" exec "$FIXTURE_CONTAINER" psql -U postgres -tAc "SELECT 1 FROM pg_database WHERE datname='$FIXTURE_DB'")
[ "$exists" = 1 ] || "$FIXTURE_DOCKER" exec "$FIXTURE_CONTAINER" createdb -U postgres "$FIXTURE_DB"
`, 0755)
		fixtureFile(t, filepath.Join(app, "runtime-teardown.sh"), `#!/bin/sh
set -eu
: "${WTC_COLLECTION:?}"
case "$FIXTURE_DB" in pg_one|pg_two) ;; *) exit 8;; esac
"$FIXTURE_DOCKER" exec "$FIXTURE_CONTAINER" dropdb -U postgres --if-exists "$FIXTURE_DB"
`, 0755)
		fragment := fmt.Sprintf(`tasks:
 db/logs:
  label: shared Postgres log observer
  cmd: [%q, logs, --follow, %q]
  tags: [accessory]
  autostart: true
  ready: {log: 'database system is ready to accept connections', timeout: 10s}
`, docker, name)
		fixtureFile(t, filepath.Join(app, "dekit.tasks.yaml"), fragment, 0644)
		if _, err := c.RuntimeAction("up", "", 10*time.Second); err != nil {
			t.Fatal(err)
		}
	}
	databases := func() string {
		return call("exec", name, "psql", "-U", "postgres", "-tAc", "SELECT datname FROM pg_database WHERE datname IN ('pg_one','pg_two') ORDER BY datname")
	}
	if got := databases(); got != "pg_one\npg_two" {
		t.Fatal("both resources were not provisioned", got)
	}
	if _, err := collections[0].RuntimeAction("down", "", 0); err != nil {
		t.Fatal(err)
	}
	if got := databases(); got != "pg_one\npg_two" {
		t.Fatal("down destroyed databases", got)
	}
	if _, err := owner.RetireCollection(RetireOptions{Name: "pg-one", Force: true}); err != nil {
		t.Fatal(err)
	}
	if got := databases(); got != "pg_two" {
		t.Fatal("retirement did not preserve other collection database", got)
	}
	if got := call("inspect", "--format", "{{.State.Running}}", name); got != "true" {
		t.Fatal("stopped shared server", got)
	}
	if items := collections[1].RuntimeStatus(); len(items) != 2 || items[0].State != "ready" {
		t.Fatal("other observer stopped", items)
	}
	if _, err := owner.RetireCollection(RetireOptions{Name: "pg-two", Force: true}); err != nil {
		t.Fatal(err)
	}
	if got := databases(); got != "" {
		t.Fatal("owned databases leaked", got)
	}
	if got := call("inspect", "--format", "{{.State.Running}}", name); got != "true" {
		t.Fatal("stopped shared server", got)
	}
}
