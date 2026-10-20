package wtc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLinkGatewayCollection(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	work := t.TempDir()
	first, second := filepath.Join(work, "wt-a"), filepath.Join(work, "wt-b")
	for _, d := range []string{first, second} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	dir := filepath.Join(t.TempDir(), "wtc-collections", "demo")
	repo := LinkedRepo{Name: "api", Remote: "https://github.com/example-org/api.git", DefaultRef: "origin/main", Path: first}
	c, err := LinkGatewayCollection(dir, []LinkedRepo{repo})
	if err != nil {
		t.Fatal(err)
	}
	if !c.AdHoc || c.Config.Runtime.Backend != "dekit" {
		t.Fatalf("ad-hoc %v, runtime %q", c.AdHoc, c.Config.Runtime.Backend)
	}
	if target, _ := os.Readlink(filepath.Join(dir, "api")); target != first {
		t.Fatalf("link points to %q", target)
	}
	// A worktree that moved is linked again; the registry keeps one entry.
	repo.Path = second
	c, err = LinkGatewayCollection(dir, []LinkedRepo{repo})
	if err != nil {
		t.Fatal(err)
	}
	if target, _ := os.Readlink(filepath.Join(dir, "api")); target != second {
		t.Fatalf("link not moved: %q", target)
	}
	if n := len(c.Registry.Repos); n != 1 {
		t.Fatalf("%d registry entries", n)
	}
	// Anything that is not a link stays.
	if err := os.Mkdir(filepath.Join(dir, "web"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := LinkGatewayCollection(dir, []LinkedRepo{{Name: "web", Remote: "https://github.com/example-org/web.git", DefaultRef: "origin/main", Path: first}}); err == nil {
		t.Fatal("replaced a directory")
	}
}

// A harness repository called harness is a link like any other: the folder
// stays the gateway's collection and keeps its recorded harness.
func TestGatewayFolderWithAHarnessCalledHarness(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	wt := t.TempDir()
	if err := os.WriteFile(filepath.Join(wt, ".harness-repos.yml"), []byte("repos:\n  - name: api\n    remote: https://github.com/example-org/api.git\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "wtc-collections", "demo")
	if err := RecordGatewayHarness(dir, "platform", "https://github.com/example-org/harness.git"); err != nil {
		t.Fatal(err)
	}
	c, err := LinkGatewayCollection(dir, []LinkedRepo{{Name: "harness", Remote: "https://github.com/example-org/harness.git", DefaultRef: "origin/main", Path: wt}})
	if err != nil {
		t.Fatal(err)
	}
	if target, _ := os.Readlink(filepath.Join(dir, "harness")); target != wt {
		t.Errorf("harness links to %q", target)
	}
	if !c.AdHoc || c.Config.Runtime.Backend != "dekit" {
		t.Errorf("no longer the gateway's folder: ad hoc %v, runtime %q", c.AdHoc, c.Config.Runtime.Backend)
	}
	if name, _, err := GatewayHarness(dir); err != nil || name != "platform" {
		t.Errorf("recorded harness %q %v", name, err)
	}
	if err := RecordGatewayHarness(dir, "ops", "https://github.com/example-org/ops.git"); err == nil {
		t.Error("a second harness was recorded")
	}
}
