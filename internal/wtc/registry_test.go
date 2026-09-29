package wtc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRefreshRegistryReportsMissingAndUnlistedOwners(t *testing.T) {
	c := fixture(t)
	for _, name := range []string{"widget.git", "archive.git"} {
		if err := os.MkdirAll(filepath.Join(c.Workspace, ".bare", name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	report, err := c.RefreshRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Missing) != 1 || report.Missing[0] != "harness-example" || len(report.Unlisted) != 1 || report.Unlisted[0] != "archive" {
		t.Fatalf("unexpected cross-check: %+v", report)
	}
	data, err := os.ReadFile(report.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "widget="+filepath.Join(c.Workspace, ".bare", "widget.git")) {
		t.Fatalf("missing local mapping: %s", data)
	}
}

func TestRefreshRegistryRejectsUnsafeBareOwner(t *testing.T) {
	c := fixture(t)
	if err := os.MkdirAll(filepath.Join(c.Workspace, ".bare", "bad=name.git"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := c.RefreshRegistry(); err == nil {
		t.Fatal("accepted a bare owner that corrupts the generated map")
	}
}

func TestRefreshRegistryIgnoresSymlinkedOwner(t *testing.T) {
	c := fixture(t)
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(c.Workspace, ".bare"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(c.Workspace, ".bare", "linked.git")); err != nil {
		t.Fatal(err)
	}
	report, err := c.RefreshRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if len(report.BareOwners) != 0 {
		t.Fatalf("included symlinked owner: %v", report.BareOwners)
	}
}
