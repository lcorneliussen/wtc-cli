package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Exercise the real command and separate synthetic reviewer/forge processes.
func TestReviewProgressFailurePreservesIndependentRun(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		runFail        bool
	}{
		{"ambiguous-create", "echo '{}'", false},
		{"transport", "echo synthetic-post-failure >&2; exit 9", false},
		{"post-and-run-fail", "echo '{}'", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			collection := t.TempDir()
			bundle := filepath.Join(collection, "bundle")
			bin := filepath.Join(collection, "bin")
			write := func(path, body string, mode os.FileMode) {
				t.Helper()
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(body), mode); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.MkdirAll(filepath.Join(collection, "app", ".git"), 0755); err != nil {
				t.Fatal(err)
			}
			write(filepath.Join(collection, "harness", ".harness-repos.yml"), "repos:\n  - name: app\n    remote: https://github.com/example/app.git\n", 0644)
			manifest, _ := json.Marshal(map[string]any{"repo": "app", "pr": "7", "forge": "github", "slug": "example/app", "head_sha": "1234567890abcdef1234567890abcdef12345678", "round": 1, "repo_dir": filepath.Join(collection, "app"), "public": true})
			write(filepath.Join(bundle, "manifest.json"), string(manifest), 0644)
			write(filepath.Join(bundle, "concerns", "code.md"), "---\nid: code\ntier: fast\n---\nReview code.\n", 0644)
			write(filepath.Join(bin, "gh"), `#!/bin/sh
case "$*" in
 *'--json headRefOid'*) echo '{"headRefOid":"1234567890abcdef1234567890abcdef12345678"}';;
 *'api -X POST'*) cat >/dev/null; echo POST >> "$FORGE_LOG"; `+tc.response+`;;
 *) echo "unexpected forge invocation: $*" >&2; exit 8;;
esac
`, 0755)
			fail := ""
			if tc.runFail {
				fail = "exit 3"
			}
			write(filepath.Join(bin, "agent"), `#!/bin/sh
case "$(basename "$3")" in
 lead.md)
  echo lead >> "$AGENT_LOG"
  `+fail+`
  echo '**Local review: pass**' > "$4/summary.md"
  echo pass > "$4/verdict";;
 *)
  echo concern >> "$AGENT_LOG"
  echo '{"concern":"code","status":"ok","findings":[{"severity":"minor","title":"Synthetic retained finding"}]}' > "$4/findings/code.json";;
esac
`, 0755)
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("FORGE_LOG", filepath.Join(collection, "forge.log"))
			t.Setenv("AGENT_LOG", filepath.Join(collection, "agent.log"))
			t.Setenv("WTC_REVIEW_AGENT_CMD", filepath.Join(bin, "agent"))
			args := []string{"wtc", "review", "run", bundle, "--collection", collection, "--post", "--hosted-first=false", "--fast", "test:", "--lead", "test:", "--json"}
			encoded, _ := json.Marshal(args)
			cmd := exec.Command(os.Args[0], "-test.run=^TestReviewProgressCommandChild$")
			cmd.Env = append(os.Environ(), "WTC_PROGRESS_ARGS="+string(encoded))
			output, err := cmd.CombinedOutput()
			if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 || !strings.Contains(string(output), "post review progress:") {
				t.Fatalf("missing final posting error/exit 1: %v %s", err, output)
			}
			if tc.runFail && !strings.Contains(string(output), "lead review failed") {
				t.Errorf("run error lost: %s", output)
			}
			if !tc.runFail && (!strings.Contains(string(output), `"ok":false`) || !strings.Contains(string(output), `"verdict":"pass"`)) {
				t.Errorf("completed result not emitted honestly: %s", output)
			}
			read := func(path string) string {
				t.Helper()
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("missing %s: %v", filepath.Base(path), err)
				}
				return string(data)
			}
			if got := read(filepath.Join(collection, "agent.log")); got != "concern\nlead\n" {
				t.Fatalf("independent processes did not complete: %q", got)
			}
			if !strings.Contains(read(filepath.Join(bundle, "findings", "code.json")), "Synthetic retained finding") {
				t.Fatal("findings lost")
			}
			verdict := "pass"
			if tc.runFail {
				if _, err := os.Stat(filepath.Join(bundle, "verdict")); !os.IsNotExist(err) {
					t.Fatalf("failed lead invented verdict: %v", err)
				}
			} else {
				if got := strings.TrimSpace(read(filepath.Join(bundle, "verdict"))); got != verdict {
					t.Fatalf("verdict=%q want %q", got, verdict)
				}
				read(filepath.Join(bundle, "summary.md"))
			}
			if got := read(filepath.Join(collection, "forge.log")); got != "POST\n" {
				t.Fatalf("ambiguous write was retried: %q", got)
			}
			if _, err := os.Stat(filepath.Join(bundle, "comment.id")); !os.IsNotExist(err) {
				t.Fatalf("invented comment id: %v", err)
			}
			receipts, _ := filepath.Glob(filepath.Join(collection, ".wtc-review-posted", "*"))
			if len(receipts) != 0 {
				t.Fatalf("invented receipts: %v", receipts)
			}
		})
	}
}

// Run the actual entry point in a child so exit status and emitted results are
// checked together with the reviewer artifacts by the parent.
func TestReviewProgressCommandChild(t *testing.T) {
	args := os.Getenv("WTC_PROGRESS_ARGS")
	if args == "" {
		return
	}
	if err := json.Unmarshal([]byte(args), &os.Args); err != nil {
		t.Fatal(err)
	}
	main()
}
