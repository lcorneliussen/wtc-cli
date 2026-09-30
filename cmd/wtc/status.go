package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/lcorneliussen/wtc-cli/internal/wtc"
	"github.com/spf13/cobra"
)

// Previews are deliberately explicit while discovery, caching, and the live
// view are being ported. Neither writes a snapshot that a status pane could
// mistake for a complete one.
func addStatusCommand(root *cobra.Command, asJSON *bool) {
	var local, forge, all, md bool
	cmd := &cobra.Command{Use: "status", Short: "Inspect worktree collection status", Args: cobra.NoArgs}
	cmd.Flags().BoolVar(&local, "local", false, "Preview local Git facts (no forge facts or cache writes)")
	cmd.Flags().BoolVar(&forge, "forge", false, "Preview local Git and PR facts (no snapshot writes)")
	cmd.Flags().BoolVar(&all, "all", false, "Include every collection in the workspace")
	cmd.Flags().BoolVar(&md, "md", false, "Render agent Markdown")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if local == forge {
			return fmt.Errorf("native status is still being ported; choose --local or --forge for a preview")
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
		var snapshot wtc.StatusSnapshot
		if forge {
			snapshot, err = c.StatusForgePreview()
		} else {
			snapshot, err = c.StatusLocalSnapshot(all)
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
