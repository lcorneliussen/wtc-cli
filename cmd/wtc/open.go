package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/lcorneliussen/wtc-cli/internal/wtc"
	"github.com/spf13/cobra"
)

type openOptions struct {
	All, List, DryRun, Focus                                    bool
	NoAgent, NoBrowse, NoStatus, NoFirstPrompt, NoRemoteControl bool
	Session, Agent, AgentArgs                                   string
	AgentArgsSet                                                bool
	Layout                                                      string
	LayoutSet                                                   bool
}

type openItem struct {
	Collection string   `json:"collection"`
	Workspace  string   `json:"workspace,omitempty"`
	Layout     string   `json:"layout,omitempty"`
	Actions    []string `json:"actions"`
	Error      string   `json:"error,omitempty"`
}

func addOpenCommand(root *cobra.Command, asJSON *bool) {
	var opt openOptions
	cmd := &cobra.Command{Use: "open [collection ...]", Short: "Open or repair collection workspaces in herdr", Args: cobra.ArbitraryArgs}
	cmd.Flags().BoolVar(&opt.All, "all", false, "Open every collection in this workspace")
	cmd.Flags().BoolVar(&opt.List, "list", false, "Report workspace and pane state without changing it")
	cmd.Flags().BoolVar(&opt.DryRun, "dry-run", false, "Show needed changes without making them")
	cmd.Flags().StringVar(&opt.Session, "session", "", "Herdr session")
	cmd.Flags().StringVar(&opt.Agent, "agent", "", "Agent kind to start (default: claude)")
	cmd.Flags().StringVar(&opt.AgentArgs, "agent-args", "", "Arguments passed to the agent, replacing defaults")
	cmd.Flags().BoolVar(&opt.NoRemoteControl, "no-remote-control", false, "Do not enable Claude Remote Control")
	cmd.Flags().BoolVar(&opt.NoAgent, "no-agent", false, "Leave the agent pane at a shell prompt")
	cmd.Flags().BoolVar(&opt.NoFirstPrompt, "no-first-prompt", false, "Do not submit the first prompt for a fresh collection")
	cmd.Flags().BoolVar(&opt.NoBrowse, "no-browse", false, "Leave the browse pane at a shell prompt")
	cmd.Flags().BoolVar(&opt.NoStatus, "no-status", false, "Leave the status pane at a shell prompt")
	cmd.Flags().BoolVar(&opt.Focus, "focus", false, "Focus the last opened workspace")
	cmd.Flags().Bool("narrow", false, "Use two stacked tabs")
	cmd.Flags().Bool("wide", false, "Use two stacked columns")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if opt.All && len(args) != 0 {
			return errors.New("--all cannot be combined with collection names")
		}
		narrow, _ := cmd.Flags().GetBool("narrow")
		wide, _ := cmd.Flags().GetBool("wide")
		if narrow && wide {
			return errors.New("choose --narrow or --wide")
		}
		if narrow {
			opt.Layout, opt.LayoutSet = "narrow", true
		}
		if wide {
			opt.Layout, opt.LayoutSet = "wide", true
		}
		opt.AgentArgsSet = cmd.Flags().Changed("agent-args")
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		c, err := wtc.Discover(cwd)
		if err != nil {
			return err
		}
		if _, err := exec.LookPath("herdr"); err != nil {
			return errors.New("herdr is not installed")
		}
		names, err := openSelectedCollections(c, opt.All, args)
		if err != nil {
			return err
		}
		opt.Session = openSession(c, opt.Session)
		running := openSessionRunning(opt.Session)
		if !running && !opt.List && !opt.DryRun {
			if err := openEnsureSession(opt.Session); err != nil {
				return err
			}
			running = true
		}
		layout := openDesiredLayout(c, opt, running)
		items := make([]openItem, 0, len(names))
		failures := 0
		for i, name := range names {
			item := openCollection(c, name, opt, layout, running, i == len(names)-1)
			if item.Error != "" {
				failures++
			}
			items = append(items, item)
		}
		if *asJSON {
			if err := emit(envelope{OK: failures == 0, Data: items, Summary: fmt.Sprintf("%d collection(s), %d failed", len(items), failures)}, true); err != nil {
				return err
			}
		} else {
			if opt.List && !running {
				fmt.Printf("herdr session %q: not running\n", opt.Session)
			} else {
				for _, item := range items {
					fmt.Printf("==> %s: %s — %s\n", item.Collection, item.Workspace, strings.Join(item.Actions, ", "))
					if item.Error != "" {
						fmt.Fprintln(os.Stderr, "error:", item.Error)
					}
				}
			}
			fmt.Printf("attach: herdr --session %s\n", opt.Session)
		}
		if failures != 0 {
			return fmt.Errorf("%d collection(s) could not be opened", failures)
		}
		return nil
	}
	root.AddCommand(cmd)
}

func openSelectedCollections(c *wtc.Context, all bool, args []string) ([]string, error) {
	if !all && len(args) == 0 {
		return []string{filepath.Base(c.Collection)}, nil
	}
	if !all {
		for _, name := range args {
			if name == "" || name == "." || name == ".." || filepath.Base(name) != name {
				return nil, fmt.Errorf("invalid collection name %q", name)
			}
		}
		return args, nil
	}
	entries, err := os.ReadDir(c.Workspace)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		if _, err := os.Stat(filepath.Join(c.Workspace, entry.Name(), "harness")); err == nil {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil, fmt.Errorf("no collections found under %s", c.Workspace)
	}
	return names, nil
}

func openSession(c *wtc.Context, requested string) string {
	if requested != "" {
		return requested
	}
	if configured := statusSetting(c, "HARNESS_HERDR_SESSION"); configured != "" {
		return configured
	}
	if c.Config.Herdr.Session != "" {
		return c.Config.Herdr.Session
	}
	return strings.TrimSuffix(strings.TrimSuffix(filepath.Base(c.Workspace), "-harness"), "-wtc")
}

func openDesiredLayout(c *wtc.Context, opt openOptions, running bool) string {
	if opt.LayoutSet {
		return opt.Layout
	}
	if preference := statusSetting(c, "WTC_LAYOUT"); preference == "wide" || preference == "narrow" {
		return preference
	}
	if running {
		if width := openSessionWidth(opt.Session); width > 0 {
			at, err := strconv.Atoi(statusSetting(c, "WTC_LAYOUT_NARROW_AT"))
			if err != nil || at <= 0 {
				at = 140
			}
			if width < at {
				return "narrow"
			}
		}
	}
	return "wide"
}
