package wtc

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRuntimeProbe(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer ok.Close()
	// A login page or a redirect answers too.
	login := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/login", http.StatusFound) }))
	defer login.Close()
	// What a tunnel says while the service behind it is down.
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusBadGateway) }))
	defer bad.Close()
	gone := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	goneURL := gone.URL
	gone.Close()

	for _, tc := range []struct {
		url    string
		want   bool
		detail string
	}{{ok.URL, true, ""}, {login.URL, true, ""}, {bad.URL, false, "HTTP 502"}, {goneURL, false, "refused"}} {
		got, detail := runtimeProbe(tc.url)
		if got != tc.want || !strings.Contains(detail, tc.detail) {
			t.Errorf("%s: %v %q, want %v %q", tc.url, got, detail, tc.want, tc.detail)
		}
	}
}

// Only ready tasks with an address are looked at; what they say is kept.
func TestRuntimeAnswering(t *testing.T) {
	items := []RuntimeItem{
		{Path: "a", State: "ready", URL: "https://a.example"},
		{Path: "b", State: "ready", URL: "https://b.example"},
		{Path: "c", State: "exited", URL: "https://c.example"},
		{Path: "d", State: "ready"},
	}
	asked := map[string]bool{}
	runtimeAnswering(items, func(raw string) (bool, string) {
		asked[raw] = true
		return raw == "https://a.example", "HTTP 502"
	})
	if items[0].Answering != "yes" || items[1].Answering != "no" || items[1].AnswerDetail != "HTTP 502" || items[2].Answering != "" || items[3].Answering != "" {
		t.Errorf("%+v", items)
	}
	if asked["https://c.example"] {
		t.Error("an exited task was looked at")
	}
}

func TestTunnelLogins(t *testing.T) {
	items := []RuntimeItem{
		{Path: "a", Kind: "endpoint", Provider: "tailscale"},
		{Path: "b", Kind: "endpoint", Provider: "tailscale"},
		{Path: "c", Kind: "endpoint", Provider: "devtunnel"},
		{Path: "d", Kind: "service"},
	}
	asked := 0
	runtimeTunnelLogins(items, func(p string) string {
		asked++
		if p == "devtunnel" {
			return "devtunnel's login expired: devtunnel user login"
		}
		return ""
	})
	if asked != 2 || items[0].Login != "" || items[2].Login == "" || items[3].Login != "" {
		t.Errorf("asked %d: %+v", asked, items)
	}
}

func TestProviderLoginReadings(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct{ json, want string }{
		{`{"BackendState":"Running","Self":{"KeyExpiry":"2027-03-06T07:30:21Z"}}`, ""},
		{`{"BackendState":"NeedsLogin","AuthURL":"https://login.tailscale.com/a/x"}`, "needs a login"},
		{`{"BackendState":"Stopped"}`, "stopped"},
		{`{"BackendState":"NeedsMachineAuth"}`, "approval"},
		{`{"BackendState":"Running","Self":{"KeyExpiry":"2026-10-01T00:00:00Z"}}`, "key expired"},
		{`not json`, ""},
	} {
		if got := tailscaleLogin([]byte(tc.json), now); (tc.want == "") != (got == "") || !strings.Contains(got, tc.want) {
			t.Errorf("tailscale %s: %q, want %q", tc.json, got, tc.want)
		}
	}
	for _, tc := range []struct{ out, want string }{
		{"Logged in as someone@example.com using GitHub.", ""},
		{"Login token expired.", "expired"},
		{"Not logged in.", "not logged in"},
	} {
		if got := devtunnelLogin(tc.out); (tc.want == "") != (got == "") || !strings.Contains(got, tc.want) {
			t.Errorf("devtunnel %q: %q, want %q", tc.out, got, tc.want)
		}
	}
}

// A tunnel whose provider client is not signed in is not opened.
func TestTunnelRefusedWithoutProviderLogin(t *testing.T) {
	c := runtimeFixture(t)
	fixtureFile(t, filepath.Join(c.Collection, ".env.collection.local"), "ECHO_PORT=43001\n", 0600)
	if _, err := c.AddService(ServiceOptions{Service: "echo", PortEnv: "ECHO_PORT", Cmd: []string{"python3", "-m", "http.server"}, NoStart: true}); err != nil {
		t.Fatal(err)
	}
	was := runtimeLoginCheck
	t.Cleanup(func() { runtimeLoginCheck = was })
	runtimeLoginCheck = func(p string) string {
		if p == "tailscale" {
			return "Tailscale needs a login: tailscale up"
		}
		return ""
	}
	_, err := c.Tunnel(TunnelOptions{Service: "echo", Provider: "tailscale"})
	if err == nil || !strings.Contains(err.Error(), "tailscale up") {
		t.Fatalf("opened a tunnel without a login: %v", err)
	}
	if data, _ := os.ReadFile(c.runtimeLocalFragment("widget")); strings.Contains(string(data), "tailscale") {
		t.Fatal("a refused tunnel was written")
	}
}

func TestRuntimeTextSaysWhatIsWrongNow(t *testing.T) {
	out := RuntimeText([]RuntimeItem{
		{Collection: "c", Repo: "app", Group: "app/web", Path: "app/web/main", Kind: "service", State: "ready", URL: "http://127.0.0.1:43001", Answering: "no", AnswerDetail: "HTTP 502"},
		{Collection: "c", Repo: "app", Group: "app/web", Path: "app/web/tailscale", Kind: "endpoint", State: "idle", Provider: "tailscale", Login: "Tailscale needs a login: tailscale up"},
	})
	for _, want := range []string{"ready = startup probe passed, then looked at now", "not answering now: HTTP 502", "Tailscale needs a login: tailscale up"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

// Status as people see it carries the live look; up's wait loop polls
// without it, so its deadline holds.
func TestRuntimeStatusWiring(t *testing.T) {
	c := runtimeFixture(t)
	runtimeLoginCheck = func(string) string { return "Tailscale needs a login: tailscale up" }
	called := 0
	runtimeAnswerCheck = func(string) (bool, string) { called++; return false, "HTTP 502" }
	items := []RuntimeItem{{Path: "a", Kind: "endpoint", Provider: "tailscale", State: "ready", URL: "https://a.example"}}
	runtimeTunnelLogins(items, runtimeLoginCheck)
	runtimeAnswering(items, runtimeAnswerCheck)
	if items[0].Login == "" || items[0].Answering != "no" || items[0].AnswerDetail != "HTTP 502" || called != 1 {
		t.Fatalf("%+v", items[0])
	}
	// Without a runner both views are the same, and the quiet one asks nothing.
	called = 0
	_ = c.runtimeStatusOnly()
	if called != 0 {
		t.Fatal("up's wait loop looked at addresses")
	}
}
