package main

import (
	"os"

	"github.com/lcorneliussen/wtc-cli/internal/wtc"
	"github.com/spf13/cobra"
)

func addDiscoveryCommand(root *cobra.Command, asJSON *bool) {
	var transcripts []string
	cmd := &cobra.Command{Use: "discover [collection]", Short: "Read private retirement evidence and propose verification steps", Args: cobra.MaximumNArgs(1)}
	cmd.Flags().StringArrayVar(&transcripts, "transcript", nil, "Explicit Claude JSONL task transcript (repeat up to eight); archive state remains unknown")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		c, err := wtc.Discover(cwd)
		if err != nil {
			return err
		}
		name := "."
		if len(args) > 0 {
			name = args[0]
		}
		report, err := c.DiscoverRetirement(name, transcripts)
		if err != nil {
			return err
		}
		return emit(envelope{OK: true, Data: report, Summary: report.Text()}, *asJSON)
	}
	root.AddCommand(cmd)
}
