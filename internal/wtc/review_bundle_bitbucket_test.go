package wtc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBitbucketReviewCommentsKeepRepliesAfterLatestReview(t *testing.T) {
	// The CLI returns an object with a comments array, and Bitbucket's REST
	// fields use content.raw and created_on rather than GitHub's body/date.
	raw := []byte(`{"comments":[
		{"content":{"raw":"older discussion"},"created_on":"2026-01-01T09:00:00Z","user":{"display_name":"Older"}},
		{"content":{"raw":"**Local review: pass**\n\n` + "`" + `wtc-review v1 head=0123456789abcdef verdict=pass blockers=0 round=1` + "`" + `"},"created_on":"2026-01-01T10:00:00Z","user":{"display_name":"Reviewer"}},
		{"content":{"raw":"**major** · Finding\n\n` + "`" + `wtc-review-inline v1 key=012345abcd` + "`" + `"},"created_on":"2026-01-01T10:01:00Z","user":{"display_name":"Reviewer"}},
		{"content":{"raw":"A later reply"},"created_on":"2026-01-01T11:00:00Z","user":{"display_name":"Author"}}
	]}`)
	comments, err := parseBitbucketBundleComments(raw)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := writeBundleComments(dir, comments); err != nil {
		t.Fatal(err)
	}
	transcript, err := os.ReadFile(filepath.Join(dir, "prior", "comments.md"))
	if err != nil || !strings.Contains(string(transcript), "Author, 2026-01-01T11:00:00Z") || !strings.Contains(string(transcript), "A later reply") || strings.Contains(string(transcript), "older discussion") || strings.Contains(string(transcript), "Finding") {
		t.Fatalf("wrong reply context: %q %v", transcript, err)
	}
	keys, err := os.ReadFile(filepath.Join(dir, "prior", "inline-keys.txt"))
	if err != nil || string(keys) != "012345abcd\n" {
		t.Fatalf("posted inline key missing: %q %v", keys, err)
	}
}

func TestBitbucketReviewCommentsUseSupportedJSONCommand(t *testing.T) {
	bin := t.TempDir()
	bb := filepath.Join(bin, "bb")
	script := `#!/bin/sh
if [ "$*" != "--json pr comments list 7 --limit 1000 --no-truncate" ]; then
  echo "unexpected bb arguments: $*" >&2
  exit 2
fi
printf '%s\n' '{"comments":[{"content":{"raw":"Review reply"},"created_on":"2026-01-01T11:00:00Z","user":{"display_name":"Author"}}]}'
`
	if err := os.WriteFile(bb, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	dir := t.TempDir()
	copyPublicReviewComments(dir, ReviewManifest{PR: "7", Forge: "bitbucket", RepoDir: dir})
	transcript, err := os.ReadFile(filepath.Join(dir, "prior", "comments.md"))
	if err != nil || !strings.Contains(string(transcript), "Review reply") {
		t.Fatalf("Bitbucket reply was not included: %q %v", transcript, err)
	}
}
