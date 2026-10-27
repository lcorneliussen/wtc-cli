//go:build !darwin && !linux

package wtc

func readStatusWatcherProcess(pid int) (statusWatcherProcess, bool) {
	return statusWatcherProcess{}, false
}
