package openclaw

import (
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
		`{"harnesses": [{"name": "none", "remote": "https://github.com/o/h"}]}`:                                                 "not \"none\"",
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
		return h.Name == "a" && name == "api" || h.Name == "b" && name == "infra"
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
		{name: "default", cfg: GatewayConfig{Harnesses: two.Harnesses, DefaultHarness: "b"}, names: []string{"api"}, want: "b"},
		{name: "the only one", cfg: GatewayConfig{Harnesses: two.Harnesses[:1]}, want: "a"},
		{name: "the only one listing the names", cfg: two, names: []string{"infra"}, want: "b"},
		{name: "both or neither list them", cfg: two, names: []string{"api", "infra"}, wantError: "Ask the person"},
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
