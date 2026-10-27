package wtc

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

func readStatusWatcherProcess(pid int) (statusWatcherProcess, bool) {
	info, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil || int(info.Proc.P_pid) != pid {
		return statusWatcherProcess{}, false
	}
	data, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil || len(data) < 4 {
		return statusWatcherProcess{}, false
	}
	argc := int(binary.NativeEndian.Uint32(data[:4]))
	data = data[4:]
	// Skip the executable path and its NUL padding; the next argc strings
	// are argv. Do not inspect the environment that follows them.
	end := bytes.IndexByte(data, 0)
	if end < 0 || argc <= 0 {
		return statusWatcherProcess{}, false
	}
	data = bytes.TrimLeft(data[end:], "\x00")
	args := make([]string, 0, min(argc, 128))
	for range argc {
		end = bytes.IndexByte(data, 0)
		if end < 0 {
			return statusWatcherProcess{}, false
		}
		args = append(args, string(data[:end]))
		data = data[end+1:]
	}
	p := statusWatcherProcess{
		start: fmt.Sprintf("%d:%d", info.Proc.P_starttime.Sec, info.Proc.P_starttime.Usec),
		args:  strings.Join(args, "\x00"),
	}
	output, err := exec.Command("/usr/sbin/lsof", "-a", "-p", strconv.Itoa(pid), "-d", "cwd", "-Fn").Output()
	if err == nil {
		for _, line := range strings.Split(string(output), "\n") {
			if strings.HasPrefix(line, "n/") {
				p.cwd = strings.TrimPrefix(line, "n")
			}
		}
	}
	return p, true
}
