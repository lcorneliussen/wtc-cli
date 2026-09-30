package main

import (
	"fmt"
	"strconv"
)

func openSplit(session, pane, direction, ratio, cwd string) (string, error) {
	result, err := openHerdr(session, "pane", "split", pane, "--direction", direction, "--ratio", ratio, "--cwd", cwd, "--no-focus")
	if err != nil {
		return "", err
	}
	id := openFirstPane(result)
	if id == "" {
		return "", fmt.Errorf("herdr did not return a pane id after splitting %s", pane)
	}
	return id, nil
}

func openRenamePane(session, pane, label string) error {
	_, err := openHerdr(session, "pane", "rename", pane, label)
	return err
}

func openLayoutState(session, workspace string) (string, []openPaneInfo, error) {
	panes, err := openPanes(session, workspace)
	if err != nil {
		return "", nil, err
	}
	tabs, err := openTabs(session, workspace)
	if err != nil {
		return "", nil, err
	}
	complete := true
	for _, label := range []string{"agent", "browse", "shell", "status"} {
		if openPaneByLabel(panes, label).ID == "" {
			complete = false
		}
	}
	if openTabByLabel(tabs, "tools").ID != "" {
		if complete {
			return "narrow", panes, nil
		}
		return "partial", panes, nil
	}
	if complete && openSameColumn(session, openPaneByLabel(panes, "agent").ID, openPaneByLabel(panes, "shell").ID) {
		return "wide", panes, nil
	}
	return "partial", panes, nil
}

func openAgentHome(panes []openPaneInfo) string {
	if pane := openPaneByLabel(panes, "agent"); pane.ID != "" {
		return pane.ID
	}
	for _, pane := range panes {
		if pane.Agent != "" {
			return pane.ID
		}
	}
	if len(panes) > 0 {
		return panes[0].ID
	}
	return ""
}

func openApplyWide(session, agent, cwd string) error {
	browse, err := openSplit(session, agent, "right", "0.40", cwd)
	if err != nil {
		return err
	}
	if err := openRenamePane(session, agent, "agent"); err != nil {
		return err
	}
	if err := openRenamePane(session, browse, "browse"); err != nil {
		return err
	}
	shell, err := openSplit(session, agent, "down", "0.80", cwd)
	if err != nil {
		return err
	}
	if err := openRenamePane(session, shell, "shell"); err != nil {
		return err
	}
	status, err := openSplit(session, browse, "down", "0.65", cwd)
	if err != nil {
		return err
	}
	return openRenamePane(session, status, "status")
}

func openApplyNarrow(session, workspace, agent, cwd string) error {
	if tab := openPaneTab(session, agent); tab != "" {
		if _, err := openHerdr(session, "tab", "rename", tab, "main"); err != nil {
			return err
		}
	}
	if err := openRenamePane(session, agent, "agent"); err != nil {
		return err
	}
	status, err := openSplit(session, agent, "down", "0.65", cwd)
	if err != nil {
		return err
	}
	if err := openRenamePane(session, status, "status"); err != nil {
		return err
	}
	created, err := openHerdr(session, "tab", "create", "--workspace", workspace, "--cwd", cwd, "--label", "tools", "--no-focus")
	if err != nil {
		return err
	}
	browse := openFirstPane(created)
	if browse == "" {
		return fmt.Errorf("herdr did not return the tools tab's pane id")
	}
	if err := openRenamePane(session, browse, "browse"); err != nil {
		return err
	}
	shell, err := openSplit(session, browse, "down", "0.80", cwd)
	if err != nil {
		return err
	}
	return openRenamePane(session, shell, "shell")
}

func openSwitchNarrow(session, workspace, agent, cwd string, panes []openPaneInfo) error {
	if err := openRenamePane(session, agent, "agent"); err != nil {
		return err
	}
	if tab := openPaneTab(session, agent); tab != "" {
		if _, err := openHerdr(session, "tab", "rename", tab, "main"); err != nil {
			return err
		}
	}
	browse := openPaneByLabel(panes, "browse").ID
	shell := openPaneByLabel(panes, "shell").ID
	if browse != "" {
		if _, err := openHerdr(session, "pane", "move", browse, "--new-tab", "--workspace", workspace, "--label", "tools", "--no-focus"); err != nil {
			return err
		}
	} else {
		created, err := openHerdr(session, "tab", "create", "--workspace", workspace, "--cwd", cwd, "--label", "tools", "--no-focus")
		if err != nil {
			return err
		}
		browse = openFirstPane(created)
		if browse == "" {
			return fmt.Errorf("herdr did not return a browse pane")
		}
		if err := openRenamePane(session, browse, "browse"); err != nil {
			return err
		}
	}
	tools := openPaneTab(session, browse)
	if shell != "" {
		if _, err := openHerdr(session, "pane", "move", shell, "--tab", tools, "--split", "down", "--target-pane", browse, "--ratio", "0.80", "--no-focus"); err != nil {
			return err
		}
	} else {
		var err error
		shell, err = openSplit(session, browse, "down", "0.80", cwd)
		if err != nil {
			return err
		}
		if err := openRenamePane(session, shell, "shell"); err != nil {
			return err
		}
	}
	if status := openPaneByLabel(panes, "status").ID; status != "" {
		main := openPaneTab(session, agent)
		if _, err := openHerdr(session, "pane", "move", status, "--tab", main, "--split", "down", "--target-pane", agent, "--ratio", "0.65", "--no-focus"); err != nil {
			return err
		}
		return nil
	}
	status, err := openSplit(session, agent, "down", "0.65", cwd)
	if err != nil {
		return err
	}
	return openRenamePane(session, status, "status")
}

func openSwitchWide(session, agent, cwd string, panes []openPaneInfo) error {
	if err := openRenamePane(session, agent, "agent"); err != nil {
		return err
	}
	main := openPaneTab(session, agent)
	if main == "" {
		return fmt.Errorf("cannot find agent tab")
	}
	browse := openPaneByLabel(panes, "browse").ID
	shell := openPaneByLabel(panes, "shell").ID
	if browse != "" {
		if _, err := openHerdr(session, "pane", "move", browse, "--tab", main, "--split", "right", "--target-pane", agent, "--ratio", "0.40", "--no-focus"); err != nil {
			return err
		}
	} else {
		var err error
		browse, err = openSplit(session, agent, "right", "0.40", cwd)
		if err != nil {
			return err
		}
		if err := openRenamePane(session, browse, "browse"); err != nil {
			return err
		}
	}
	if shell != "" {
		if _, err := openHerdr(session, "pane", "move", shell, "--tab", main, "--split", "down", "--target-pane", agent, "--ratio", "0.80", "--no-focus"); err != nil {
			return err
		}
	} else {
		var err error
		shell, err = openSplit(session, agent, "down", "0.80", cwd)
		if err != nil {
			return err
		}
		if err := openRenamePane(session, shell, "shell"); err != nil {
			return err
		}
	}
	if status := openPaneByLabel(panes, "status").ID; status != "" {
		if _, err := openHerdr(session, "pane", "move", status, "--tab", main, "--split", "down", "--target-pane", browse, "--ratio", "0.65", "--no-focus"); err != nil {
			return err
		}
		return nil
	}
	status, err := openSplit(session, browse, "down", "0.65", cwd)
	if err != nil {
		return err
	}
	return openRenamePane(session, status, "status")
}

func openEnsureLayout(session, workspace, cwd, desired string) (string, error) {
	current, panes, err := openLayoutState(session, workspace)
	if err != nil {
		return "", err
	}
	if current == desired {
		return "unchanged", nil
	}
	agent := openAgentHome(panes)
	if agent == "" {
		return "", fmt.Errorf("workspace %s has no pane for the agent", workspace)
	}
	if current == "partial" && len(panes) == 1 {
		if desired == "narrow" {
			err = openApplyNarrow(session, workspace, agent, cwd)
		} else {
			err = openApplyWide(session, agent, cwd)
		}
		return "built", err
	}
	if desired == "narrow" {
		err = openSwitchNarrow(session, workspace, agent, cwd, panes)
	} else {
		err = openSwitchWide(session, agent, cwd, panes)
	}
	return "switched", err
}

func openEnsureBrowse(session, workspace, cwd string) (string, bool, error) {
	panes, err := openPanes(session, workspace)
	if err != nil {
		return "", false, err
	}
	if pane := openPaneByLabel(panes, "browse"); pane.ID != "" {
		return pane.ID, false, nil
	}
	if pane := openPaneByLabel(panes, "tui"); pane.ID != "" {
		if err := openRenamePane(session, pane.ID, "browse"); err != nil {
			return "", false, err
		}
		return pane.ID, false, nil
	}
	tabs, err := openTabs(session, workspace)
	if err != nil {
		return "", false, err
	}
	if openTabByLabel(tabs, "tools").ID != "" {
		return "", false, nil
	}
	agent := openPaneByLabel(panes, "agent").ID
	if agent == "" {
		agent = openPaneByLabel(panes, "shell").ID
	}
	if agent == "" {
		return "", false, nil
	}
	tab := openPaneTab(session, agent)
	count := 0
	for _, pane := range panes {
		if pane.TabID == tab {
			count++
		}
	}
	if count > 3 {
		return "", false, nil
	}
	browse, err := openSplit(session, agent, "right", "0.40", cwd)
	if err != nil {
		return "", false, err
	}
	if err := openRenamePane(session, browse, "browse"); err != nil {
		return "", false, err
	}
	return browse, true, nil
}

func openEnsureStatus(session, workspace, cwd string) (string, bool, error) {
	panes, err := openPanes(session, workspace)
	if err != nil {
		return "", false, err
	}
	if pane := openPaneByLabel(panes, "status"); pane.ID != "" {
		return pane.ID, false, nil
	}
	tabs, err := openTabs(session, workspace)
	if err != nil {
		return "", false, err
	}
	base := ""
	direction, ratio := "down", "0.65"
	if openTabByLabel(tabs, "tools").ID != "" {
		base = openPaneByLabel(panes, "agent").ID
	} else {
		browse := openPaneByLabel(panes, "browse").ID
		shell := openPaneByLabel(panes, "shell").ID
		agent := openPaneByLabel(panes, "agent").ID
		if browse != "" && shell != "" && agent != "" && openSameColumn(session, shell, agent) {
			base = browse
		} else if shell != "" {
			base, direction, ratio = shell, "right", ""
		} else {
			base = browse
		}
	}
	if base == "" {
		return "", false, nil
	}
	var status string
	if ratio != "" {
		status, err = openSplit(session, base, direction, ratio, cwd)
	} else {
		result, callErr := openHerdr(session, "pane", "split", base, "--direction", direction, "--cwd", cwd, "--no-focus")
		err = callErr
		status = openFirstPane(result)
		if err == nil && status == "" {
			err = fmt.Errorf("herdr did not return a status pane")
		}
	}
	if err != nil {
		return "", false, err
	}
	if err := openRenamePane(session, status, "status"); err != nil {
		return "", false, err
	}
	return status, true, nil
}

func openLayoutDescription(layout string, panes []openPaneInfo) string {
	if layout == "" {
		return "unknown"
	}
	return layout + " (" + strconv.Itoa(len(panes)) + " panes)"
}
