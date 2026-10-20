package main

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// fakeGateway answers the commands openClawChecks runs.
type fakeGateway struct {
	git, plugins, customPlugins, agents string
	files                               map[string]string
	noOpenClaw                          bool
}

func (g fakeGateway) env(version string) openClawEnv {
	return openClawEnv{
		lookPath: func(name string) (string, error) {
			if name == "openclaw" && g.noOpenClaw {
				return "", errors.New("not found")
			}
			return "/usr/bin/" + name, nil
		},
		run: func(name string, args ...string) (string, error) {
			switch strings.Join(append([]string{name}, args...), " ") {
			case "git --version":
				return g.git, nil
			case "openclaw plugins list --json":
				return g.plugins, nil
			case "openclaw config get " + customPluginsKey:
				return g.customPlugins, nil
			case "openclaw agents list --json":
				return g.agents, nil
			}
			return "", errors.New("unexpected command")
		},
		readFile: func(path string) ([]byte, error) {
			if data, ok := g.files[path]; ok {
				return []byte(data), nil
			}
			return nil, os.ErrNotExist
		},
		version: version,
	}
}

func checksByName(checks []doctorCheck) map[string]doctorCheck {
	m := map[string]doctorCheck{}
	for _, c := range checks {
		m[c.Name] = c
	}
	return m
}

func healthyGateway() fakeGateway {
	return fakeGateway{
		git:           "git version 2.47.3\n",
		plugins:       `{"plugins":[{"id":"bitbucket","version":"0.3.0","enabled":true},{"id":"wtc","version":"0.1.43","enabled":true,"status":"loaded"}]}`,
		customPlugins: "true\n",
		agents:        `[{"id":"main","workspace":"/ws/main"},{"id":"ops","workspace":"/ws/ops"}]`,
		files: map[string]string{
			"/ws/main/AGENTS.md": "# Team\n\n## Worktree collections\n\nRead `wtc docs instructions/openclaw.md`.\n",
			"/ws/ops/AGENTS.md":  "# Ops\n\nNothing about collections.\n",
		},
	}
}

func TestOpenClawDoctorChecksTheGateway(t *testing.T) {
	checks := checksByName(openClawChecks(healthyGateway().env("v0.1.43")))
	for _, name := range []string{"git", "gh", "plugin", "control-ui", "agents.md:main"} {
		if !checks[name].OK {
			t.Errorf("%s: %+v", name, checks[name])
		}
	}
	ops := checks["agents.md:ops"]
	if ops.OK || ops.Required || !strings.Contains(ops.Detail, "wtc docs openclaw-AGENTS.md >> /ws/ops/AGENTS.md") {
		t.Errorf("an AGENTS.md without wtc is advice that says how to fix it: %+v", ops)
	}
}

func TestOpenClawDoctorFlagsWhatIsWrong(t *testing.T) {
	g := healthyGateway()
	g.git = "git version 2.39.5\n"
	g.customPlugins = "false\n"
	g.files = map[string]string{}
	checks := checksByName(openClawChecks(g.env("0.1.44")))
	if c := checks["git"]; c.OK || !c.Required || !strings.Contains(c.Detail, "2.42") {
		t.Errorf("old git: %+v", c)
	}
	if c := checks["plugin"]; c.OK || !strings.Contains(c.Detail, "0.1.43 is installed, but this wtc is 0.1.44") {
		t.Errorf("stale plugin: %+v", c)
	}
	if c := checks["control-ui"]; c.OK || !c.Required {
		t.Errorf("custom plugins off: %+v", c)
	}
	if c := checks["agents.md:main"]; c.OK || !strings.Contains(c.Detail, "> /ws/main/AGENTS.md") {
		t.Errorf("missing AGENTS.md: %+v", c)
	}

	// A dev build compares no versions; a missing plugin is required.
	if c := checksByName(openClawChecks(healthyGateway().env("0.1.44-dev.2")))["plugin"]; !c.OK {
		t.Errorf("a dev build should accept any plugin version: %+v", c)
	}
	g = healthyGateway()
	g.plugins = `{"plugins":[]}`
	if c := checksByName(openClawChecks(g.env("0.1.43")))["plugin"]; c.OK || !c.Required || !strings.Contains(c.Detail, "wtc openclaw install") {
		t.Errorf("no plugin: %+v", c)
	}

	// Without openclaw, only what can be checked is.
	g = healthyGateway()
	g.noOpenClaw = true
	checks = checksByName(openClawChecks(g.env("0.1.43")))
	if c := checks["openclaw"]; c.OK || !c.Required {
		t.Errorf("no openclaw: %+v", c)
	}
	if _, ok := checks["plugin"]; ok {
		t.Error("the plugin cannot be checked without openclaw")
	}
}
