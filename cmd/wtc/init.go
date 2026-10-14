package main

import (
	"os"

	"github.com/lcorneliussen/wtc-cli/internal/wtc"
	"github.com/spf13/cobra"
)

func addInitCommand(root *cobra.Command, asJSON *bool) {
	cmd := &cobra.Command{
		Use:   "init [directory]",
		Short: "Make an empty folder an ad-hoc collection, without a harness repository",
		Args:  cobra.MaximumNArgs(1),
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		if len(args) == 1 {
			dir = args[0]
		}
		result, err := wtc.InitAdHoc(dir)
		if err != nil {
			return err
		}
		return emit(envelope{OK: true, Data: result, Summary: "done: ad-hoc collection at " + result.Collection + "\nnext: wtc add-repo <clone-url|path-to-checkout>"}, *asJSON)
	}
	root.AddCommand(cmd)
}
