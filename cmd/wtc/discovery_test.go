package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lcorneliussen/wtc-cli/internal/wtc"
	"github.com/spf13/cobra"
)

func TestDiscoveryCLIExplicitInputsAndPrivateEnvelope(t *testing.T) {
	collection := filepath.Join(t.TempDir(), "example-task")
	harness := filepath.Join(collection, "harness")
	if err := os.MkdirAll(harness, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(harness, ".harness-repos.yml"), []byte("repos:\n  - name: widget\n    remote: https://example.invalid/widget.git\n"), 0600); err != nil {
		t.Fatal(err)
	}
	transcript := filepath.Join(t.TempDir(), "task.jsonl")
	line, err := json.Marshal(map[string]any{"type": "assistant", "cwd": collection, "timestamp": "2026-01-01T00:00:00Z", "message": map[string]any{"role": "assistant", "content": []any{map[string]string{"type": "text", "text": "Done. Still verify current facts."}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(transcript, append(line, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(collection)
	for _, asJSON := range []bool{true, false} {
		var jsonOutput bool
		root := &cobra.Command{Use: "wtc", SilenceUsage: true, SilenceErrors: true}
		root.PersistentFlags().BoolVar(&jsonOutput, "json", false, "")
		addDiscoveryCommand(root, &jsonOutput)
		args := []string{"discover", "--transcript", transcript, "--transcript", transcript}
		if asJSON {
			args = append(args, "--json")
		} else {
			args = append(args, "example-task")
		}
		root.SetArgs(args)
		read, write, e := os.Pipe()
		if e != nil {
			t.Fatal(e)
		}
		stdout := os.Stdout
		os.Stdout = write
		// Drain concurrently: the report can exceed a pipe's buffer capacity.
		output := make(chan []byte, 1)
		go func() { b, _ := io.ReadAll(read); output <- b }()
		runErr := root.Execute()
		os.Stdout = stdout
		write.Close()
		raw := <-output
		read.Close()
		if runErr != nil {
			t.Fatal(runErr)
		}
		if asJSON {
			var got struct {
				OK   bool                `json:"ok"`
				Data wtc.DiscoveryReport `json:"data"`
			}
			if e := json.Unmarshal(raw, &got); e != nil {
				t.Fatalf("invalid JSON: %v %s", e, raw)
			}
			if !got.OK || got.Data.Schema != 1 || got.Data.Collection != collection || got.Data.Disposition != "needs-verification" || len(got.Data.Transcripts) != 2 || got.Data.Transcripts[0].ArchiveState != "unknown" || got.Data.Transcripts[0].MatchedNotes != 1 || len(got.Data.Blockers) == 0 {
				t.Fatalf("lost private evidence contract: %s", raw)
			}
		} else if !strings.Contains(string(raw), "no cleanup authorization") || !strings.Contains(string(raw), "Untrusted hint") {
			t.Fatalf("missing evidence labels: %s", raw)
		}
	}
}
