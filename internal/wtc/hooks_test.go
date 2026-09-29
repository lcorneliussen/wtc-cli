package wtc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHarnessHooksCanVetoAndPostFailuresDoNotUndo(t *testing.T) {
	c := fixture(t)
	hooks := filepath.Join(c.Harness, "hooks", "wtc")
	if err := os.MkdirAll(hooks, 0755); err != nil {
		t.Fatal(err)
	}
	pre := filepath.Join(hooks, "env.pre.sh")
	if err := os.WriteFile(pre, []byte("#!/bin/sh\nexit 7\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := c.RunHook("env.pre", nil); err == nil {
		t.Fatal("pre-hook failed to veto")
	}
	post := filepath.Join(hooks, "env.post.sh")
	if err := os.WriteFile(post, []byte("#!/bin/sh\nexit 7\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := c.RunHook("env.post", nil); err != nil {
		t.Fatalf("post-hook blocked completed action: %v", err)
	}
	if err := os.Chmod(post, 0644); err != nil {
		t.Fatal(err)
	}
	if err := c.RunHook("env.post", nil); err != nil {
		t.Fatalf("non-executable post-hook blocked completed action: %v", err)
	}
}
