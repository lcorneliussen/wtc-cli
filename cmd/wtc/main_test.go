package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompatibility(t *testing.T) {
	cases := []struct {
		requirement, version string
		want                 bool
	}{
		{">=0.4,<0.5", "0.4.2", true},
		{">=0.4,<0.5", "0.5.0", false},
		{">=0.4,<0.5", "dev", false},
		{"invalid", "0.4.2", false},
	}
	for _, c := range cases {
		if got := compatible(c.requirement, c.version); got != c.want {
			t.Errorf("compatible(%q,%q)=%v", c.requirement, c.version, got)
		}
	}
}

func TestEnvPreflightsPinAndShowsMiseDryRun(t *testing.T) {
	collection := filepath.Join(t.TempDir(), "demo")
	harness := filepath.Join(collection, "harness")
	if err := os.MkdirAll(harness, 0755); err != nil {
		t.Fatal(err)
	}
	registry := "repos:\n  - name: example\n    remote: https://example.invalid/example.git\n    default_ref: origin/main\n"
	if err := os.WriteFile(filepath.Join(harness, ".harness-repos.yml"), []byte(registry), 0644); err != nil {
		t.Fatal(err)
	}
	envPath := filepath.Join(collection, ".env.collection")
	misePath := filepath.Join(collection, "mise.toml")
	for _, path := range []string{envPath, misePath} {
		if err := os.WriteFile(path, []byte("keep\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	pinPath := filepath.Join(harness, ".wtc-cli-version")
	if err := os.WriteFile(pinPath, []byte("latest\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "run", ".", "env", "--collection", collection)
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("accepted invalid pin: %s", out)
	}
	for _, path := range []string{envPath, misePath} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != "keep\n" {
			t.Fatalf("changed %s on invalid pin: %q, %v", path, data, err)
		}
	}
	if err := os.WriteFile(pinPath, []byte("0.1.1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command("go", "run", ".", "env", "--collection", collection, "--dry-run")
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), `"github:lcorneliussen/wtc-cli" = "0.1.1"`) {
		t.Fatalf("dry run did not show pin: %s, %v", out, err)
	}
	cmd = exec.Command("go", "run", ".", "env")
	out, err = cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "list") || !strings.Contains(string(out), "setup") {
		t.Fatalf("bare env did not show its commands: %s, %v", out, err)
	}
	cmd = exec.Command("go", "run", ".", "env", "setup", "--collection", collection, "--dry-run")
	out, err = cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), `"github:lcorneliussen/wtc-cli" = "0.1.1"`) {
		t.Fatalf("named setup dry run did not show pin: %s, %v", out, err)
	}
}
