package openclaw

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestIntentJSON(t *testing.T) {
	add, err := AddRepo("api", "api", "72-openclaw-host", "main", "Work on api")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		intent Intent
		want   string
	}{
		{"add-repo", add, `{"host":"openclaw","version":1,"action":"add-repo","actor":"parent","repo":"api","tool":"sessions_spawn","args":{"task":"Work on api","label":"72-openclaw-host api","projectId":"api","worktree":true,"worktreeName":"72-openclaw-host","worktreeBaseRef":"main","visible":true}}`},
		{"publish on GitHub", Publish("api", ParseOrigin("https://github.com/example-org/api.git"), "72-openclaw-host", "main", "Add thing", ""),
			`{"host":"openclaw","version":1,"action":"publish","actor":"repo-session","repo":"api","forge":"github","tool":"github_publish","args":{"title":"Add thing"}}`},
		{"publish on Bitbucket", Publish("docs", ParseOrigin("git@bitbucket.org:example-ws/docs.git"), "72-openclaw-host", "main", "Add thing", ""),
			`{"host":"openclaw","version":1,"action":"publish","actor":"repo-session","repo":"docs","forge":"bitbucket","args":{"owner":"example-ws","repo":"docs","source":"openclaw/72-openclaw-host","dest":"main","title":"Add thing","draft":true}}`},
		{"status", Status(), `{"host":"openclaw","version":1,"action":"status","actor":"current","tool":"sessions_list","args":{}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.intent)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Fatalf("got  %s\nwant %s", got, tc.want)
			}
		})
	}
}

func TestAddRepoRejectsWhatTheGatewayWould(t *testing.T) {
	if _, err := AddRepo("api", "", "72-x", "", "t"); err == nil {
		t.Fatal("accepted an empty project")
	}
	for _, name := range []string{"", "Feat-X", "feat/x", "feat_x", "-x", strings.Repeat("a", 65)} {
		if _, err := AddRepo("api", "api", name, "", "t"); err == nil {
			t.Fatalf("accepted worktree name %q", name)
		}
	}
	if got := Branch("72-openclaw-host"); got != "openclaw/72-openclaw-host" {
		t.Fatalf("branch %q", got)
	}
}

func TestParseOrigin(t *testing.T) {
	for url, want := range map[string]Origin{
		"https://github.com/example-org/api.git":          {ForgeGitHub, "example-org", "api"},
		"git@github.com:example-org/api.git":              {ForgeGitHub, "example-org", "api"},
		"ssh://git@github.com/example-org/api":            {ForgeGitHub, "example-org", "api"},
		"git@bitbucket.org:example-ws/docs.git":           {ForgeBitbucket, "example-ws", "docs"},
		"bitbucket.org:example-ws/docs.git":               {ForgeBitbucket, "example-ws", "docs"},
		"https://user@bitbucket.org/example-ws/docs.git/": {ForgeBitbucket, "example-ws", "docs"},
		"ssh://git@bitbucket.org:22/example-ws/docs.git":  {ForgeBitbucket, "example-ws", "docs"},
		"https://gitlab.com/example-org/group/site.git":   {Forge: ForgeUnknown},
		"/tmp/origins/api.git":                            {Forge: ForgeUnknown},
	} {
		if got := ParseOrigin(url); got != want {
			t.Errorf("%s: got %+v, want %+v", url, got, want)
		}
	}
}
