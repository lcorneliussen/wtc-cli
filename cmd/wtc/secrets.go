package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/lcorneliussen/wtc-cli/internal/wtc"
	"github.com/spf13/cobra"
)

func addSecretsCommands(root *cobra.Command, asJSON *bool) {
	var collection, repo string
	var dryRun, includeProd bool
	secrets := &cobra.Command{Use: "secrets", Short: "Manage collection secret links"}
	secrets.PersistentFlags().StringVar(&collection, "collection", "", "Collection directory (default: current)")
	link := &cobra.Command{Use: "link", Short: "Link gitignored control-root files into worktrees", Args: cobra.NoArgs}
	link.Flags().StringVar(&repo, "repo", "", "Only link files for this repository")
	link.Flags().BoolVar(&dryRun, "dry-run", false, "Report changes without writing")
	link.Flags().BoolVar(&includeProd, "include-prod", false, "Include configured production-capable paths")
	link.RunE = func(cmd *cobra.Command, args []string) error {
		var c *wtc.Context
		var err error
		if collection == "" {
			var cwd string
			cwd, err = os.Getwd()
			if err == nil {
				c, err = wtc.Discover(cwd)
			}
		} else {
			c, err = wtc.OpenCollection(collection)
		}
		if err != nil {
			return err
		}
		values := map[string]string{"repo": repo, "include_prod": fmt.Sprint(includeProd)}
		if !dryRun {
			if err := c.RunHook("secrets.link.pre", values); err != nil {
				return err
			}
		}
		result, linkErr := c.LinkSecrets(wtc.SecretLinkOptions{Repo: repo, DryRun: dryRun, IncludeProd: includeProd})
		if linkErr == nil && !dryRun {
			if err := c.RunHook("secrets.link.post", values); err != nil {
				return err
			}
		}
		summary := fmt.Sprintf("linked=%d already-current=%d refused=%d prod-skipped=%d backed-up=%d", result.Linked, result.Current, result.Refused, result.ProdSkipped, result.BackedUp)
		if *asJSON {
			if err := emit(envelope{OK: linkErr == nil, Data: result, Summary: summary}, true); err != nil {
				return err
			}
		} else {
			for _, action := range result.Actions {
				fmt.Println(action)
			}
			fmt.Println(summary)
		}
		return linkErr
	}
	secrets.AddCommand(link)
	list := &cobra.Command{Use: "list", Short: "List control-root file names and their link state without reading contents", Args: cobra.NoArgs}
	var listRepo string
	list.Flags().StringVar(&listRepo, "repo", "", "Only list files under this repository's control-root directory")
	list.RunE = func(cmd *cobra.Command, args []string) error {
		var c *wtc.Context
		var err error
		if collection == "" {
			var cwd string
			cwd, err = os.Getwd()
			if err == nil {
				c, err = wtc.Discover(cwd)
			}
		} else {
			c, err = wtc.OpenCollection(collection)
		}
		if err != nil {
			return err
		}
		result, err := c.ListSecrets(listRepo)
		if err != nil {
			return err
		}
		if *asJSON {
			return emit(envelope{OK: true, Data: result, Summary: fmt.Sprintf("%d file(s)", len(result.Files))}, true)
		}
		out := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(out, "PATH\tSCOPE\tTARGET\tSTATE\tIGNORED\tPROD")
		for _, file := range result.Files {
			prod := ""
			if file.Production {
				prod = "yes"
			}
			ignored := ""
			if file.Ignored != nil {
				ignored = "no"
				if *file.Ignored {
					ignored = "yes"
				}
			}
			fmt.Fprintf(out, "%s\t%s\t%s\t%s\t%s\t%s\n", file.Path, file.Scope, file.Target, file.State, ignored, prod)
		}
		return out.Flush()
	}
	secrets.AddCommand(list)
	root.AddCommand(secrets)
}
