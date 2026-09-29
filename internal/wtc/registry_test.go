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
