package wtc

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type StatusProcess struct {
	PID     int     `json:"pid"`
	PPID    int     `json:"ppid"`
	CPU     float64 `json:"cpu"`
	Memory  float64 `json:"memory"`
	RSSKiB  int     `json:"rss_kib"`
	Command string  `json:"command"`
}

func (c *Context) statusHerdrSession() string {
	if session := os.Getenv("HARNESS_HERDR_SESSION"); session != "" {
		return session
	}
	if c.Config.Herdr.Session != "" {
		return c.Config.Herdr.Session
	}
	name := filepath.Base(c.Workspace)
	name = strings.TrimSuffix(name, "-harness")
	return strings.TrimSuffix(name, "-wtc")
}

func statusParseProcesses(output, session string) []StatusProcess {
	all := []StatusProcess{}
	rootPID := 0
	needle := "herdr --session " + session + " server"
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		pid, err1 := strconv.Atoi(fields[0])
		ppid, err2 := strconv.Atoi(fields[1])
		cpu, err3 := strconv.ParseFloat(fields[2], 64)
		mem, err4 := strconv.ParseFloat(fields[3], 64)
		rss, err5 := strconv.Atoi(fields[4])
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil || err5 != nil {
			continue
		}
		command := strings.Join(fields[5:], " ")
		if rootPID == 0 && strings.Contains(command, needle) {
			rootPID = pid
		}
		all = append(all, StatusProcess{PID: pid, PPID: ppid, CPU: cpu, Memory: mem, RSSKiB: rss, Command: command})
	}
	if rootPID == 0 {
		return []StatusProcess{}
	}
	keep := map[int]bool{rootPID: true}
	for changed := true; changed; {
		changed = false
		for _, process := range all {
			if keep[process.PPID] && !keep[process.PID] {
				keep[process.PID] = true
				changed = true
			}
		}
	}
	selected := []StatusProcess{}
	for _, process := range all {
		if process.PID != rootPID && keep[process.PID] {
			selected = append(selected, process)
		}
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].CPU > selected[j].CPU })
	return selected
}

func (c *Context) StatusProcesses() ([]StatusProcess, error) {
	out, err := statusCommand(10*time.Second, "ps", "-axo", "pid=,ppid=,pcpu=,pmem=,rss=,args=")
	if err != nil {
		return nil, fmt.Errorf("ps: %w", err)
	}
	return statusParseProcesses(string(out), c.statusHerdrSession()), nil
}

func StatusProcessesText(processes []StatusProcess) string {
	if len(processes) == 0 {
		return "(no processes under the herdr session)\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%7s %6s %6s %9s  %s\n", "PID", "%CPU", "%MEM", "RSS", "COMMAND")
	for _, process := range processes {
		command := process.Command
		if chars := []rune(command); len(chars) > 58 {
			command = string(chars[:58])
		}
		fmt.Fprintf(&b, "%7d %6.1f %6.1f %8.0fM  %s\n", process.PID, process.CPU, process.Memory, float64(process.RSSKiB)/1024, command)
	}
	return b.String()
}
