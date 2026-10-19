package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteOpenClawPluginStampsTheVersion(t *testing.T) {
	dir := t.TempDir()
	if err := writeOpenClawPlugin(dir, "v1.2.3"); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"index.js", "package.json", "openclaw.plugin.json", "dist/control-ui/v1/index.js", "skills/wtc/SKILL.md"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("%s was not written: %v", f, err)
		}
	}
	for _, f := range []string{"package.json", "openclaw.plugin.json"} {
		data, _ := os.ReadFile(filepath.Join(dir, f))
		if !strings.Contains(string(data), `"version": "1.2.3"`) {
			t.Errorf("%s is not stamped with the version:\n%s", f, data)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "embed.go")); err == nil {
		t.Error("the Go file that embeds the plugin is not part of it")
	}
	skill, _ := os.ReadFile(filepath.Join(dir, "skills/wtc/SKILL.md"))
	if !strings.Contains(string(skill), "wtc docs instructions/openclaw.md") {
		t.Error("the skill does not point the agent at the embedded instructions")
	}
}

// install runs openclaw from the PATH: config first, then the plugin.
func TestOpenClawInstallRunsOpenClaw(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "wtc")
	if out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %s: %v", out, err)
	}
	bin := filepath.Join(root, "bin")
	log := filepath.Join(root, "calls")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	// The fake records its arguments and that the plugin it is given exists.
	fake := "#!/bin/sh\necho \"$*\" >> " + log + "\nif [ \"$1\" = plugins ]; then test -f \"$3/openclaw.plugin.json\" && echo manifest >> " + log + "; fi\n"
	if err := os.WriteFile(filepath.Join(bin, "openclaw"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "openclaw", "install")
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("install: %s: %v", out, err)
	}
	calls, _ := os.ReadFile(log)
	lines := strings.Split(strings.TrimSpace(string(calls)), "\n")
	if len(lines) != 3 || lines[0] != "config set "+customPluginsKey+" true" || !strings.HasPrefix(lines[1], "plugins install ") || !strings.HasSuffix(lines[1], " --force") || lines[2] != "manifest" {
		t.Fatalf("openclaw was called with:\n%s", calls)
	}
}
