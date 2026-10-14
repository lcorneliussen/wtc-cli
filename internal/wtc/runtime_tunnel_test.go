package wtc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimeTunnelRecognisesProvidersFromTheCommand(t *testing.T) {
	c := runtimeFixture(t)
	fragment := filepath.Join(c.Collection, "widget/.harness/dekit.tasks.yaml")
	data, _ := os.ReadFile(fragment)
	tunnel := func(cmd, extra string) runtimeTask {
		t.Helper()
		fixtureFile(t, fragment, strings.Replace(string(data), " web/tunnel:\n  cmd: [sh, -c, 'sleep 60']\n", " web/tunnel:\n  cmd: "+cmd+"\n"+extra, 1), 0644)
		m, _, err := c.runtimePlan("dekit")
		if err != nil {
			t.Fatal(err)
		}
		return m.Tasks[1]
	}
	for _, tc := range []struct {
		cmd, extra, provider, exposure, basis string
	}{
		{`[sh, -c, 'exec tailscale serve --https=8443 http://127.0.0.1:3000']`, "", "tailscale", ExposureTailnet, ExposureCommand},
		{`[tailscale, funnel, "3000"]`, "", "tailscale", ExposurePublic, ExposureCommand},
		{`[cloudflared, tunnel, --url, "http://127.0.0.1:3000"]`, "", "cloudflare", ExposurePublic, ExposureCommand},
		{`[cloudflared, tunnel, run, preview]`, "", "cloudflare", "", ""},
		{`[ngrok, http, "3000"]`, "", "ngrok", ExposurePublic, ExposureCommand},
		{`[ngrok, http, "3000", --oauth=google]`, "", "ngrok", ExposureProtected, ExposureCommand},
		{`[devtunnel, host, -p, "3000"]`, "", "devtunnel", ExposureAccount, ExposureCommand},
		{`[devtunnel, host, -p, "3000", --allow-anonymous]`, "", "devtunnel", ExposurePublic, ExposureCommand},
		// A declaration beats the command; an explicit provider needs no recognisable command.
		{`[ngrok, http, "3000"]`, "  x-wtc: {public: false}\n", "ngrok", ExposureProtected, ExposureDeclared},
		{`[sh, -c, 'sleep 60']`, "  x-wtc: {tunnel: devtunnel}\n", "devtunnel", "", ""},
		{`[sh, -c, 'sleep 60']`, "", "", "", ""},
	} {
		task := tunnel(tc.cmd, tc.extra)
		if task.Provider != tc.provider || task.Exposure != tc.exposure || task.ExposureBasis != tc.basis {
			t.Errorf("%s %s: got %q %q %q, want %q %q %q", tc.cmd, tc.extra, task.Provider, task.Exposure, task.ExposureBasis, tc.provider, tc.exposure, tc.basis)
		}
		// A known client brings its own URL pattern.
		if p, ok := runtimeTunnelProviderNamed(tc.provider); ok && task.URLLog != p.urlLog {
			t.Errorf("%s: url_log = %q", tc.cmd, task.URLLog)
		}
	}
	fixtureFile(t, fragment, string(data)+"  x-wtc: {tunnel: bore}\n", 0644)
	if _, _, err := c.runtimePlan("dekit"); err == nil || !strings.Contains(err.Error(), "tailscale, cloudflare, ngrok, devtunnel") {
		t.Fatalf("accepted an unknown provider: %v", err)
	}
}

func TestRuntimeTunnelURLPatternsFindProviderAddresses(t *testing.T) {
	for name, line := range map[string]string{
		"tailscale":  "Available within your tailnet:\n\nhttps://workstation.example.ts.net:8443/\n|-- proxy http://127.0.0.1:3000",
		"cloudflare": "INF |  https://calm-river-example.trycloudflare.com                                |",
		"ngrok":      `t=2026-10-14 lvl=info msg="started tunnel" obj=tunnels name=command_line addr=http://localhost:3000 url=https://ab12-example.ngrok-free.app`,
		"devtunnel":  "Connect via browser: https://abc123x-3000.euw.devtunnels.ms",
	} {
		p, _ := runtimeTunnelProviderNamed(name)
		log := filepath.Join(t.TempDir(), "task.log")
		fixtureFile(t, log, line+"\n", 0600)
		got := runtimeLogURL(log, p.urlLog)
		if got == "" || RuntimeTunnelProvider(got) != name {
			t.Errorf("%s: found %q", name, got)
		}
	}
}

func TestRuntimeTunnelExposurePrecedence(t *testing.T) {
	served := []byte(`{"Foreground":{"x":{"TCP":{"8443":{"HTTPS":true}},"Web":{"workstation.example.ts.net:8443":{}}}}}`)
	funnel := []byte(`{"Web":{"workstation.example.ts.net:443":{}},"AllowFunnel":{"workstation.example.ts.net:443":true}}`)
	if got := runtimeTailscaleExposure(served, "https://workstation.example.ts.net:8443"); got != ExposureTailnet {
		t.Fatalf("served address = %q", got)
	}
	if got := runtimeTailscaleExposure(funnel, "https://workstation.example.ts.net"); got != ExposurePublic {
		t.Fatalf("funnel address = %q", got)
	}
	if got := runtimeTailscaleExposure(served, "https://other.example.ts.net:8443"); got != "" {
		t.Fatalf("unserved address = %q", got)
	}
	check := func(data []byte) func(string) string {
		return func(raw string) string { return runtimeTailscaleExposure(data, raw) }
	}
	for _, tc := range []struct {
		item            RuntimeItem
		check           func(string) string
		exposure, basis string
	}{
		// The live check overrides a declaration in either direction.
		{RuntimeItem{Kind: "endpoint", URL: "https://workstation.example.ts.net", Exposure: ExposureProtected, ExposureBasis: ExposureDeclared}, check(funnel), ExposurePublic, ExposureVerified},
		{RuntimeItem{Kind: "endpoint", URL: "https://workstation.example.ts.net:8443"}, check(served), ExposureTailnet, ExposureVerified},
		// Without an answer from Tailscale the address alone says nothing.
		{RuntimeItem{Kind: "endpoint", URL: "https://workstation.example.ts.net:8443"}, check(nil), ExposureUnknown, ""},
		{RuntimeItem{Kind: "endpoint", URL: "https://workstation.example.ts.net:8443", Exposure: ExposureTailnet, ExposureBasis: ExposureCommand}, check(nil), ExposureTailnet, ExposureCommand},
		{RuntimeItem{Kind: "endpoint", URL: "https://calm-river.trycloudflare.com"}, nil, ExposurePublic, ExposureDefault},
		{RuntimeItem{Kind: "endpoint", URL: "https://preview.example.com", Provider: "cloudflare"}, nil, ExposureUnknown, ""},
		{RuntimeItem{Kind: "endpoint", URL: "https://abc-3000.euw.devtunnels.ms"}, nil, ExposureAccount, ExposureDefault},
		{RuntimeItem{Kind: "endpoint", URL: "https://preview.example.com"}, nil, ExposureUnknown, ""},
	} {
		item := tc.item
		runtimeTunnelExposure(&item, tc.check)
		if item.Exposure != tc.exposure || item.ExposureBasis != tc.basis {
			t.Errorf("%s: got %q %q, want %q %q", tc.item.URL, item.Exposure, item.ExposureBasis, tc.exposure, tc.basis)
		}
	}
}
