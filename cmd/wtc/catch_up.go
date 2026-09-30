package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/lcorneliussen/wtc-cli/internal/wtc"
	"github.com/spf13/cobra"
)

func addCatchUpCommand(root *cobra.Command, asJSON *bool) {
	var opt wtc.CatchUpOptions
	var repos string
	var harnessOnly bool
	cmd := &cobra.Command{Use: "catch-up [collection ...]", Short: "Catch up selected worktree collections"}
	cmd.Flags().BoolVar(&opt.All, "all", false, "Select every collection in the workspace (clean only)")
	cmd.Flags().StringVar(&repos, "repos", "", "Comma-separated sibling or registry names")
	cmd.Flags().BoolVar(&harnessOnly, "harness-only", false, "Select only each collection's harness")
	cmd.Flags().BoolVar(&opt.CleanOnly, "clean-only", false, "Leave dirty worktrees with their owner")
	cmd.Flags().BoolVar(&opt.DryRun, "dry-run", false, "Plan using local refs without fetching or writing")
	cmd.Flags().BoolVar(&opt.ReloadStatus, "reload-status", false, "Restart eligible status panes after the harness updates")
	cmd.Flags().StringVar(&opt.Report, "report", "", "Also write a JSON report and a readable .md companion")
	cmd.Flags().BoolVar(&opt.NoSkills, "no-skills", false, "Skip skill refresh")
	cmd.Flags().BoolVar(&opt.NoMCP, "no-mcp", false, "Skip MCP refresh")
	cmd.Flags().BoolVar(&opt.NoEnv, "no-env", false, "Skip environment refresh")
	cmd.Flags().BoolVar(&opt.NoSecrets, "no-secrets", false, "Skip secret links")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if harnessOnly && repos != "" {
			return fmt.Errorf("--harness-only and --repos cannot be combined")
		}
		if harnessOnly {
			repos = "harness"
		}
		if repos != "" {
			if strings.HasPrefix(repos, ",") || strings.HasSuffix(repos, ",") || strings.Contains(repos, ",,") {
				return fmt.Errorf("invalid repository selector")
			}
			opt.Repos = strings.Split(repos, ",")
		}
		opt.Collections = args
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		c, err := wtc.Discover(cwd)
		if err != nil {
			return err
		}
		report, runErr := c.CatchUp(opt)
		if *asJSON {
			if err := json.NewEncoder(os.Stdout).Encode(report); err != nil {
				return err
			}
		} else {
			fmt.Print(report.Markdown())
		}
		return runErr
	}
	root.AddCommand(cmd)
}
