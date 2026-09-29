package wtc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T) *Context {
	t.Helper()
	root := t.TempDir()
	collection := filepath.Join(root, "demo")
	harness := filepath.Join(collection, "harness")
	if err := os.MkdirAll(harness, 0755); err != nil {
		t.Fatal(err)
	}
	registry := "repos:\n  - name: harness-example\n    remote: https://example.invalid/harness-example.git\n    default_ref: origin/main\n  - name: widget\n    remote: https://example.invalid/widget.git\n    default_ref: origin/main\n    port_offset: 2\n    x-deploy-workflow: release\n"
	if err := os.WriteFile(filepath.Join(harness, ".harness-repos.yml"), []byte(registry), 0644); err != nil {
		t.Fatal(err)
	}
	c, err := Discover(collection)
	if err != nil {
		t.Fatal(err)
	}
	c.ConfigRoot = filepath.Join(root, "config")
	return c
}
func TestRenderEnvPreservesPortBase(t *testing.T) {
	c := fixture(t)
	old := []byte("COLLECTION_PORT_BASE=43100\n")
	if err := os.WriteFile(filepath.Join(c.Collection, ".env.collection"), old, 0644); err != nil {
		t.Fatal(err)
	}
	got, err := c.RenderEnv()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"COLLECTION_PORT_BASE=43100", "WIDGET_PORT=43102", "WTC_COLLECTION=demo"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("missing %s in %s", want, got)
		}
	}
	if err := c.WriteEnv(got); err != nil {
		t.Fatal(err)
	}
	again, err := c.RenderEnv()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(again) {
		t.Fatal("regeneration moved a port or changed output")
	}
}
func TestLocalEnvSymlinkStopsGeneration(t *testing.T) {
	c := fixture(t)
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.Symlink(outside, filepath.Join(c.Collection, ".env.collection.local")); err != nil {
		t.Fatal(err)
	}
	if err := c.ValidateEnvSupport(); err == nil {
		t.Fatal("accepted symlinked local env")
	}
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Fatal("followed symlink")
	}
}
func TestEjectRefusesOverwrite(t *testing.T) {
	c := fixture(t)
	paths, err := Eject(c.Harness, []string{"instructions/publication-privacy.md"})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 {
		t.Fatalf("got %d files", len(paths))
	}
	if _, err := Eject(c.Harness, []string{"instructions/publication-privacy.md"}); err == nil {
		t.Fatal("overwrote ejected default")
	}
}

func TestEjectSkillGlob(t *testing.T) {
	c := fixture(t)
	paths, err := Eject(c.Harness, []string{"skills/wtc-*"})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) < 10 {
		t.Fatalf("unexpected paths: %v", paths)
	}
}
