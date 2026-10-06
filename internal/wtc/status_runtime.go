package wtc

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

func (c *Context) statusRuntime(snapshot *StatusSnapshot, all bool) {
	if !all {
		snapshot.Runtime = c.RuntimeStatus()
	} else {
		// A sweep uses this collection's trusted executable, never a binary
		// selected by a foreign collection's manifest or hook.
		binary, binaryErr := c.runtimeBinary()
		dirs, err := WorkspaceCollections(c.Workspace)
		if err != nil {
			return
		}
		for _, dir := range dirs {
			target, err := OpenCollection(dir)
			if err != nil {
				continue
			}
			m, err := target.runtimeManifest()
			if err != nil {
				snapshot.Runtime = append(snapshot.Runtime, RuntimeItem{Collection: filepath.Base(dir), Path: "runtime", State: "unknown", Error: err.Error()})
				continue
			}
			if m == nil {
				continue
			}
			if binaryErr != nil {
				for _, task := range m.Tasks {
					item := task.RuntimeItem
					item.State = "unknown"
					item.Error = binaryErr.Error()
					snapshot.Runtime = append(snapshot.Runtime, item)
				}
				continue
			}
			m.Binary = binary
			snapshot.Runtime = append(snapshot.Runtime, target.runtimeStatus(m)...)
		}
	}
	for i := range snapshot.Repos {
		row := &snapshot.Repos[i]
		counts := map[string]int{}
		for _, item := range snapshot.Runtime {
			if item.Repo == row.Repo && item.Collection == row.Collection {
				state := item.State
				if item.Kind == "resources" && state == "provision-hook-completed" {
					state = "provisioned"
				}
				counts[state]++
			}
		}
		states := []string{}
		for state := range counts {
			states = append(states, state)
		}
		// Keep service state visible in a narrow repo column. The detailed
		// list retains the exact resource-hook fact rather than implying health.
		sort.Slice(states, func(i, j int) bool {
			rank := func(state string) int {
				switch state {
				case "ready":
					return 1
				case "provisioned":
					return 2
				default:
					return 0
				}
			}
			if rank(states[i]) != rank(states[j]) {
				return rank(states[i]) < rank(states[j])
			}
			return states[i] < states[j]
		})
		parts := []string{}
		for _, state := range states {
			parts = append(parts, fmt.Sprintf("%d %s", counts[state], state))
		}
		row.Runtime = strings.Join(parts, ", ")
	}
}

func RuntimeText(items []RuntimeItem) string {
	if len(items) == 0 {
		return ""
	}
	items = append([]RuntimeItem(nil), items...)
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].Collection+"/"+items[i].Group+"/"+items[i].Path < items[j].Collection+"/"+items[j].Group+"/"+items[j].Path
	})
	var b strings.Builder
	b.WriteString("Runtime (dekit; ready = startup probe passed)\n")
	group := ""
	for _, item := range items {
		key := item.Collection + "/" + item.Group
		if key != group {
			fmt.Fprintf(&b, "  %s\n", key)
			group = key
		}
		name := strings.TrimPrefix(item.Path, item.Group+"/")
		if name == item.Path {
			name = filepath.Base(item.Path)
		}
		fmt.Fprintf(&b, "    %-12s %-10s %s", name, item.Kind, item.State)
		if item.Label != "" {
			fmt.Fprintf(&b, " · %s", item.Label)
		}
		if item.URL != "" {
			fmt.Fprintf(&b, " · %s", item.URL)
		}
		if item.ExitCode != nil {
			fmt.Fprintf(&b, " · exit %d", *item.ExitCode)
		}
		if item.Signal != nil {
			fmt.Fprintf(&b, " · signal %d", *item.Signal)
		}
		if item.Reason != "" {
			fmt.Fprintf(&b, " · %s", item.Reason)
		}
		if item.Error != "" {
			fmt.Fprintf(&b, " · %s", item.Error)
		}
		b.WriteByte('\n')
	}
	return b.String()
}
