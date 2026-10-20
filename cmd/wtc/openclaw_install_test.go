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

// install runs openclaw from the PATH: config first, then the plugin, then
// the reload that makes the running gateway use it.
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
	if len(lines) != 4 || lines[0] != "config set "+customPluginsKey+" true" || !strings.HasPrefix(lines[1], "plugins install ") || !strings.HasSuffix(lines[1], " --force") || lines[2] != "manifest" || lines[3] != "plugins reload wtc" {
		t.Fatalf("openclaw was called with:\n%s", calls)
	}
}

func TestOpenClawInstallFlagsAndErrors(t *testing.T) {
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
	run := func(path string, args ...string) (string, error) {
		cmd := exec.Command(binary, append([]string{"openclaw", "install"}, args...)...)
		cmd.Env = append(os.Environ(), "PATH="+path)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	// --dir writes the plugin and calls nothing.
	dir := filepath.Join(root, "plugin")
	if out, err := run(bin, "--dir", dir); err != nil || !strings.Contains(out, "openclaw plugins install "+dir) {
		t.Fatalf("--dir: %s %v", out, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "openclaw.plugin.json")); err != nil {
		t.Fatal("--dir wrote no plugin")
	}

	// Without openclaw on the PATH it says where to run it.
	if out, err := run(bin); err == nil || !strings.Contains(out, "openclaw is not on the PATH") {
		t.Fatalf("no openclaw: %s %v", out, err)
	}

	// --no-config installs without touching the setting; a failing
	// openclaw is an error, except a failing reload, which only asks for a
	// restart.
	fake := "#!/bin/sh\necho \"$*\" >> " + log + "\n[ -n \"$FAIL\" ] && [ \"$2\" = \"$FAIL\" ] && exit 1\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "openclaw"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := run(bin, "--no-config"); err != nil {
		t.Fatalf("--no-config: %s %v", out, err)
	}
	calls, _ := os.ReadFile(log)
	if lines := strings.Split(strings.TrimSpace(string(calls)), "\n"); len(lines) != 2 || !strings.HasPrefix(lines[0], "plugins install ") || lines[1] != "plugins reload wtc" {
		t.Fatalf("--no-config called openclaw with:\n%s", calls)
	}
	fail := func(sub string) (string, error) {
		cmd := exec.Command(binary, "openclaw", "install")
		cmd.Env = append(os.Environ(), "PATH="+bin, "FAIL="+sub)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	if out, err := fail("set"); err == nil || !strings.Contains(out, "openclaw config set") {
		t.Fatalf("a failing openclaw was not reported: %s %v", out, err)
	}
	if out, err := fail("reload"); err != nil || !strings.Contains(out, "restart the gateway") {
		t.Fatalf("a failing reload should install and ask for a restart: %s %v", out, err)
	}
}
