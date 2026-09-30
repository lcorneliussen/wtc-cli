package wtc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestStatusSnapshotCachesCanonicalAndLegacyFormats(t *testing.T) {
	c := newWorkspaceFixture(t)
	snapshot, err := c.StatusLocalSnapshot(false)
	if err != nil {
		t.Fatal(err)
	}
	for i := range snapshot.Repos {
		if snapshot.Repos[i].Dir == "harness" {
			snapshot.Repos[i].PR = &StatusPRFacts{Number: "7", Checks: "SUCCESS", Review: "approved"}
		}
	}
	if err := c.WriteStatusSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	read, err := c.ReadStatusSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if read.Schema != 1 || read.Collection != "main" || len(read.Repos) != len(snapshot.Repos) {
		t.Fatalf("canonical snapshot changed: %+v", read)
	}
	md, err := os.ReadFile(filepath.Join(c.Collection, ".wtc-status.md"))
	if err != nil || !strings.Contains(string(md), "## Repos") || !strings.Contains(string(md), "#7 ✓") {
		t.Fatalf("Markdown cache missing PR fact: %s %v", md, err)
	}
	legacyBytes, err := os.ReadFile(filepath.Join(c.Collection, ".last-wtc-status.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var legacy statusLegacyCache
	if err := yaml.Unmarshal(legacyBytes, &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.Collection != "main" || len(legacy.Repos) != len(snapshot.Repos) || legacy.Repos[0].Repo != "harness" || legacy.Repos[0].PR != "7" {
		t.Fatalf("legacy cache changed: %+v", legacy)
	}
	if !strings.Contains(string(legacyBytes), "    state: 'OPEN'\n") || !strings.Contains(string(legacyBytes), "    title: ''\n") {
		t.Fatalf("legacy cache is not in the shell reader's quoted subset: %s", legacyBytes)
	}
	if got := statusLegacyQuote(" O'Brien\nwidget "); got != "'O''Brien widget'" {
		t.Fatalf("wrong legacy quote: %q", got)
	}
	if age, err := c.CachedStatusAge(); err != nil || age < 0 {
		t.Fatalf("wrong cache age: %d %v", age, err)
	}
	if err := os.Remove(filepath.Join(c.Collection, ".wtc-status.json")); err != nil {
		t.Fatal(err)
	}
	plain, err := c.LegacyStatusText()
	if err != nil || !strings.Contains(plain, "no .wtc-status.json") || !strings.Contains(plain, "harness") || !strings.Contains(plain, "#7") {
		t.Fatalf("legacy fallback failed: %s %v", plain, err)
	}
}

func TestStatusSnapshotCacheRejectsOtherCollection(t *testing.T) {
	c := newWorkspaceFixture(t)
	snapshot, err := c.StatusLocalSnapshot(false)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Collection = "other"
	if err := c.WriteStatusSnapshot(snapshot); err == nil {
		t.Fatal("wrote a snapshot for another collection")
	}
	if _, err := os.Stat(filepath.Join(c.Collection, ".wtc-status.json")); !os.IsNotExist(err) {
		t.Fatalf("unexpected cache file: %v", err)
	}
}
