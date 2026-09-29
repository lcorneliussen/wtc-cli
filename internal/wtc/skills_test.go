package wtc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderSkillsLayersDefaultsAndPreservesLocalFiles(t *testing.T) {
	c := fixture(t)
	write := func(path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(c.Harness, "skills", "wtc-local", "SKILL.md"), "local skill\n")
	write(filepath.Join(c.Harness, "overlays", "skills", "wtc-pr", "SKILL.md"), "override pr skill\n")
	write(filepath.Join(c.Harness, "collection-AGENTS.md"), "collection instructions\n")
	write(filepath.Join(c.Harness, "collection-SCOPE.md"), "# {{COLLECTION}}\n{{REPOS}}\n")
	write(filepath.Join(c.Harness, "hooks", "agent-env.json"), "{}\n")
	write(filepath.Join(c.Collection, "widget", ".git"), "gitdir: synthetic\n")
	write(filepath.Join(c.Collection, ".claude", "skills", "wtc-local", "SKILL.md"), "personal override\n")
	stale := filepath.Join(c.Collection, ".agents", "skills", "wtc-gone")
	if err := os.MkdirAll(filepath.Dir(stale), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../harness/skills/wtc-gone", stale); err != nil {
		t.Fatal(err)
	}
	unrelated := filepath.Join(c.Collection, ".agents", "skills", "personal-link")
	if err := os.Symlink("/tmp/personal-skill", unrelated); err != nil {
		t.Fatal(err)
	}
	customEnvrc := filepath.Join(c.Collection, ".envrc")
	write(customEnvrc, "export USER_CHOICE=1\n")

	dry, err := c.RenderSkills(SkillRenderOptions{DryRun: true, SeedScope: true})
	if err != nil {
		t.Fatal(err)
	}
	if dry.Linked == 0 || dry.Pruned != 1 || dry.Skipped < 2 {
		t.Fatalf("dry-run missed changes or overrides: %+v", dry)
	}
	if _, err := os.Lstat(filepath.Join(c.Collection, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatal("dry run created AGENTS.md")
	}
	if _, err := os.Lstat(filepath.Join(c.Collection, ".wtc", "skills")); !os.IsNotExist(err) {
		t.Fatal("dry run materialized defaults")
	}
	if _, err := os.Lstat(filepath.Join(c.Collection, "WTC-SCOPE.md")); !os.IsNotExist(err) {
		t.Fatal("dry run seeded scope")
	}
	if _, err := os.Lstat(filepath.Join(c.Collection, ".env.toolchain")); !os.IsNotExist(err) {
		t.Fatal("dry run refreshed toolchain cache")
	}

	first, err := c.RenderSkills(SkillRenderOptions{SeedScope: true})
	if err != nil {
		t.Fatal(err)
	}
	if first.Pruned != 1 || first.Linked == 0 {
		t.Fatalf("first render did not change links: %+v", first)
	}
	if _, err := os.Lstat(stale); !os.IsNotExist(err) {
		t.Fatal("stale owned link survived")
	}
	if got, err := os.Readlink(unrelated); err != nil || got != "/tmp/personal-skill" {
		t.Fatal("unrelated skill link was pruned")
	}
	assertLink := func(path, want string) {
		t.Helper()
		got, err := os.Readlink(filepath.Join(c.Collection, path))
		if err != nil || got != want {
			t.Fatalf("%s -> %q, want %q: %v", path, got, want, err)
		}
	}
	assertLink(".agents/skills/wtc-local", "../../harness/skills/wtc-local")
	assertLink(".agents/skills/wtc-pr", "../../harness/overlays/skills/wtc-pr")
	assertLink(".claude/skills/wtc-customize", "../../.wtc/skills/wtc-customize")
	assertLink("AGENTS.md", "harness/collection-AGENTS.md")
	assertLink(".cursor/hooks.json", "../harness/hooks/agent-env.json")
	defaultSkill, err := os.ReadFile(filepath.Join(c.Collection, ".wtc", "skills", "wtc-customize", "SKILL.md"))
	if err != nil || !strings.Contains(string(defaultSkill), "wtc-customize") {
		t.Fatalf("embedded skill not materialized: %v", err)
	}
	scope, err := os.ReadFile(filepath.Join(c.Collection, "WTC-SCOPE.md"))
	if err != nil || !strings.Contains(string(scope), "# demo") || !strings.Contains(string(scope), "| `widget` |") {
		t.Fatalf("scope not seeded from checked-out repos: %s, %v", scope, err)
	}
	if _, err := os.Stat(filepath.Join(c.Collection, ".env.toolchain")); err != nil {
		t.Fatal("toolchain cache missing:", err)
	}
	if body, _ := os.ReadFile(customEnvrc); string(body) != "export USER_CHOICE=1\n" {
		t.Fatal("custom envrc overwritten")
	}
	if body, _ := os.ReadFile(filepath.Join(c.Collection, ".claude", "skills", "wtc-local", "SKILL.md")); string(body) != "personal override\n" {
		t.Fatal("personal skill overwritten")
	}
	write(filepath.Join(c.Collection, "WTC-SCOPE.md"), "hand-edited scope\n")
	second, err := c.RenderSkills(SkillRenderOptions{SeedScope: true})
	if err != nil || second.Linked != 0 || second.Pruned != 0 {
		t.Fatalf("second render was not idempotent: %+v, %v", second, err)
	}
	if body, _ := os.ReadFile(filepath.Join(c.Collection, "WTC-SCOPE.md")); string(body) != "hand-edited scope\n" {
		t.Fatal("existing scope overwritten")
	}
}

func TestRenderSkillsUsesEmbeddedEntryWhenHarnessHasNone(t *testing.T) {
	c := fixture(t)
	if _, err := c.RenderSkills(SkillRenderOptions{}); err != nil {
		t.Fatal(err)
	}
	target, err := os.Readlink(filepath.Join(c.Collection, "AGENTS.md"))
	if err != nil || target != ".wtc/collection-AGENTS.md" {
		t.Fatalf("embedded entry link = %q, %v", target, err)
	}
	data, err := os.ReadFile(filepath.Join(c.Collection, ".wtc", "collection-AGENTS.md"))
	if err != nil || !strings.Contains(string(data), "worktree collection") {
		t.Fatalf("embedded entry missing: %v", err)
	}
}
