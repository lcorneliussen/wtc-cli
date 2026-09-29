package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/lcorneliussen/wtc-cli/internal/wtc"
	"github.com/spf13/cobra"
)

func addRetireCommand(root *cobra.Command, asJSON *bool) {
	var force bool
	cmd := &cobra.Command{
		Use:   "retire <collection>",
		Short: "Retire a finished worktree collection",
		Args:  cobra.ExactArgs(1),
	}
	cmd.Flags().BoolVar(&force, "force", false, "Allow dirty or unpushed work after checking that it is disposable")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		c, err := wtc.Discover(cwd)
		if err != nil {
			return err
		}
		result, err := c.RetireCollection(wtc.RetireOptions{Name: args[0], Force: force})
		if err != nil {
			return err
		}
		summary := fmt.Sprintf("retired %s", args[0])
		if !result.FolderRemoved {
			summary = fmt.Sprintf("retired %s; files remain: %s", args[0], strings.Join(result.Leftovers, ", "))
		}
		return emit(envelope{OK: true, Data: result, Summary: summary}, *asJSON)
	}
	root.AddCommand(cmd)
}
