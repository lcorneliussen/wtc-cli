package wtc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReviewStatusUsesNewestCommentAndRequiresPostingReceipt(t *testing.T) {
	collection := t.TempDir()
	head := "1234567890abcdef1234567890abcdef12345678"
	comments := []byte(`{"comments":[
    {"body":"wtc-review v1 head=1234567890abcdef1234567890abcdef12345678 verdict=pass blockers=0 round=1", "createdAt":"2026-01-01T00:00:00Z", "url":"https://github.com/example/repo/pull/7#issuecomment-11"},
    {"body":"progress: wtc-review v1 head=1234567890abcdef1234567890abcdef12345678 verdict=pending blockers=0 round=2", "createdAt":"2026-01-02T00:00:00Z", "url":"https://github.com/example/repo/pull/7#issuecomment-12"}
  ]}`)
	got, err := ParseReviewStatusComments(comments, 7, head, "github", "example/repo", collection, true)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != "untrusted" || got.Verdict != "pending" || got.CommentID != "12" {
		t.Fatalf("newest untrusted pending comment did not close gate: %+v", got)
	}
	receipt := reviewReceiptPath(collection, "github", "example/repo", "7", head, "12", "pending")
	if err := os.MkdirAll(filepath.Dir(receipt), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(receipt, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	got, err = ParseReviewStatusComments(comments, 7, head, "github", "example/repo", collection, true)
	if err != nil || got.State != "current" || got.Verdict != "pending" {
		t.Fatalf("trusted pending review misread: %+v %v", got, err)
	}
}

func TestReviewStatusRejectsReversedOrShortHeadPrefix(t *testing.T) {
	if SameReviewSHA("1234567890ab", "1234567890abcdef") || SameReviewSHA("1234567890abcdef", "1234567") {
		t.Fatal("accepted reversed or too-short PR head")
	}
	if !SameReviewSHA("1234567890abcdef", "1234567890ab") {
		t.Fatal("rejected valid forge head prefix")
	}
}
