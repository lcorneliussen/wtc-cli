package wtc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestTabaConfigNamespaceSurvivesLegacyGeneratedOutput(t *testing.T) {
	root := adHocFixture(t)
	c := initAdHoc(t, filepath.Join(root, "task"))
	alias := filepath.Join(c.Collection, ".taba", "config")
	if err := os.MkdirAll(filepath.Dir(alias), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(c.Harness, alias); err != nil {
		t.Fatal(err)
	}
	fixtureFile(t, filepath.Join(alias, "taba.toml"), "[runtime]\nbackend = 'dekit'\nbinary = 'chosen-dekit'\n", 0644)
	nested := filepath.Join(c.Collection, "nested")
	if err := os.Mkdir(nested, 0755); err != nil {
		t.Fatal(err)
	}
	c, err := Discover(nested)
	if err != nil || !c.AdHoc || c.Harness != alias || c.Config.Runtime.Binary != "chosen-dekit" {
		t.Fatalf("alternate config was not discovered: %+v %v", c, err)
	}
	if got := FindCollectionRoot(nested); got != c.Collection {
		t.Fatalf("dev discovery chose %q", got)
	}
	source := adHocSource(t, root, "widget")
	if _, err := c.RegisterAdHocRepo(AdHocRepoOptions{Spec: "file://" + source}); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(alias, "repos.yml")); err != nil || !strings.Contains(string(data), "widget") {
		t.Fatalf("registration missed the selected registry: %s %v", data, err)
	}
	if _, err := c.RenderSkills(SkillRenderOptions{}); err != nil {
		t.Fatal(err)
	}
	writeDevBuild(t, c.Collection, "0.1.61-dev.1+dev.gabc1234", 0755)
	if again, err := Discover(nested); err != nil || again.Harness != alias || len(again.Registry.Repos) != 1 {
		t.Fatalf("legacy generated output shadowed config: %+v %v", again, err)
	}
	if _, err := os.Lstat(filepath.Join(c.Collection, AdHocConfigDir)); !os.IsNotExist(err) {
		t.Fatal("reading or rendering created legacy user configuration")
	}
	if err := RecordGatewayHarness(c.Collection, "platform", "https://example.invalid/harness.git"); err != nil {
		t.Fatal(err)
	}
	if name, _, err := GatewayHarness(c.Collection); err != nil || name != "platform" {
		t.Fatalf("gateway metadata was not read from the alias: %q %v", name, err)
	}
	if _, err := os.Lstat(filepath.Join(alias, "wtc.toml")); !os.IsNotExist(err) {
		t.Fatal("gateway metadata shadowed the selected config file")
	}
}

func TestTabaConfigNamespacePreservesLegacyPrecedenceAndBoundaries(t *testing.T) {
	for _, kind := range []string{"both", "parent-file", "parent-dangling", "config-file", "config-dangling", "incomplete", "invalid-config", "registry-directory", "registry-dangling", "registry-fifo"} {
		t.Run(kind, func(t *testing.T) {
			root := adHocFixture(t)
			fixtureFile(t, filepath.Join(root, "harness", ".harness-repos.yml"), "repos: [{name: widget}]\n", 0644)
			dir := filepath.Join(root, "nested")
			fixtureFile(t, filepath.Join(dir, ".taba", "config", "repos.yml"), "repos: []\n", 0644)
			fixtureFile(t, filepath.Join(dir, ".taba", "config", "taba.toml"), "[herdr]\nsession = 'alternate'\n", 0644)
			legacy := filepath.Join(dir, AdHocConfigDir)
			switch kind {
			case "parent-file":
				fixtureFile(t, filepath.Join(dir, ".wtc"), "invalid", 0644)
			case "parent-dangling":
				if err := os.Symlink("missing", filepath.Join(dir, ".wtc")); err != nil {
					t.Fatal(err)
				}
			case "config-file":
				fixtureFile(t, legacy, "invalid", 0644)
			case "config-dangling":
				if err := os.MkdirAll(filepath.Dir(legacy), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("missing", legacy); err != nil {
					t.Fatal(err)
				}
			case "incomplete":
				if err := os.MkdirAll(legacy, 0755); err != nil {
					t.Fatal(err)
				}
			case "registry-directory", "registry-dangling", "registry-fifo":
				if err := os.MkdirAll(legacy, 0755); err != nil {
					t.Fatal(err)
				}
				registry := filepath.Join(legacy, "repos.yml")
				var err error
				switch kind {
				case "registry-directory":
					err = os.Mkdir(registry, 0755)
				case "registry-dangling":
					err = os.Symlink("missing", registry)
				case "registry-fifo":
					err = unix.Mkfifo(registry, 0600)
				}
				if err != nil {
					t.Fatal(err)
				}
			default:
				fixtureFile(t, filepath.Join(legacy, "repos.yml"), "repos: []\n", 0644)
				config := "[herdr]\nsession = 'legacy'\n"
				if kind == "invalid-config" {
					config = "invalid = ["
				}
				fixtureFile(t, filepath.Join(legacy, "wtc.toml"), config, 0644)
			}
			c, err := Discover(dir)
			if kind == "both" {
				if err != nil || c.Harness != legacy || c.Config.Herdr.Session != "legacy" {
					t.Fatalf("legacy namespace lost precedence: %+v %v", c, err)
				}
			} else {
				if err == nil {
					t.Fatalf("broken nested config selected another collection: %+v", c)
				}
				if _, err := OpenCollection(dir); err == nil {
					t.Fatal("explicit open bypassed broken legacy state")
				}
			}
			if found := FindCollectionRoot(dir); found != "" && found != dir {
				t.Fatalf("early dev discovery crossed a broken boundary: %q", found)
			}
		})
	}
}

func TestTabaDevBuildSelectionAndRemoval(t *testing.T) {
	collection := t.TempDir()
	writeDevBuild(t, collection, "0.1.61-dev.1+dev.gabc1234", 0755)
	alias := filepath.Join(collection, ".taba", "dev")
	if err := os.MkdirAll(filepath.Dir(alias), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(devBuildDir(collection), alias); err != nil {
		t.Fatal(err)
	}
	if got := ReadDevBuild(collection); got == nil || got.Binary != filepath.Join(alias, "wtc") {
		t.Fatalf("alternate dev build was not selected: %+v", got)
	}
	if err := os.Chmod(filepath.Join(alias, "wtc"), 0775); err != nil {
		t.Fatal(err)
	}
	if got := ReadDevBuild(collection); got != nil {
		t.Fatalf("group-writable alternate binary was trusted: %+v", got)
	}
	if err := os.Chmod(filepath.Join(alias, "wtc"), 0755); err != nil {
		t.Fatal(err)
	}
	writeDevBuild(t, collection, "invalid", 0755)
	if got := ReadDevBuild(collection); got != nil {
		t.Fatalf("invalid legacy build fell through to the alternate: %+v", got)
	}
	legacy := writeDevBuild(t, collection, "0.1.61-dev.2+dev.gdef1234", 0755)
	if got := ReadDevBuild(collection); got == nil || got.Binary != legacy {
		t.Fatalf("legacy dev slot lost precedence: %+v", got)
	}
	if removed, err := UninstallDevBuild(collection); err != nil || !removed {
		t.Fatalf("uninstall failed: %v %v", removed, err)
	}
	if got := ReadDevBuild(collection); got != nil {
		t.Fatalf("uninstall reactivated a hidden dev build: %+v", got)
	}
	if removed, err := UninstallDevBuild(collection); err != nil || removed {
		t.Fatalf("repeated uninstall changed state: %v %v", removed, err)
	}
}

func TestTabaDevRemovalDoesNotFollowLinkedParents(t *testing.T) {
	collection := t.TempDir()
	legacy := writeDevBuild(t, collection, "0.1.61-dev.1+dev.gabc1234", 0755)
	outside := t.TempDir()
	fixtureFile(t, filepath.Join(outside, "dev", "keep"), "keep", 0644)
	if err := os.Symlink(outside, filepath.Join(collection, ".taba")); err != nil {
		t.Fatal(err)
	}
	if _, err := UninstallDevBuild(collection); err == nil {
		t.Fatal("uninstall followed a linked parent")
	}
	for _, path := range []string{legacy, filepath.Join(outside, "dev", "keep")} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("uninstall removed state before preflight finished: %v", err)
		}
	}
}

func TestTabaGeneratedLinksRenderLegacyAndRetirePreservesLocalState(t *testing.T) {
	root := adHocFixture(t)
	c := initAdHoc(t, filepath.Join(root, "task"))
	alias := filepath.Join(c.Collection, ".taba", "config")
	if err := os.MkdirAll(filepath.Dir(alias), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(c.Harness, alias); err != nil {
		t.Fatal(err)
	}
	c, err := OpenCollection(c.Collection)
	if err != nil {
		t.Fatal(err)
	}
	fixtureFile(t, filepath.Join(alias, "skills", "local", "SKILL.md"), "local override\n", 0644)
	fixtureFile(t, filepath.Join(alias, "collection-AGENTS.md"), "local collection instructions\n", 0644)
	fixtureFile(t, filepath.Join(alias, "hooks", "agent-env.json"), "{}\n", 0644)
	fixtureFile(t, filepath.Join(c.Collection, ".taba", "instructions", "old.md"), "old generated instructions", 0644)
	fixtureFile(t, filepath.Join(c.Collection, ".taba", "skills", "retired", "SKILL.md"), "old generated skill", 0644)
	fixtureFile(t, filepath.Join(c.Collection, ".taba", "keep.txt"), "local state", 0644)
	fixtureFile(t, filepath.Join(c.Collection, ".agents", "skills", "personal", "SKILL.md"), "personal skill", 0644)
	stale := filepath.Join(c.Collection, ".agents", "skills", "retired")
	if err := os.Symlink("../../.taba/skills/retired", stale); err != nil {
		t.Fatal(err)
	}
	escaped := filepath.Join(c.Collection, ".agents", "skills", "escaped")
	if err := os.Symlink("../../.taba/skills/../../personal", escaped); err != nil {
		t.Fatal(err)
	}
	if result, err := c.RenderSkills(SkillRenderOptions{DryRun: true}); err != nil || result.Pruned != 1 {
		t.Fatalf("preview did not recognize alternate generated link: %+v %v", result, err)
	}
	if _, err := os.Lstat(stale); err != nil {
		t.Fatal("preview changed an alternate link")
	}
	if result, err := c.RenderSkills(SkillRenderOptions{}); err != nil || result.Pruned != 1 {
		t.Fatalf("render did not prune alternate generated link: %+v %v", result, err)
	}
	for path, want := range map[string]string{
		".agents/skills/local":     "../../.taba/config/skills/local",
		".agents/skills/wtc-start": "../../.wtc/skills/wtc-start",
		"AGENTS.md":                ".taba/config/collection-AGENTS.md",
		".cursor/hooks.json":       "../.taba/config/hooks/agent-env.json",
	} {
		if link, err := os.Readlink(filepath.Join(c.Collection, path)); err != nil || link != want {
			t.Fatalf("rendered %s -> %q, want %q: %v", path, link, want, err)
		}
	}
	if err := removeRetiredGeneratedFiles(c.Collection, true); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{".taba/instructions", ".taba/skills", ".taba/config/repos.yml", ".agents/skills/local", ".agents/skills/wtc-start", "AGENTS.md", ".cursor/hooks.json", ".claude/settings.json", ".grok/hooks/wtc-agent-env.json"} {
		if _, err := os.Lstat(filepath.Join(c.Collection, path)); !os.IsNotExist(err) {
			t.Fatalf("generated %s survived retirement: %v", path, err)
		}
	}
	for _, path := range []string{".taba/keep.txt", ".taba/config/skills/local/SKILL.md", ".agents/skills/personal/SKILL.md", ".taba/config/collection-AGENTS.md", ".taba/config/hooks/agent-env.json"} {
		if _, err := os.Stat(filepath.Join(c.Collection, path)); err != nil {
			t.Fatalf("local state %s was removed: %v", path, err)
		}
	}
	if link, err := os.Readlink(escaped); err != nil || link != "../../.taba/skills/../../personal" {
		t.Fatalf("an unrelated link was classified as generated: %q %v", link, err)
	}
}

func TestTabaRetirementDoesNotFollowLinkedNamespace(t *testing.T) {
	target, outside := t.TempDir(), t.TempDir()
	defaultBody, err := ReadDefault("collection-AGENTS.md")
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"config/repos.yml": "repos: []\n", "collection-AGENTS.md": string(defaultBody), "skills/local/SKILL.md": "keep"} {
		fixtureFile(t, filepath.Join(outside, name), body, 0644)
	}
	if err := os.Symlink(outside, filepath.Join(target, ".taba")); err != nil {
		t.Fatal(err)
	}
	if err := removeRetiredGeneratedFiles(target, true); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"config/repos.yml", "collection-AGENTS.md", "skills/local/SKILL.md"} {
		if _, err := os.Stat(filepath.Join(outside, name)); err != nil {
			t.Fatalf("retirement followed linked parent: %s %v", name, err)
		}
	}
}
