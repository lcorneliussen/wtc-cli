package wtc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func installEnvLifecycleFixture(t *testing.T, c *Context) {
	t.Helper()
	harness := filepath.Join(c.Workspace, "source-agent-harness")
	fixtureFile(t, filepath.Join(harness, "hooks", "wtc", "env.pre.sh"), "#!/bin/sh\nprintf '%s' \"$WTC_COLLECTION\" > .env-pre-ran\n", 0755)
	fixtureFile(t, filepath.Join(harness, "hooks", "wtc", "env.post.sh"), "#!/bin/sh\n[ -f .env-pre-ran ] || exit 33\nprintf '\\nAPP_COLLECTION=%s\\n' \"$WTC_COLLECTION\" >> .env.collection\n", 0755)
	widget := filepath.Join(c.Workspace, "source-widget")
	fixtureFile(t, filepath.Join(widget, ".harness", "init.sh"), "#!/bin/sh\nprintf '%s|%s' \"${APP_COLLECTION:-missing}\" \"$(cat ../.env-pre-ran 2>/dev/null)\" > alias-init-ran\n", 0755)
	for name, source := range map[string]string{"agent-harness": harness, "widget": widget} {
		fixtureGit(t, "-C", source, "add", "-A")
		fixtureGit(t, "-C", source, "-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "environment lifecycle fixture")
		fixtureGit(t, "--git-dir="+filepath.Join(c.Workspace, ".bare", name+".git"), "fetch", "-q", "origin", "+refs/heads/*:refs/remotes/origin/*")
	}
}

func TestNewCollectionRunsEnvironmentHooksBeforeInit(t *testing.T) {
	c := newWorkspaceFixture(t)
	installEnvLifecycleFixture(t, c)
	r, err := c.NewCollection(NewOptions{Slug: "isolated", Repos: []string{"widget"}})
	if err != nil {
		t.Fatal(err)
	}
	init, err := os.ReadFile(filepath.Join(r.Collection, "widget", "alias-init-ran"))
	if err != nil || string(init) != "isolated|isolated" {
		t.Fatalf("init must observe environment customization: %q (%v)", init, err)
	}
}

func TestAddRepositoriesRunsEnvironmentHooksBeforeInit(t *testing.T) {
	for _, absent := range []bool{false, true} {
		name := "regenerated"
		if absent {
			name = "absent"
		}
		t.Run(name, func(t *testing.T) {
			c := newWorkspaceFixture(t)
			installEnvLifecycleFixture(t, c)
			r, err := c.NewCollection(NewOptions{Slug: "existing"})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(filepath.Join(r.Collection, ".env-pre-ran")); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			if absent {
				if err := os.Remove(filepath.Join(r.Collection, ".env.collection")); err != nil {
					t.Fatal(err)
				}
			} else {
				fixtureFile(t, filepath.Join(r.Collection, ".env.collection"), "WTC_CONFIG_ROOT="+c.ConfigRoot+"\nAPP_COLLECTION=stale\n", 0644)
			}
			fixtureFile(t, filepath.Join(r.Collection, ".env.collection.local"), "LOCAL_OVERRIDE=retained\n", 0600)
			if _, err := c.AddRepositories(AddRepoOptions{Collection: "existing", Repos: []string{"widget"}}); err != nil {
				t.Fatal(err)
			}
			init, err := os.ReadFile(filepath.Join(r.Collection, "widget", "alias-init-ran"))
			if err != nil || string(init) != "existing|existing" {
				t.Fatalf("init must observe refreshed customization: %q (%v)", init, err)
			}
			local, err := os.ReadFile(filepath.Join(r.Collection, ".env.collection.local"))
			if err != nil || string(local) != "LOCAL_OVERRIDE=retained\n" {
				t.Fatalf("local overrides changed: %q (%v)", local, err)
			}
		})
	}
}

func TestEnvironmentPreHookVetoStopsRepositoryInit(t *testing.T) {
	for _, add := range []bool{false, true} {
		name := "new"
		if add {
			name = "add-repo"
		}
		t.Run(name, func(t *testing.T) {
			c := newWorkspaceFixture(t)
			installEnvLifecycleFixture(t, c)
			var collection string
			var err error
			if add {
				r, createErr := c.NewCollection(NewOptions{Slug: "existing"})
				if createErr != nil {
					t.Fatal(createErr)
				}
				collection = r.Collection
				fixtureFile(t, filepath.Join(collection, "harness", "hooks", "wtc", "env.pre.sh"), "#!/bin/sh\nexit 35\n", 0755)
				_, err = c.AddRepositories(AddRepoOptions{Collection: "existing", Repos: []string{"widget"}})
			} else {
				harness := filepath.Join(c.Workspace, "source-agent-harness")
				fixtureFile(t, filepath.Join(harness, "hooks", "wtc", "env.pre.sh"), "#!/bin/sh\nexit 35\n", 0755)
				fixtureGit(t, "-C", harness, "add", "-A")
				fixtureGit(t, "-C", harness, "-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "veto fixture")
				fixtureGit(t, "--git-dir="+filepath.Join(c.Workspace, ".bare", "agent-harness.git"), "fetch", "-q", "origin", "+refs/heads/*:refs/remotes/origin/*")
				collection = filepath.Join(c.Workspace, "vetoed")
				_, err = c.NewCollection(NewOptions{Slug: "vetoed", Repos: []string{"widget"}})
			}
			if err == nil || !strings.Contains(err.Error(), "env.pre") {
				t.Fatalf("expected env.pre veto, got %v", err)
			}
			if _, err := os.Stat(filepath.Join(collection, "widget", "alias-init-ran")); !os.IsNotExist(err) {
				t.Fatalf("repository init ran after veto: %v", err)
			}
		})
	}
}
