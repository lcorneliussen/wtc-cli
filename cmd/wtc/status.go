package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/lcorneliussen/wtc-cli/internal/wtc"
	"github.com/spf13/cobra"
)

// Explicit previews do not write snapshots that a status pane could mistake
// for a completed one-shot collection.
func addStatusCommand(root *cobra.Command, asJSON *bool) {
	var local, forge, all, md, cached, noFetch bool
	cmd := &cobra.Command{Use: "status", Short: "Inspect worktree collection status", Args: cobra.NoArgs}
	cmd.Flags().BoolVar(&local, "local", false, "Preview local Git facts (no forge facts or cache writes)")
	cmd.Flags().BoolVar(&forge, "forge", false, "Preview local Git and PR facts (no snapshot writes)")
	cmd.Flags().BoolVar(&all, "all", false, "Include every collection in the workspace")
	cmd.Flags().BoolVar(&md, "md", false, "Render agent Markdown")
	cmd.Flags().BoolVar(&cached, "cached", false, "Read the last completed snapshot without Git or forge calls")
	cmd.Flags().BoolVar(&noFetch, "no-fetch", false, "Use current local refs without fetching")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if cached {
			if local || forge || all || noFetch {
				return fmt.Errorf("--cached cannot be combined with --local, --forge, --all, or --no-fetch")
			}
		} else if local && forge {
			return fmt.Errorf("--local and --forge cannot be combined")
		}
		if forge && all {
			return fmt.Errorf("--forge is currently scoped to this collection")
		}
		if md && *asJSON {
			return fmt.Errorf("--md and --json cannot be combined")
		}
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		c, err := wtc.Discover(cwd)
		if err != nil {
			return err
		}
		if cached {
			snapshot, err := c.ReadStatusSnapshot()
			if err == nil {
				if *asJSON {
					return json.NewEncoder(os.Stdout).Encode(snapshot)
				}
				age, err := c.CachedStatusAge()
				if err != nil {
					return err
				}
				body := snapshot.Markdown()
				fmt.Printf("# %s (snapshot, %ds old)\n%s", snapshot.Collection, age, strings.TrimPrefix(body, "# "+snapshot.Collection+"\n"))
				return nil
			}
			if !os.IsNotExist(err) || *asJSON {
				return err
			}
			text, err := c.LegacyStatusText()
			if err != nil {
				return err
			}
			fmt.Print(text)
			return nil
		}
		var snapshot wtc.StatusSnapshot
		var fetched wtc.StatusFetchReport
		if forge {
			snapshot, fetched, err = c.StatusLivePreview(noFetch)
		} else if local {
			snapshot, err = c.StatusLocalSnapshot(all)
		} else {
			snapshot, fetched, err = c.StatusLiveSnapshot(all, noFetch)
		}
		if fetched.Failed > 0 {
			fmt.Fprintf(os.Stderr, "wtc: warning: %d status ref refresh(es) failed; showing local refs\n", fetched.Failed)
		}
		if err != nil {
			return err
		}
		if md || !*asJSON {
			fmt.Print(snapshot.Markdown())
			return nil
		}
		return json.NewEncoder(os.Stdout).Encode(snapshot)
	}
	root.AddCommand(cmd)
}
