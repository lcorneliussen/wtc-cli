package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/lcorneliussen/wtc-cli/internal/wtc"
	"github.com/spf13/cobra"
)

func addMCPCommands(root *cobra.Command, asJSON *bool) {
	var collection string
	var dryRun bool
	mcp := &cobra.Command{Use: "mcp", Short: "Render agent MCP configuration"}
	mcp.PersistentFlags().StringVar(&collection, "collection", "", "Collection directory (default: current)")
	render := &cobra.Command{Use: "render", Short: "Render the harness MCP registry into agent configs", Args: cobra.NoArgs}
	render.Flags().BoolVar(&dryRun, "dry-run", false, "Report changes without writing")
	render.RunE = func(cmd *cobra.Command, args []string) error {
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
		if _, err := os.Stat(c.Harness + "/.mcp-servers.yml"); os.IsNotExist(err) {
			return emit(envelope{OK: true, Summary: "MCP registry absent; nothing to render"}, *asJSON)
		} else if err != nil {
			return err
		}
		outputs, err := c.RenderMCP()
		if err != nil {
			return err
		}
		if !dryRun {
			if err := c.RunHook("mcp.render.pre", nil); err != nil {
				return err
			}
		}
		changed, err := c.WriteMCP(outputs, dryRun)
		if err != nil {
			return err
		}
		if !dryRun {
			if err := c.RunHook("mcp.render.post", nil); err != nil {
				return err
			}
		}
		summary := fmt.Sprintf("%d MCP configs changed", len(changed))
		if dryRun {
			summary = fmt.Sprintf("%d MCP configs would change", len(changed))
		}
		if *asJSON {
			return emit(envelope{OK: true, Data: map[string]any{"changed": changed, "missing_env": outputs.Missing, "dry_run": dryRun}, Summary: summary}, true)
		}
		fmt.Println(summary)
		for _, rel := range changed {
			fmt.Println("  " + rel)
		}
		if len(outputs.Missing) > 0 {
			fmt.Fprintln(os.Stderr, "note: unset in this shell: "+strings.Join(outputs.Missing, " "))
		}
		return nil
	}
	mcp.AddCommand(render)
	root.AddCommand(mcp)
}
