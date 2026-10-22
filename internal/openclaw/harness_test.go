package openclaw

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseGatewayConfig(t *testing.T) {
	cfg, err := ParseGatewayConfig("[plugins] noise\n" + `{"teamTailnet": true, "harnesses": [{"name": "platform", "remote": "git@github.com:example-org/platform-harness.git"}]}`)
	if err != nil || len(cfg.Harnesses) != 1 || cfg.Harnesses[0].Ref != "main" {
		t.Fatalf("%+v %v", cfg, err)
	}
	if cfg, err := ParseGatewayConfig(""); err != nil || len(cfg.Harnesses) != 0 {
		t.Errorf("empty: %+v %v", cfg, err)
	}
	for config, want := range map[string]string{
		`{"harnesses": [{"name": "none", "remote": "https://github.com/o/h"}]}`:                                                 `neither "none"`,
		`{"harnesses": [{"name": "a", "remote": "https://github.com/o/h"}, {"name": "a", "remote": "https://github.com/o/i"}]}`: "twice",
		`{"harnesses": [{"name": "a", "remote": "/srv/h.git"}]}`:                                                                "not a GitHub or Bitbucket",
		`{"harnesses": [{"name": "a", "remote": "https://github.com/o/h", "ref": "-x"}]}`:                                       "not a branch name",
		`{"harnesses": [{"name": "a", "remote": "https://github.com/o/h"}], "defaultHarness": "b"}`:                             "defaultHarness b",
		`{"harnesses": "a"}`: "cannot unmarshal",
	} {
		if _, err := ParseGatewayConfig(config); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v, want %q", config, err, want)
		}
	}
}

func TestChooseHarness(t *testing.T) {
	two := GatewayConfig{Harnesses: []Harness{{Name: "a", Remote: "https://github.com/o/a"}, {Name: "b", Remote: "https://github.com/o/b"}}}
	lists := func(h Harness, name string) bool {
		return h.Name == "a" && name == "api" || h.Name == "b" && name == "infra" || name == "shared"
	}
	withDefault := func(name string) GatewayConfig {
		return GatewayConfig{Harnesses: two.Harnesses, DefaultHarness: name}
	}
	for _, tc := range []struct {
		name            string
		cfg             GatewayConfig
		flag, recorded  string
		names           []string
		want, wantError string
	}{
		{name: "no harnesses", cfg: GatewayConfig{}, names: []string{"api"}},
		{name: "kept", cfg: two, recorded: "b", names: []string{"api"}, want: "b"},
		{name: "kept, same flag", cfg: two, recorded: "b", flag: "b", want: "b"},
		{name: "kept, other flag", cfg: two, recorded: "b", flag: "a", wantError: "uses harness b"},
		{name: "kept, gone", cfg: two, recorded: "c", wantError: "no longer configures"},
		{name: "none", cfg: two, flag: NoHarness},
		{name: "flag", cfg: two, flag: "a", want: "a"},
		{name: "unknown flag", cfg: two, flag: "c", wantError: "no harness c"},
		{name: "flag without harnesses", cfg: GatewayConfig{}, flag: "c", wantError: "configures no harnesses"},
		// The registries decide before the default does.
		{name: "the one listing it, not the default", cfg: withDefault("b"), names: []string{"api"}, want: "a"},
		{name: "the only one listing the names", cfg: two, names: []string{"infra"}, want: "b"},
		{name: "listed beside an ad hoc one", cfg: two, names: []string{"api", "https://github.com/o/other"}, want: "a"},
		// Listed by none: the default or the only one, and add-repo asks
		// the person about the repository.
		{name: "listed by none: the default", cfg: withDefault("b"), names: []string{"https://github.com/o/other"}, want: "b"},
		{name: "listed by none: the only one", cfg: GatewayConfig{Harnesses: two.Harnesses[:1]}, names: []string{"infra"}, want: "a"},
		{name: "listed by none", cfg: two, names: []string{"https://github.com/o/other"}, wantError: "None lists"},
		{name: "the one listing all", cfg: two, names: []string{"api", "shared"}, want: "a"},
		{name: "listed by both: the default", cfg: withDefault("b"), names: []string{"shared"}, want: "b"},
		{name: "listed by both, no default", cfg: two, names: []string{"shared"}, wantError: "Several list"},
		{name: "each lists one", cfg: two, names: []string{"api", "infra"}, wantError: "Ask the person"},
		{name: "no names: the default", cfg: withDefault("b"), want: "b"},
		{name: "no names: the only one", cfg: GatewayConfig{Harnesses: two.Harnesses[:1]}, want: "a"},
		{name: "no names", cfg: two, wantError: "Ask the person"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, err := ChooseHarness(tc.cfg, tc.flag, tc.recorded, tc.names, lists)
			got := ""
			if h != nil {
				got = h.Name
			}
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Errorf("error %v, want %q", err, tc.wantError)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Errorf("got %q %v, want %q", got, err, tc.want)
			}
		})
	}
}

// A collection's folder is under its harness, or ad-hoc; one made before
// that, in wtc-collections, is still its folder.
func TestCollectionFolder(t *testing.T) {
	state := t.TempDir()
	if got := CollectionFolder(state, "1-x"); got != "" {
		t.Errorf("no folder yet: %q", got)
	}
	if got, want := NewCollectionFolder(state, "1-x", "platform"), filepath.Join(state, "wtc", "platform", "1-x"); got != want {
		t.Errorf("under its harness: %s, want %s", got, want)
	}
	if got, want := NewCollectionFolder(state, "1-x", ""), filepath.Join(state, "wtc", AdHocFolder, "1-x"); got != want {
		t.Errorf("without one: %s, want %s", got, want)
	}
	for _, dir := range []string{filepath.Join(state, "wtc", "platform", "2-x"), filepath.Join(state, "wtc-collections", "3-x")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// Found where it is, whatever harness is asked for.
	if got := NewCollectionFolder(state, "2-x", ""); got != filepath.Join(state, "wtc", "platform", "2-x") {
		t.Errorf("an existing folder: %s", got)
	}
	if got := CollectionFolder(state, "3-x"); got != filepath.Join(state, "wtc-collections", "3-x") {
		t.Errorf("a folder from before: %s", got)
	}
	// A worker's collection goes beside its task's.
	if got := NewCollectionFolder(state, "2-x--a", ""); got != filepath.Join(state, "wtc", "platform", "2-x--a") {
		t.Errorf("a worker of a task in a harness: %s", got)
	}
	if got := NewCollectionFolder(state, "3-x--a", ""); got != filepath.Join(state, "wtc", AdHocFolder, "3-x--a") {
		t.Errorf("a worker of a task from before: %s", got)
	}
	if _, err := ParseGatewayConfig(`{"harnesses": [{"name": "ad-hoc", "remote": "https://github.com/o/h"}]}`); err == nil {
		t.Error("a harness called ad-hoc")
	}
}
