package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runWTC(t *testing.T, args ...string) error {
	t.Helper()
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = append([]string{"wtc", "--json"}, args...)
	return run()
}

func TestInitThenRetireInPlaceOutsideHerdr(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PATH", "/usr/bin:/bin")
	t.Setenv("WTC_CONFIG_ROOT", "")
	t.Setenv("WTC_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	dir := filepath.Join(root, "task")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	if err := runWTC(t, "init"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".wtc", "config", "repos.yml")); err != nil {
		t.Fatalf("init wrote no local registry: %v", err)
	}
	if err := runWTC(t, "skills", "render", "--all"); err == nil || !strings.Contains(err.Error(), "ad-hoc") {
		t.Fatalf("skills render --all swept an ad-hoc parent: %v", err)
	}
	// Inside Herdr the usual handoff applies; outside it retires in place.
	t.Setenv("HERDR_ENV", "1")
	t.Setenv("HERDR_SESSION", "")
	if err := runWTC(t, "retire", "."); err == nil || !strings.Contains(err.Error(), "Herdr pane") {
		t.Fatalf("inside Herdr, self-retire should hand off: %v", err)
	}
	t.Setenv("HERDR_ENV", "")
	if err := runWTC(t, "retire", "."); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("collection folder remains: %v", err)
	}
}
