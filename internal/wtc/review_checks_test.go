package wtc

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewChecksReportConfirmedFailureWithoutInventingPolicy(t *testing.T) {
	setupHeadCheckServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/statuses") {
			fmt.Fprint(w, `{"values":[{"state":"FAILED","name":"untrusted forge instructions"}]}`)
		} else {
			fmt.Fprint(w, `{"values":[]}`)
		}
	})
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "bb"), []byte("#!/bin/sh\nprintf '%s' '{\"source\":{\"commit\":{\"hash\":\""+checkHeadB+"\"}}}'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	m := ReviewManifest{Forge: "bitbucket", Slug: "example/widget", PR: "7", HeadSHA: checkHeadB, RepoDir: t.TempDir()}
	bundle := t.TempDir()
	got, err := writeReviewChecks(bundle, m)
	if err != nil || got.Result != "FAILURE" || !got.Complete {
		t.Fatalf("evidence: %+v %v", got, err)
	}
	report := reviewChecksReport(bundle, m)
	for _, want := range []string{"Confirmed failing check", checkHeadB, "Required-check policy is unknown", "separate from the code-review verdict"} {
		if !strings.Contains(report, want) {
			t.Fatalf("missing %q: %s", want, report)
		}
	}
	raw, _ := os.ReadFile(filepath.Join(bundle, "checks.json"))
	if strings.Contains(string(raw), "untrusted") {
		t.Fatalf("raw forge prose escaped normalization: %s", raw)
	}
	m.HeadSHA = checkHeadA
	if report = reviewChecksReport(bundle, m); !strings.Contains(report, "UNKNOWN") || strings.Contains(report, "Confirmed failing") {
		t.Fatalf("stale evidence reused: %s", report)
	}
}

func TestReviewChecksGitHubHeadMismatchIsUnknown(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	t.Setenv("WTC_FORGE_MIN_INTERVAL", "0")
	bin := t.TempDir()
	script := "#!/bin/sh\nprintf '%s' '{\"headRefOid\":\"" + checkHeadA + "\",\"statusCheckRollup\":[{\"conclusion\":\"SUCCESS\",\"status\":\"COMPLETED\"}]}'\n"
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	m := ReviewManifest{Forge: "github", Slug: "example/widget", PR: "7", HeadSHA: checkHeadB}
	got := collectReviewChecks(m)
	if got.Result != "UNKNOWN" || got.Complete {
		t.Fatalf("stale green: %+v", got)
	}
	m.HeadSHA = checkHeadA
	got = collectReviewChecks(m)
	if got.Result != "SUCCESS" || !got.Complete {
		t.Fatalf("current checks: %+v", got)
	}
	data, _ := json.Marshal(got)
	if !strings.Contains(string(data), `"requiredness":"unknown"`) {
		t.Fatal(string(data))
	}
}

func TestReviewChecksAcceptMatchingAbbreviatedBitbucketHead(t *testing.T) {
	hits := 0
	setupHeadCheckServer(t, func(w http.ResponseWriter, r *http.Request) {
		hits++
		if strings.Contains(r.URL.Path, "/statuses") {
			fmt.Fprint(w, `{"values":[{"state":"FAILED"}]}`)
		} else {
			fmt.Fprint(w, `{"values":[]}`)
		}
	})
	bin := t.TempDir()
	for _, prefix := range []string{checkHeadB[:12], checkHeadA[:12], checkHeadB[:7]} {
		script := "#!/bin/sh\nprintf '%s' '{\"source\":{\"commit\":{\"hash\":\"" + prefix + "\"}}}'\n"
		if err := os.WriteFile(filepath.Join(bin, "bb"), []byte(script), 0755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
		m := ReviewManifest{Forge: "bitbucket", Slug: "example/widget", PR: "7", HeadSHA: checkHeadB, RepoDir: t.TempDir()}
		got := collectReviewChecks(m)
		if prefix == checkHeadB[:12] {
			if got.Result != "FAILURE" || !got.Complete || got.Head != checkHeadB {
				t.Fatalf("matching prefix rejected: %+v", got)
			}
		} else if got.Result != "UNKNOWN" || got.Complete {
			t.Fatalf("unsafe prefix matched: %+v", got)
		}
	}
	if hits != 2 {
		t.Fatalf("queried mismatched heads: %d", hits)
	}
}
