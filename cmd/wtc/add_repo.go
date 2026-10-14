package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"

	"github.com/lcorneliussen/wtc-cli/internal/wtc"
	"github.com/spf13/cobra"
)

func addAddRepoCommand(root *cobra.Command, asJSON *bool) {
	var collection, branch, name, target string
	var tip bool
	cmd := &cobra.Command{
		Use:   "add-repo <repo|clone-url|path> [...]",
		Short: "Add repository worktrees to a collection",
		Args:  cobra.MinimumNArgs(1),
	}
	cmd.Flags().StringVar(&collection, "collection", "", "Target collection name (default: current)")
	cmd.Flags().StringVarP(&branch, "branch", "b", "", "Explicitly check out a branch in each new worktree")
	cmd.Flags().BoolVar(&tip, "tip", false, "Compatibility flag; detached tip is the default")
	cmd.Flags().StringVar(&name, "name", "", "Ad-hoc: registry name for a new repository (default: from its remote)")
	cmd.Flags().StringVar(&target, "target", "", "Ad-hoc: development branch and PR target for a new repository (default: ask, or the remote's default)")
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
		repos := args
		if c.AdHoc {
			if (name != "" || target != "") && len(args) != 1 {
				return fmt.Errorf("--name and --target describe one repository")
			}
			var ask func(string, string) (string, error)
			if !*asJSON && term.IsTerminal(os.Stdin.Fd()) && term.IsTerminal(os.Stderr.Fd()) {
				in := bufio.NewReader(os.Stdin)
				ask = func(repo, suggested string) (string, error) {
					fmt.Fprintf(os.Stderr, "%s: development branch and PR target [%s]: ", repo, suggested)
					answer, err := in.ReadString('\n')
					return strings.TrimSpace(answer), err
				}
			}
			repos = make([]string, 0, len(args))
			for _, spec := range args {
				repo, err := c.RegisterAdHocRepo(wtc.AdHocRepoOptions{Spec: spec, Name: name, Target: target, Ask: ask})
				if err != nil {
					return err
				}
				repos = append(repos, repo.Name)
			}
		} else if name != "" || target != "" {
			return fmt.Errorf("--name and --target apply to ad-hoc collections; edit the harness registry instead")
		}
		result, err := c.AddRepositories(wtc.AddRepoOptions{Collection: collection, Repos: repos, Branch: branch})
		if err != nil {
			return err
		}
		return emit(envelope{OK: true, Data: result, Summary: "done: added repositories to " + result.Collection}, *asJSON)
	}
	root.AddCommand(cmd)
}
