package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/lcorneliussen/wtc-cli/internal/wtc"
	"github.com/spf13/cobra"
)

func addNewCommand(root *cobra.Command, asJSON *bool) {
	var issue, tracker, pr, branch string
	var open, noOpen, tip bool
	cmd := &cobra.Command{Use: "new [slug] [repo ...]", Short: "Create a worktree collection", Args: cobra.ArbitraryArgs}
	cmd.Flags().StringVar(&issue, "issue", "", "Issue ID whose repository becomes the primary sibling")
	cmd.Flags().StringVar(&tracker, "tracker", "", "Tracker key for the launch note")
	cmd.Flags().StringVar(&pr, "pr", "", "Pull request in <repo>#<number> form")
	cmd.Flags().StringVarP(&branch, "branch", "b", "", "Explicitly check out this branch in each sibling")
	cmd.Flags().BoolVar(&tip, "tip", false, "Compatibility flag; detached tip is the default")
	cmd.Flags().BoolVar(&open, "open", false, "Open the new collection in herdr")
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "Leave the collection unopened")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if open && noOpen {
			return fmt.Errorf("--open and --no-open cannot be combined")
		}
		if pr == "" && len(args) == 0 {
			return fmt.Errorf("a collection slug is required")
		}
		if tip {
			fmt.Fprintln(os.Stderr, "note: --tip is already the default")
		}
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		c, err := wtc.Discover(cwd)
		if err != nil {
			return err
		}
		opt := wtc.NewOptions{Issue: issue, Tracker: tracker, PR: pr, Branch: branch}
		if pr == "" {
			opt.Slug, opt.Repos = args[0], args[1:]
		} else {
			opt.Repos = args
		}
		result, err := c.NewCollection(opt)
		if err != nil {
			return err
		}
		shouldOpen := open
		if !open && !noOpen {
			if _, err := exec.LookPath("herdr"); err == nil {
				session := c.Config.Herdr.Session
				if session == "" {
					session = strings.TrimSuffix(strings.TrimSuffix(filepath.Base(c.Workspace), "-harness"), "-wtc")
				}
				probe := exec.Command("herdr", "--session", session, "workspace", "list")
				shouldOpen = probe.Run() == nil
			}
		}
		if shouldOpen {
			path := filepath.Join(result.Collection, "harness", "tools", "wtc-open.sh")
			if _, err := os.Stat(path); err != nil {
				return fmt.Errorf("collection created at %s, but open tool is unavailable: %w", result.Collection, err)
			}
			openCmd := exec.Command(path, filepath.Base(result.Collection))
			openCmd.Dir = result.Collection
			openCmd.Stdout, openCmd.Stderr = os.Stderr, os.Stderr
			if err := openCmd.Run(); err != nil {
				return fmt.Errorf("collection created at %s, but opening failed: %w", result.Collection, err)
			}
		}
		return emit(envelope{OK: true, Data: result, Summary: "done: " + result.Collection}, *asJSON)
	}
	root.AddCommand(cmd)
}
