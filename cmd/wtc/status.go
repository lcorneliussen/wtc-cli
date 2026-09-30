package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/lcorneliussen/wtc-cli/internal/wtc"
	"github.com/spf13/cobra"
)

// Explicit previews do not write snapshots that a status pane could mistake
// for a completed one-shot collection.
func addStatusCommand(root *cobra.Command, asJSON *bool) {
	var local, forge, all, md, ansi, reposOnly, cached, noFetch, tui, procs, noClick bool
	var watchSeconds int
	cmd := &cobra.Command{Use: "status", Short: "Inspect worktree collection status", Args: cobra.NoArgs}
	cmd.Flags().BoolVar(&local, "local", false, "Preview local Git facts (no forge facts or cache writes)")
	cmd.Flags().BoolVar(&forge, "forge", false, "Preview local Git and PR facts (no snapshot writes)")
	cmd.Flags().BoolVar(&all, "all", false, "Include every collection in the workspace")
	cmd.Flags().BoolVar(&md, "md", false, "Render agent Markdown")
	cmd.Flags().BoolVar(&ansi, "ansi", false, "Render a terminal table")
	cmd.Flags().BoolVar(&reposOnly, "repos", false, "Show repository rows without the enlisted PR section")
	cmd.Flags().BoolVar(&cached, "cached", false, "Read the last completed snapshot without Git or forge calls")
	cmd.Flags().BoolVar(&noFetch, "no-fetch", false, "Use current local refs without fetching")
	cmd.Flags().BoolVar(&tui, "tui", false, "Run the interactive status view")
	cmd.Flags().BoolVar(&procs, "procs", false, "Show processes under the herdr session")
	cmd.Flags().BoolVar(&noClick, "no-click", false, "Disable build URL mouse targets in the TUI")
	cmd.Flags().IntVar(&watchSeconds, "watch", 0, "Run the interactive view with this refresh interval in seconds")
	cmd.Flags().Lookup("watch").NoOptDefVal = "30"
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		interactive := (tui || watchSeconds > 0) && term.IsTerminal(os.Stdin.Fd()) && term.IsTerminal(os.Stdout.Fd())
		if watchSeconds < 0 {
			return fmt.Errorf("--watch must be nonnegative")
		}
		if procs && (local || forge || cached || all || reposOnly) {
			return fmt.Errorf("--procs cannot be combined with preview, cached, --all, or --repos")
		}
		if cached {
			if local || forge || all || noFetch {
				return fmt.Errorf("--cached cannot be combined with --local, --forge, --all, or --no-fetch")
			}
		} else if local && forge {
			return fmt.Errorf("--local and --forge cannot be combined")
		}
		if forge && all {
			return fmt.Errorf("--forge is currently scoped to this collection")
		}
		if (md && *asJSON) || (ansi && (md || *asJSON)) {
			return fmt.Errorf("--md, --ansi, and --json are mutually exclusive")
		}
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		c, err := wtc.Discover(cwd)
		if err != nil {
			return err
		}
		if !procs && !cmd.Flags().Changed("repos") && statusTrue(statusSetting(c, "WTC_STATUS_REPOS")) {
			reposOnly = true
		}
		if interactive {
			seconds := statusIntervalSetting(c, "WTC_STATUS_WATCH", 30)
			if watchSeconds > 0 {
				seconds = watchSeconds
			}
			background := statusIntervalSetting(c, "WTC_STATUS_WATCH_BG", 300)
			if background == 0 {
				background = seconds
			}
			clickSetting := statusSetting(c, "WTC_STATUS_NO_CLICK")
			if clickSetting == "" {
				clickSetting = statusSetting(c, "HARNESS_STATUS_NO_CLICK")
			}
			if !cmd.Flags().Changed("no-click") && statusTrue(clickSetting) {
				noClick = true
			}
			if seconds > 0 {
				if local || forge || cached || md || ansi || *asJSON {
					return fmt.Errorf("--tui/--watch cannot be combined with preview, cached, Markdown, or JSON output")
				}
				return runStatusTUI(c, all, procs, reposOnly, noFetch, noClick, time.Duration(seconds)*time.Second, time.Duration(background)*time.Second)
			}
		}
		if procs {
			processes, err := c.StatusProcesses()
			if err != nil {
				return err
			}
			if *asJSON {
				return json.NewEncoder(os.Stdout).Encode(processes)
			}
			fmt.Print(wtc.StatusProcessesText(processes))
			return nil
		}
		if cached {
			snapshot, err := c.ReadStatusSnapshot()
			if err == nil {
				if *asJSON {
					return json.NewEncoder(os.Stdout).Encode(snapshot)
				}
				age, err := c.CachedStatusAge()
				if err != nil {
					return err
				}
				if ansi || (!md && term.IsTerminal(os.Stdout.Fd())) {
					fmt.Printf("snapshot %ds old\n%s", age, statusTable(snapshot, reposOnly, statusTerminalWidth()))
					return nil
				}
				body := snapshot.Markdown()
				if reposOnly {
					body = snapshot.ReposMarkdown()
				}
				fmt.Printf("# %s (snapshot, %ds old)\n%s", snapshot.Collection, age, strings.TrimPrefix(body, "# "+snapshot.Collection+"\n"))
				return nil
			}
			if !os.IsNotExist(err) || *asJSON {
				return err
			}
			text, err := c.LegacyStatusText()
			if err != nil {
				return err
			}
			fmt.Print(text)
			return nil
		}
		var snapshot wtc.StatusSnapshot
		var fetched wtc.StatusFetchReport
		if forge {
			snapshot, fetched, err = c.StatusLivePreview(noFetch)
		} else if local {
			snapshot, err = c.StatusLocalSnapshot(all)
		} else {
			snapshot, fetched, err = c.StatusLiveSnapshot(all, noFetch)
		}
		if fetched.Failed > 0 {
			fmt.Fprintf(os.Stderr, "wtc: warning: %d status ref refresh(es) failed; showing local refs\n", fetched.Failed)
		}
		if err != nil {
			return err
		}
		if *asJSON {
			return json.NewEncoder(os.Stdout).Encode(snapshot)
		}
		if ansi || (!md && term.IsTerminal(os.Stdout.Fd())) {
			fmt.Print(statusTable(snapshot, reposOnly, statusTerminalWidth()))
			return nil
		}
		if reposOnly {
			fmt.Print(snapshot.ReposMarkdown())
		} else {
			fmt.Print(snapshot.Markdown())
		}
		return nil
	}
	root.AddCommand(cmd)
}

func statusTerminalWidth() int {
	width, _, err := term.GetSize(os.Stdout.Fd())
	if err != nil || width <= 0 {
		return 80
	}
	return width
}

func statusSetting(c *wtc.Context, name string) string {
	if value, ok := os.LookupEnv(name); ok && value != "" {
		return value
	}
	file, err := os.Open(filepath.Join(c.ConfigRoot, "wtc.env"))
	if err != nil {
		return ""
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	value := ""
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		line = strings.TrimPrefix(line, "export ")
		key, setting, found := strings.Cut(line, "=")
		if !found || strings.TrimSpace(key) != name {
			continue
		}
		value = strings.Trim(strings.TrimSpace(setting), "'\"")
	}
	return value
}

func statusTrue(value string) bool {
	switch strings.ToLower(value) {
	case "yes", "true", "1":
		return true
	}
	return false
}

func statusIntervalSetting(c *wtc.Context, name string, fallback int) int {
	raw := statusSetting(c, name)
	if raw == "" && name == "WTC_STATUS_WATCH" {
		raw = statusSetting(c, "HARNESS_STATUS_WATCH")
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return fallback
	}
	return value
}
