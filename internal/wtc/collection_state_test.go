package wtc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestTabaPRRegistryReadsCarryForwardIntoLegacyWrites(t *testing.T) {
	c := &Context{Collection: t.TempDir()}
	alias := filepath.Join(c.Collection, ".taba-prs")
	initial := "# Preserve comments and final merge facts.\nwidget 7 done https://example.invalid/pull/7 Finished\n# merged-pr widget 7 2026-09-30T12:00:00Z SUCCESS\n"
	fixtureFile(t, alias, initial, 0644)
	records, err := c.ListPRs()
	if err != nil || len(records) != 1 || records[0].MergedOn == "" || c.PRFile() != alias {
		t.Fatalf("alternate registry was not read: %+v %v", records, err)
	}
	if _, err := os.Stat(filepath.Join(c.Collection, ".wtc-prs")); !os.IsNotExist(err) {
		t.Fatal("listing created legacy state")
	}
	if _, err := c.EnlistPR(PRRecord{Repo: "widget", Number: "8", Branch: "next", URL: "https://example.invalid/pull/8"}); err != nil {
		t.Fatal(err)
	}
	records, err = c.ListPRs()
	if err != nil || len(records) != 2 || records[0].Number != "7" || records[0].MergedOn == "" || filepath.Base(c.PRFile()) != ".wtc-prs" {
		t.Fatalf("legacy write lost alternate rows: %+v %v", records, err)
	}
	if data, err := os.ReadFile(alias); err != nil || string(data) != initial {
		t.Fatalf("alternate source was changed: %s %v", data, err)
	}
	if err := c.UnlistPR("widget", "8"); err != nil {
		t.Fatal(err)
	}
	records, err = c.ListPRs()
	if err != nil || len(records) != 1 || records[0].Number != "7" {
		t.Fatalf("legacy registry did not remain authoritative: %+v %v", records, err)
	}
}

func TestTabaPRMergeFactsCarryForwardIntoLegacyWrites(t *testing.T) {
	c := &Context{Collection: t.TempDir()}
	fixtureFile(t, filepath.Join(c.Collection, ".taba-prs"), "widget 7 done\n", 0644)
	records, err := c.ListPRs()
	if err != nil {
		t.Fatal(err)
	}
	count, err := c.recordMergedPRs(records, []statusPRDetail{{State: "MERGED", MergedOn: "2026-09-30T12:00:00Z", Checks: "SUCCESS"}})
	if err != nil || count != 1 {
		t.Fatalf("merge recording failed: %d %v", count, err)
	}
	records, err = c.ListPRs()
	if err != nil || len(records) != 1 || records[0].FinalChecks != "SUCCESS" || filepath.Base(c.PRFile()) != ".wtc-prs" {
		t.Fatalf("merge facts were lost: %+v %v", records, err)
	}
}

func TestTabaPRRegistryUsesLegacyLockAndReselectsAfterWaiting(t *testing.T) {
	c := &Context{Collection: t.TempDir()}
	fixtureFile(t, filepath.Join(c.Collection, ".taba-prs"), "widget 7 original\n", 0644)
	lock, err := os.OpenFile(filepath.Join(c.Collection, ".wtc-prs.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	defer unix.Flock(int(lock.Fd()), unix.LOCK_UN)
	done := make(chan error, 1)
	go func() {
		done <- c.rewritePRs("widget", "8", &PRRecord{Repo: "widget", Number: "8", Branch: "new"})
	}()
	select {
	case err := <-done:
		t.Fatalf("writer bypassed the legacy lock: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	// Simulate an older client writing while it owns the shared lock.
	fixtureFile(t, filepath.Join(c.Collection, ".wtc-prs"), "widget 7 original\nwidget 9 older-client\n", 0644)
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("writer did not finish after unlocking")
	}
	records, err := c.ListPRs()
	if err != nil || len(records) != 3 {
		t.Fatalf("writer used a stale alternate input: %+v %v", records, err)
	}
}

func TestTabaStateReadersKeepLegacyPrecedenceAndRejectBrokenInputs(t *testing.T) {
	for _, pair := range [][2]string{{".wtc-prs", ".taba-prs"}, {".wtc-status.json", ".taba-status.json"}, {".last-wtc-status.yml", ".last-taba-status.yml"}} {
		t.Run(pair[0], func(t *testing.T) {
			c := &Context{Collection: t.TempDir()}
			legacy, alias := filepath.Join(c.Collection, pair[0]), filepath.Join(c.Collection, pair[1])
			fixtureFile(t, alias, "alternate", 0644)
			fixtureFile(t, legacy, "legacy", 0644)
			if path, err := collectionStateInput(c.Collection, pair[0], pair[1]); err != nil || path != legacy {
				t.Fatalf("legacy lost precedence: %q %v", path, err)
			}
			if err := os.Remove(legacy); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("missing", legacy); err != nil {
				t.Fatal(err)
			}
			if _, err := collectionStateInput(c.Collection, pair[0], pair[1]); err == nil {
				t.Fatal("broken legacy input was bypassed")
			}
			if pair[0] == ".wtc-prs" {
				if _, err := c.ListPRs(); err == nil {
					t.Fatal("registry read hid a broken legacy input")
				}
				if err := c.UnlistPR("widget", "7"); err == nil {
					t.Fatal("registry mutation hid a broken legacy input")
				}
			}
			if err := os.Remove(legacy); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(legacy, 0755); err != nil {
				t.Fatal(err)
			}
			if _, err := collectionStateInput(c.Collection, pair[0], pair[1]); err == nil {
				t.Fatal("directory was accepted as a state file")
			}
		})
	}
}

func TestTabaStatusCachesReadAlternateButWriteLegacy(t *testing.T) {
	c := &Context{Collection: t.TempDir()}
	snapshot := StatusSnapshot{Schema: 1, Collection: filepath.Base(c.Collection), GeneratedAt: "2026-10-27T00:00:00Z"}
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(c.Collection, ".taba-status.json")
	fixtureFile(t, alias, string(data), 0644)
	old := time.Now().Add(-2 * time.Minute)
	if err := os.Chtimes(alias, old, old); err != nil {
		t.Fatal(err)
	}
	got, err := c.ReadStatusSnapshot()
	if err != nil || got.GeneratedAt != snapshot.GeneratedAt {
		t.Fatalf("alternate snapshot was not read: %+v %v", got, err)
	}
	if age, err := c.CachedStatusAge(); err != nil || age < 120 {
		t.Fatalf("alternate snapshot age was not read: %d %v", age, err)
	}
	if _, err := os.Stat(filepath.Join(c.Collection, ".wtc-status.json")); !os.IsNotExist(err) {
		t.Fatal("reading created legacy state")
	}
	snapshot.GeneratedAt = "2026-10-27T00:01:00Z"
	if err := c.WriteStatusSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	got, err = c.ReadStatusSnapshot()
	if err != nil || got.GeneratedAt != snapshot.GeneratedAt {
		t.Fatalf("legacy write did not become authoritative: %+v %v", got, err)
	}
	if preserved, err := os.ReadFile(alias); err != nil || string(preserved) != string(data) {
		t.Fatal("alternate cache was overwritten")
	}
	for _, name := range []string{".wtc-status.md", ".last-wtc-status.yml"} {
		if _, err := os.Stat(filepath.Join(c.Collection, name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(c.Collection, ".wtc-status.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(c.Collection, ".last-wtc-status.yml"), filepath.Join(c.Collection, ".last-taba-status.yml")); err != nil {
		t.Fatal(err)
	}
	text, err := c.LegacyStatusText()
	if err != nil || !strings.Contains(text, ".last-taba-status.yml") {
		t.Fatalf("alternate YAML fallback was not read: %s %v", text, err)
	}
}
