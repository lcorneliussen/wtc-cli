package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lcorneliussen/wtc-cli/internal/openclaw"
)

func TestSandboxChecks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/wtc/session" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"repo":"api","branch":"openclaw/12-x","base":"main","repos":["api","web"]}`))
	}))
	defer srv.Close()
	gw := openclaw.GatewayFromEnv(func(string) string { return srv.URL })
	nothing := func(name string, args ...string) (string, error) { return "", errors.New("no") }

	checks := sandboxChecks(gw, func(string) string { return "" }, nothing)
	if len(checks) != 2 || !checks[0].OK || !strings.Contains(checks[0].Detail, "2 repos: api, web") || !checks[1].OK {
		t.Fatalf("a clean sandbox: %+v", checks)
	}

	env := map[string]string{"GH_TOKEN": "x"}
	helper := func(name string, args ...string) (string, error) {
		if name == "git" {
			return "store\n", nil
		}
		return "", errors.New("no")
	}
	checks = sandboxChecks(gw, func(k string) string { return env[k] }, helper)
	if checks[1].OK || !strings.Contains(checks[1].Detail, "GH_TOKEN") || !strings.Contains(checks[1].Detail, "credential.helper") {
		t.Fatalf("a sandbox with credentials: %+v", checks[1])
	}

	srv.Close()
	if checks := sandboxChecks(gw, func(string) string { return "" }, nothing); checks[0].OK {
		t.Fatalf("an unreachable service: %+v", checks[0])
	}
}
