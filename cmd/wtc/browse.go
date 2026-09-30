package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lcorneliussen/wtc-cli/internal/wtc"
	"github.com/spf13/cobra"
)

type browseHerdrResult struct {
	Result struct {
		Workspaces []struct {
			Label string `json:"label"`
			ID    string `json:"workspace_id"`
		} `json:"workspaces"`
		Panes []struct {
			Label string `json:"label"`
			ID    string `json:"pane_id"`
			TabID string `json:"tab_id"`
		} `json:"panes"`
		Tabs []struct {
			Label string `json:"label"`
			ID    string `json:"tab_id"`
		} `json:"tabs"`
		Agents []struct {
			PaneID string `json:"pane_id"`
		} `json:"agents"`
		PaneID string `json:"pane_id"`
		TabID  string `json:"tab_id"`
		Pane   struct {
			ID string `json:"pane_id"`
		} `json:"pane"`
		RootPane struct {
			ID string `json:"pane_id"`
		} `json:"root_pane"`
		Tab struct {
			ID string `json:"tab_id"`
		} `json:"tab"`
		ProcessInfo struct {
			Group     int `json:"foreground_process_group_id"`
			Processes []struct {
				PID  int      `json:"pid"`
				Name string   `json:"name"`
				Argv []string `json:"argv"`
			} `json:"foreground_processes"`
		} `json:"process_info"`
	} `json:"result"`
}

func addBrowseCommand(root *cobra.Command, asJSON *bool) {
	var here bool
	var session string
	cmd := &cobra.Command{Use: "browse [collection]", Short: "Open the collection's Neovim browser", Args: cobra.MaximumNArgs(1)}
	cmd.Flags().BoolVar(&here, "here", false, "Open Neovim in this terminal, including an agent pane")
	cmd.Flags().Bool("no-focus", false, "Compatibility flag; browse keeps the current herdr focus")
	cmd.Flags().StringVar(&session, "session", "", "Herdr session (defaults to the workspace session)")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if *asJSON {
			return errors.New("browse is interactive and does not support --json")
		}
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		c, err := wtc.Discover(cwd)
		if err != nil {
			return err
		}
		if len(args) == 1 {
			name := args[0]
			if name == "" || name == "." || name == ".." || filepath.Base(name) != name {
				return fmt.Errorf("expected a collection name")
			}
			c, err = wtc.OpenCollection(filepath.Join(c.Workspace, name))
			if err != nil {
				return err
			}
		}
		if _, err := exec.LookPath("nvim"); err != nil {
			return errors.New("nvim is not on PATH")
		}
		session = browseSession(c, session)
		if !here {
			callerAgent, err := browseCallerIsAgent()
			if err != nil {
				return err
			}
			if callerAgent {
				return browseInPane(c, session)
			}
		}
		return browseHere(c)
	}
	root.AddCommand(cmd)
}

func browseSession(c *wtc.Context, requested string) string {
	if requested != "" {
		return requested
	}
	if active := os.Getenv("HERDR_SESSION"); active != "" {
		return active
	}
	if configured := statusSetting(c, "HARNESS_HERDR_SESSION"); configured != "" {
		return configured
	}
	if c.Config.Herdr.Session != "" {
		return c.Config.Herdr.Session
	}
	return strings.TrimSuffix(strings.TrimSuffix(filepath.Base(c.Workspace), "-harness"), "-wtc")
}

func browseHerdr(session string, args ...string) (browseHerdrResult, error) {
	var result browseHerdrResult
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	argv := append([]string{"--session", session}, args...)
	out, err := exec.CommandContext(ctx, "herdr", argv...).CombinedOutput()
	if err != nil {
		return result, fmt.Errorf("herdr %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	if len(out) == 0 {
		return result, nil
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return result, fmt.Errorf("herdr %s returned invalid JSON: %w", strings.Join(args, " "), err)
	}
	return result, nil
}

func browseCallerIsAgent() (bool, error) {
	callerSession := os.Getenv("HERDR_SESSION")
	if os.Getenv("HERDR_ENV") != "1" || os.Getenv("HERDR_PANE_ID") == "" || callerSession == "" {
		return false, nil
	}
	result, err := browseHerdr(callerSession, "agent", "list")
	if err != nil {
		return false, fmt.Errorf("cannot determine whether this herdr pane belongs to an agent: %w", err)
	}
	for _, agent := range result.Result.Agents {
		if agent.PaneID == os.Getenv("HERDR_PANE_ID") {
			return true, nil
		}
	}
	return false, nil
}

func browseHere(c *wtc.Context) error {
	body, err := browseView(c)
	if err != nil {
		return err
	}
	lua, err := os.CreateTemp("", "wtc-browse-*.lua")
	if err != nil {
		return err
	}
	defer os.Remove(lua.Name())
	if _, err := lua.Write(body); err != nil {
		lua.Close()
		return err
	}
	if err := lua.Close(); err != nil {
		return err
	}
	socket := browseSocket(c.Workspace, filepath.Base(c.Collection))
	argv := []string{}
	probeContext, stopProbe := context.WithTimeout(context.Background(), 3*time.Second)
	probe := exec.CommandContext(probeContext, "nvim", "--server", socket, "--remote-expr", "1")
	probeErr := probe.Run()
	probeTimedOut := probeContext.Err() != nil
	stopProbe()
	if probeTimedOut {
		return fmt.Errorf("browse Neovim at %s did not answer within three seconds", socket)
	}
	if probeErr != nil {
		if err := os.Remove(socket); err != nil && !os.IsNotExist(err) {
			return err
		}
		argv = append(argv, "--listen", socket)
	} else {
		fmt.Fprintf(os.Stderr, "==> %s: browse Neovim already listens on %s; opening another window\n", filepath.Base(c.Collection), socket)
	}
	argv = append(argv,
		"-c", "lua vim.g.wtc_browse_root = "+strconv.Quote(c.Collection),
		"-c", "lua dofile("+strconv.Quote(lua.Name())+")")
	process := exec.Command("nvim", argv...)
	process.Dir = c.Collection
	process.Stdin, process.Stdout, process.Stderr = os.Stdin, os.Stdout, os.Stderr
	return process.Run()
}

func browseView(c *wtc.Context) ([]byte, error) {
	override := filepath.Join(c.Harness, "overlays", "browse", "wtc-browse.lua")
	if info, err := os.Stat(override); err == nil {
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("browse overlay is not a regular file: %s", override)
		}
		return os.ReadFile(override)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return wtc.ReadDefault("browse/wtc-browse.lua")
}

// POSIX cksum is also used by the shell compatibility shim and status clicks.
func browseChecksum(data string) uint32 {
	const polynomial uint32 = 0x04c11db7
	crc := uint32(0)
	step := func(value byte) {
		crc ^= uint32(value) << 24
		for i := 0; i < 8; i++ {
			if crc&0x80000000 != 0 {
				crc = (crc << 1) ^ polynomial
			} else {
				crc <<= 1
			}
		}
	}
	for i := 0; i < len(data); i++ {
		step(data[i])
	}
	for n := len(data); n > 0; n >>= 8 {
		step(byte(n))
	}
	return ^crc
}

func browseSocket(workspace, collection string) string {
	path := "/tmp/wtc-browse-" + filepath.Base(workspace) + "-" + collection + ".nvim"
	if len(path) <= 100 {
		return path
	}
	stem := filepath.Base(workspace) + "-" + collection
	if len(stem) > 60 {
		stem = stem[:60]
	}
	return fmt.Sprintf("/tmp/wtc-browse-%s-%d.nvim", stem, browseChecksum(workspace+"/"+collection))
}

func browseInPane(c *wtc.Context, session string) error {
	result, err := browseHerdr(session, "workspace", "list")
	if err != nil {
		return fmt.Errorf("herdr session is unavailable: %w", err)
	}
	workspaceID := ""
	for _, workspace := range result.Result.Workspaces {
		if workspace.Label == filepath.Base(c.Collection) {
			workspaceID = workspace.ID
			break
		}
	}
	if workspaceID == "" {
		return fmt.Errorf("collection %s has no herdr workspace; run wtc open first or use --here", filepath.Base(c.Collection))
	}
	panes, err := browseHerdr(session, "pane", "list", "--workspace", workspaceID)
	if err != nil {
		return err
	}
	paneID := ""
	for _, pane := range panes.Result.Panes {
		if pane.Label == "browse" {
			paneID = pane.ID
			break
		}
	}
	if paneID == "" {
		for _, pane := range panes.Result.Panes {
			if pane.Label == "tui" {
				paneID = pane.ID
				_, err = browseHerdr(session, "pane", "rename", paneID, "browse")
				if err != nil {
					return err
				}
				break
			}
		}
	}
	if paneID == "" {
		paneID, err = browseEnsurePane(c, session, workspaceID, panes.Result.Panes)
		if err != nil {
			return err
		}
	}
	state, err := browsePaneState(session, paneID)
	if err != nil {
		return err
	}
	switch state {
	case "nvim":
		fmt.Printf("==> %s: Neovim already in %s\n", filepath.Base(c.Collection), paneID)
	case "idle":
		command, err := browsePaneCommand(c)
		if err != nil {
			return err
		}
		_, err = browseHerdr(session, "pane", "run", paneID, command)
		if err != nil {
			return err
		}
		fmt.Printf("==> %s: Neovim in %s\n", filepath.Base(c.Collection), paneID)
	default:
		return fmt.Errorf("browse pane %s is busy; quit its program or use --here", paneID)
	}
	browseEnsurePRTab(c, session, workspaceID)
	return nil
}

func browseShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func browsePaneCommand(c *wtc.Context) (string, error) {
	shim := filepath.Join(c.Harness, "tools", "wtc-browse.sh")
	if info, err := os.Stat(shim); err == nil && info.Mode().IsRegular() && info.Mode()&0111 != 0 {
		return browseShellQuote(shim) + " --here", nil
	} else if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	return "cd " + browseShellQuote(c.Collection) + " && wtc browse --here", nil
}

func browseEnsurePane(c *wtc.Context, session, workspaceID string, panes []struct {
	Label string `json:"label"`
	ID    string `json:"pane_id"`
	TabID string `json:"tab_id"`
}) (string, error) {
	var agent, shell string
	for _, pane := range panes {
		switch pane.Label {
		case "agent":
			agent = pane.ID
		case "shell":
			shell = pane.ID
		}
	}
	if agent == "" {
		if shell != "" {
			return shell, nil
		}
		return "", errors.New("workspace has no browse, agent, or shell pane")
	}
	// A narrow workspace has its browse pane on the tools tab. Avoid adding a
	// new column to main; an off-template workspace also uses its shell pane.
	tabs, err := browseHerdr(session, "tab", "list", "--workspace", workspaceID)
	if err != nil {
		return "", err
	}
	for _, tab := range tabs.Result.Tabs {
		if tab.Label == "tools" && shell != "" {
			return shell, nil
		}
	}
	agentTab := ""
	for _, pane := range panes {
		if pane.ID == agent {
			agentTab = pane.TabID
		}
	}
	count := 0
	for _, pane := range panes {
		if pane.TabID == agentTab {
			count++
		}
	}
	if count > 3 && shell != "" {
		return shell, nil
	}
	created, err := browseHerdr(session, "pane", "split", agent, "--direction", "right", "--ratio", "0.40", "--cwd", c.Collection, "--no-focus")
	if err != nil {
		return "", err
	}
	id := created.Result.PaneID
	if id == "" {
		id = created.Result.Pane.ID
	}
	if id == "" && len(created.Result.Panes) > 0 {
		id = created.Result.Panes[0].ID
	}
	if id == "" {
		return "", errors.New("herdr did not return a new pane id")
	}
	if _, err := browseHerdr(session, "pane", "rename", id, "browse"); err != nil {
		return "", err
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		state, _ := browsePaneState(session, id)
		if state == "idle" {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	return id, nil
}

func browsePaneState(session, paneID string) (string, error) {
	result, err := browseHerdr(session, "pane", "process-info", "--pane", paneID)
	if err != nil {
		return "", err
	}
	for _, process := range result.Result.ProcessInfo.Processes {
		if process.PID != result.Result.ProcessInfo.Group {
			continue
		}
		if filepath.Base(process.Name) == "nvim" {
			return "nvim", nil
		}
		if len(process.Argv) <= 1 {
			name := filepath.Base(strings.TrimPrefix(process.Name, "-"))
			switch name {
			case "bash", "zsh", "sh", "fish", "nu", "nushell", "dash", "ksh":
				return "idle", nil
			}
		}
		return "busy", nil
	}
	return "idle", nil
}

func browseEnsurePRTab(c *wtc.Context, session, workspaceID string) {
	if _, err := exec.LookPath("gh"); err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if exec.CommandContext(ctx, "gh", "dash", "-h").Run() != nil {
		return
	}
	tabs, err := browseHerdr(session, "tab", "list", "--workspace", workspaceID)
	if err != nil {
		return
	}
	tabID := ""
	for _, tab := range tabs.Result.Tabs {
		if tab.Label == "pr" {
			tabID = tab.ID
		}
	}
	if tabID == "" {
		created, err := browseHerdr(session, "tab", "create", "--workspace", workspaceID, "--cwd", c.Collection, "--label", "pr", "--no-focus")
		if err != nil {
			return
		}
		tabID = created.Result.TabID
		if tabID == "" {
			tabID = created.Result.Tab.ID
		}
		if tabID == "" && len(created.Result.Tabs) > 0 {
			tabID = created.Result.Tabs[0].ID
		}
		paneID := created.Result.PaneID
		if paneID == "" {
			paneID = created.Result.RootPane.ID
		}
		if paneID == "" && len(created.Result.Panes) > 0 {
			paneID = created.Result.Panes[0].ID
		}
		if paneID != "" {
			_, _ = browseHerdr(session, "pane", "rename", paneID, "pr")
			deadline := time.Now().Add(10 * time.Second)
			for time.Now().Before(deadline) {
				state, _ := browsePaneState(session, paneID)
				if state == "idle" {
					break
				}
				time.Sleep(250 * time.Millisecond)
			}
		}
	}
	panes, err := browseHerdr(session, "pane", "list", "--workspace", workspaceID)
	if err != nil {
		return
	}
	for _, pane := range panes.Result.Panes {
		if pane.Label != "pr" {
			continue
		}
		state, err := browsePaneState(session, pane.ID)
		if err == nil && state == "idle" {
			_, _ = browseHerdr(session, "pane", "run", pane.ID, "gh dash")
		}
		return
	}
}
