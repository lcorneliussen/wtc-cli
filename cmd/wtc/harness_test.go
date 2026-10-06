package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestHarnessCommandsWorkOutsideCollection(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "wtc")
	if out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %s: %v", out, err)
	}
	run := func(args ...string) (string, error) {
		cmd := exec.Command(binary, args...)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	if out, err := run("docs", "instructions/publication-privacy.md"); err != nil || !strings.Contains(out, "Publication privacy") {
		t.Fatal(out, err)
	}
	if out, err := run("docs", "../../../etc/passwd"); err == nil || !strings.Contains(out, "unknown embedded document") {
		t.Fatal(out, err)
	}
	if out, err := run("harness", "init", "harness", "--remote", "https://example.invalid/harness.git"); err == nil || !strings.Contains(out, "cli-version") {
		t.Fatal(out, err)
	}
	if _, err := os.Stat(filepath.Join(root, "harness")); !os.IsNotExist(err) {
		t.Fatal("invalid command wrote scaffold")
	}
	if out, err := run("harness", "init", "harness", "--remote", "https://example.invalid/harness.git", "--cli-version", "0.1.38"); err != nil || !strings.Contains(out, "created harness") {
		t.Fatal(out, err)
	}
}
