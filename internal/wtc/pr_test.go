package wtc

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
)

func TestPRIdentityStaysInCollection(t *testing.T) {
	for _, repo := range []string{"..", "../outside", "/tmp/outside", "nested/repo", "foo=bar", "foo\nbar"} {
		if err := ValidatePRIdentity(repo, "7"); err == nil {
			t.Fatalf("accepted repository path %q", repo)
		}
	}
	if err := ValidatePRIdentity("ext.wtc-boilerplate", "50"); err != nil {
		t.Fatal(err)
	}
}

func TestPRFileStaysReadableByShellTools(t *testing.T) {
	c := fixture(t)
	first, err := c.EnlistPR(PRRecord{Repo: "widget", Number: "42", Branch: "feature", Title: "A title with spaces"})
	if err != nil {
		t.Fatal(err)
	}
	if first.URL != "" { // Unknown forge: a clickable guess would be misleading.
		t.Fatalf("inferred unknown forge URL: %s", first.URL)
	}
	if _, err := c.EnlistPR(PRRecord{Repo: "widget", Number: "42", Branch: "feature", URL: "https://example.invalid/42", Title: "Updated title"}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.EnlistPR(PRRecord{Repo: "widget", Number: "43", Branch: "other"}); err != nil {
		t.Fatal(err)
	}
	records, err := c.ListPRs()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].Title != "Updated title" || records[1].Number != "43" {
		t.Fatalf("unexpected records: %+v", records)
	}
	data, err := os.ReadFile(c.PRFile())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), "widget 42") != 1 || !strings.Contains(string(data), "widget 42 feature https://example.invalid/42 Updated title") {
		t.Fatalf("incorrect shared file format: %s", data)
	}
	if err := c.UnlistPR("widget", "42"); err != nil {
		t.Fatal(err)
	}
	records, err = c.ListPRs()
	if err != nil || len(records) != 1 || records[0].Number != "43" {
		t.Fatalf("unlist failed: %+v, %v", records, err)
	}
}

func TestPRURLOnlyForKnownForges(t *testing.T) {
	for _, tc := range []struct{ remote, want string }{
		{"git@github.com:example/widget.git", "https://github.com/example/widget/pull/7"},
		{"https://bitbucket.org/example/widget.git", "https://bitbucket.org/example/widget/pull-requests/7"},
		{"https://example.invalid/example/widget.git", ""},
	} {
		if got := prURL(tc.remote, "7"); got != tc.want {
			t.Errorf("prURL(%q) = %q, want %q", tc.remote, got, tc.want)
		}
	}
}

func TestConcurrentPRRegistryUpdatesPreserveRowsAndMergeFacts(t *testing.T) {
	c := fixture(t)
	if _, err := c.EnlistPR(PRRecord{Repo: "widget", Number: "7", Branch: "done", URL: "https://example.invalid/pull/7"}); err != nil {
		t.Fatal(err)
	}
	records, err := c.ListPRs()
	if err != nil {
		t.Fatal(err)
	}
	const additions = 32
	start := make(chan struct{})
	errors := make(chan error, additions+1)
	var workers sync.WaitGroup
	workers.Add(additions + 1)
	go func() {
		defer workers.Done()
		<-start
		if count, err := c.recordMergedPRs(records, []statusPRDetail{{Number: "7", State: "MERGED", MergedOn: "2026-09-30T12:00:00Z", Checks: "SUCCESS"}}); err != nil || count != 1 {
			errors <- fmt.Errorf("record merge: count=%d err=%v", count, err)
		}
	}()
	for number := 8; number < 8+additions; number++ {
		number := number
		go func() {
			defer workers.Done()
			<-start
			id := fmt.Sprint(number)
			_, err := c.EnlistPR(PRRecord{Repo: "widget", Number: id, Branch: "next", URL: "https://example.invalid/pull/" + id})
			if err != nil {
				errors <- err
			}
		}()
	}
	close(start)
	workers.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
	records, err = c.ListPRs()
	if err != nil || len(records) != additions+1 {
		t.Fatalf("concurrent registry rewrite lost records: %d %v", len(records), err)
	}
	merged := false
	for _, record := range records {
		if record.Number == "7" {
			merged = record.MergedOn == "2026-09-30T12:00:00Z" && record.FinalChecks == "SUCCESS"
		}
	}
	if !merged {
		t.Fatal("concurrent enlistment lost final merge facts")
	}
}
