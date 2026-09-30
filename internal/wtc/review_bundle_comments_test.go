package wtc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublicReviewCommentsKeepRepliesAfterLatestStatusAndAllInlineKeys(t *testing.T) {
	conversation := []byte(`{"comments":[
		{"author":{"login":"reviewer"},"createdAt":"2026-01-01T10:00:00Z","body":"**Local review: pass**\n\n\u0060wtc-review v1 head=aaaaaaaaaaaa verdict=pass blockers=0 round=1\u0060"},
		{"author":{"login":"reviewer"},"createdAt":"2026-01-01T12:00:00Z","body":"**Local review: in progress**\n\n\u0060wtc-review v1 head=bbbbbbbbbbbb verdict=pending blockers=0 round=2\u0060"},
		{"author":{"login":"author"},"createdAt":"2026-01-01T13:00:00Z","body":"Fixed the fallback assertion.\n> \u0060wtc-review v1 head=aaaaaaaaaaaa verdict=pass blockers=0 round=1\u0060"}
	]}`)
	inline := []byte(`[[
		{"user":{"login":"reviewer"},"created_at":"2026-01-01T09:00:00Z","body":"wtc-review-inline v1 key=012345abcd"},
		{"user":{"login":"author"},"created_at":"2026-01-01T14:00:00Z","body":"The updated test passes. wtc-review-inline v1 key=def567abcd"}
	]]`)
	comments, err := parseBundleComments(conversation, inline)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prior := filepath.Join(dir, "prior")
	if err := os.MkdirAll(prior, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(prior, "inline-keys.txt"), []byte("aaa555bbbb\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := writeBundleComments(dir, comments); err != nil {
		t.Fatal(err)
	}
	transcript, err := os.ReadFile(filepath.Join(prior, "comments.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(transcript), "Fixed the fallback assertion") || !strings.Contains(string(transcript), "The updated test passes") || strings.Contains(string(transcript), "012345abcd") || strings.Contains(string(transcript), "**Local review: in progress**") || strings.Index(string(transcript), "Fixed the fallback assertion") > strings.Index(string(transcript), "The updated test passes") {
		t.Fatalf("incorrect reply transcript: %s", transcript)
	}
	keys, err := os.ReadFile(filepath.Join(prior, "inline-keys.txt"))
	if err != nil || string(keys) != "012345abcd\naaa555bbbb\ndef567abcd\n" {
		t.Fatalf("prior inline keys lost: %s %v", keys, err)
	}
}
