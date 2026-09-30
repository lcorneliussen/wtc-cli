package wtc

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStatusForgeCacheIsPrivateFreshAndDoesNotReadSymlinks(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	t.Setenv("WTC_FORGE_CACHE_AGE", "1")
	forge, slug, number := "github.com", "example/widget", "7"
	statusWriteForgeCache(forge, slug, number, []byte(`{"number":7,"state":"OPEN"}`))
	path := statusForgeCachePath(forge, slug, number)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("forge cache permissions: %o", info.Mode().Perm())
	}
	if data, ok := statusReadForgeCache(forge, slug, number); !ok || len(data) == 0 {
		t.Fatal("fresh cache was not read")
	}
	past := time.Now().Add(-2 * time.Second)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatal(err)
	}
	if _, ok := statusReadForgeCache(forge, slug, number); ok {
		t.Fatal("stale forge cache was read")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("sensitive"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}
	if _, ok := statusReadForgeCache(forge, slug, number); ok {
		t.Fatal("symlink cache was read")
	}
}
