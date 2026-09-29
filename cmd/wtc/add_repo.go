package main

import (
	"os"

	"github.com/lcorneliussen/wtc-cli/internal/wtc"
	"github.com/spf13/cobra"
)

func addAddRepoCommand(root *cobra.Command, asJSON *bool) {
	var collection, branch string
	var tip bool
	cmd := &cobra.Command{
		Use:   "add-repo <repo> [repo ...]",
		Short: "Add repository worktrees to a collection",
		Args:  cobra.MinimumNArgs(1),
	}
	cmd.Flags().StringVar(&collection, "collection", "", "Target collection name (default: current)")
	cmd.Flags().StringVarP(&branch, "branch", "b", "", "Explicitly check out a branch in each new worktree")
	cmd.Flags().BoolVar(&tip, "tip", false, "Compatibility flag; detached tip is the default")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if tip {
			_, _ = os.Stderr.WriteString("note: --tip is already the default\n")
		}
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		c, err := wtc.Discover(cwd)
		if err != nil {
			return err
		}
		result, err := c.AddRepositories(wtc.AddRepoOptions{Collection: collection, Repos: args, Branch: branch})
		if err != nil {
			return err
		}
		return emit(envelope{OK: true, Data: result, Summary: "done: added repositories to " + result.Collection}, *asJSON)
	}
	root.AddCommand(cmd)
}
