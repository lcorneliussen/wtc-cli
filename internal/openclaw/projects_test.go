package openclaw

import "testing"

func TestProjectDestinationUsesExistingRecordWithoutWriting(t *testing.T) {
	checkout, git := managedWorktree(t, "https://github.com/example/api.git")
	git("update-ref", "refs/remotes/origin/develop", "HEAD")
	if got := ProjectDestination(checkout.Dir, "42-new"); got != "main" {
		t.Fatalf("default: %q", got)
	}
	if err := RecordBase(checkout.Dir, "42-new", "develop"); err != nil {
		t.Fatal(err)
	}
	if got := ProjectDestination(checkout.Dir, "42-new"); got != "develop" {
		t.Fatalf("recorded: %q", got)
	}
	if got := ProjectDestination(checkout.Dir, ""); got != "main" {
		t.Fatalf("new collection default: %q", got)
	}
	if got := RecordedBase(checkout.Dir, "42-new"); got != "develop" {
		t.Fatalf("metadata changed record: %q", got)
	}
}
