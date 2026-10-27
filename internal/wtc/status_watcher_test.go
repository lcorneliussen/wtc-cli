package wtc

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Re-exec only self-created fixtures. The overridden argv exercises process
// discovery without starting real status collectors or touching host panes.
func init() {
	mode := os.Getenv("WTC_TEST_STATUS_WATCHER")
	if mode == "" {
		return
	}
	if mode == "registered" {
		stopped := make(chan struct{})
		c, err := OpenCollection(os.Getenv("WTC_TEST_STATUS_COLLECTION"))
		if err != nil {
			os.Exit(2)
		}
		cleanup, err := c.RegisterStatusWatcher(func() { close(stopped) })
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		fmt.Println("ready")
		<-stopped
		cleanup()
		os.Exit(0)
	}
	fmt.Println("ready")
	for {
		time.Sleep(time.Hour)
	}
}

func watcherCollection(t *testing.T) *Context {
	t.Helper()
	target := t.TempDir()
	fixtureFile(t, filepath.Join(target, AdHocConfigDir, adHocRegistryFile), "repos: []\n", 0600)
	c, err := OpenCollection(target)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func watcherFixture(t *testing.T, collection, cwd, mode string, args ...string) (*exec.Cmd, <-chan error) {
	t.Helper()
	cmd := exec.Command(os.Args[0])
	cmd.Args = args
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "WTC_TEST_STATUS_WATCHER="+mode, "WTC_TEST_STATUS_COLLECTION="+collection)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	ready := make(chan bool, 1)
	go func() { ready <- bufio.NewScanner(stdout).Scan() }()
	select {
	case ok := <-ready:
		if !ok {
			t.Fatal("fixture did not become ready")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("fixture startup timed out")
	}
	return cmd, exited
}

func waitWatcherExit(t *testing.T, exited <-chan error) {
	t.Helper()
	select {
	case <-exited:
	case <-time.After(10 * time.Second):
		t.Fatal("watcher outlived retirement")
	}
}

func TestRegisteredStatusWatchersRetireByResolvedCollection(t *testing.T) {
	target, other := watcherCollection(t).Collection, watcherCollection(t).Collection
	_, first := watcherFixture(t, target, other, "registered", "wtc", "status", filepath.Base(target), "--tui")
	_, second := watcherFixture(t, target, target, "registered", "./taba", "status", "--tui")
	_, unrelated := watcherFixture(t, other, target, "registered", "wtc", "status", "--tui")
	if err := stopRegisteredStatusWatchers(target); err != nil {
		t.Fatal(err)
	}
	waitWatcherExit(t, first)
	waitWatcherExit(t, second)
	select {
	case err := <-unrelated:
		t.Fatalf("unrelated watcher stopped: %v", err)
	default:
	}
	if _, err := os.Stat(filepath.Join(target, statusWatcherDir)); !os.IsNotExist(err) {
		t.Fatalf("registry remains: %v", err)
	}
	if err := stopRegisteredStatusWatchers(other); err != nil {
		t.Fatal(err)
	}
	waitWatcherExit(t, unrelated)
}

func TestStatusWatcherStaleAndReplacedLeases(t *testing.T) {
	c := watcherCollection(t)
	target := c.Collection
	_, unrelated := watcherFixture(t, target, target, "idle", "wtc", "status", "--tui")
	// Even PID-shaped stale records do not confer ownership of a process.
	fixtureFile(t, filepath.Join(target, statusWatcherDir, "watcher-stale"), "12345\n", 0600)
	if err := stopRegisteredStatusWatchers(target); err != nil {
		t.Fatal(err)
	}
	select {
	case <-unrelated:
		t.Fatal("stale lease signaled an unrelated process")
	default:
	}
	stopped := make(chan struct{})
	cleanup, err := c.RegisterStatusWatcher(func() { close(stopped) })
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	entries, _ := os.ReadDir(filepath.Join(target, statusWatcherDir))
	path := filepath.Join(target, statusWatcherDir, entries[0].Name())
	if err := os.Rename(path, path+"-old"); err != nil {
		t.Fatal(err)
	}
	fixtureFile(t, path, "replacement", 0600)
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("watcher accepted a replaced lease identity")
	}
	cleanup()
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "replacement" {
		t.Fatalf("cleanup removed a replacement identity: %q, %v", data, err)
	}
}

func TestRetiredStatusWatcherCommandScope(t *testing.T) {
	target := t.TempDir()
	child := filepath.Join(target, "repo")
	if err := os.Mkdir(child, 0700); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		cwd, command string
		want         bool
	}{
		{target, "wtc status --tui", false},
		{child, "./taba status --watch=30", false},
		{target, "wtc status", false}, // one-shot commands are not leases
		{target, "bash harness/tools/wtc-status-tui.sh", true},
		{child, "sh ../harness/tools/wtc-status.sh --tui", true},
		{"", "bash " + filepath.Join(target, "harness/tools/wtc-status-tui.sh"), true},
		{target, "sh harness/tools/wtc-status.sh", false},
		{target, "wtc status other --tui", false},
		{target, "wtc status --all --tui", false},
		{target, "wtc status --no-watch", false},
		{target, "wtc status --json", false},
		{target + "-other", "wtc status --tui", false},
		{target, "sh -c 'wtc status --tui'", false},
		{target, "echo " + filepath.Join(target, "harness/tools/wtc-status-tui.sh"), false},
		{target, "wtc other status --tui", false},
	} {
		if got := retiredStatusWatcherCommand(target, tc.cwd, tc.command); got != tc.want {
			t.Errorf("cwd=%q command=%q: got %v, want %v", tc.cwd, tc.command, got, tc.want)
		}
	}
}

func TestRetiredWatcherRevalidatesProcessIdentity(t *testing.T) {
	target := t.TempDir()
	original := statusWatcherProcess{start: "first", cwd: target, args: "bash harness/tools/wtc-status-tui.sh"}
	for _, changed := range []statusWatcherProcess{
		{start: "reused", cwd: target, args: original.args},
		{start: original.start, cwd: t.TempDir(), args: original.args},
		{start: original.start, cwd: target, args: "wtc status other --tui"},
		{},
		original,
	} {
		reads, killed := 0, false
		stopRetiredStatusWatcher(target, 12345, func(int) (statusWatcherProcess, bool) {
			reads++
			if reads == 1 {
				return original, true
			}
			return changed, changed.start != ""
		}, func(int) { killed = true })
		if killed != (changed == original) {
			t.Errorf("identity %v: killed=%v", changed, killed)
		}
	}
}

func TestRetireLegacyWatcherProcesses(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("process inspection is supported on macOS and Linux")
	}
	target, other := t.TempDir(), t.TempDir()
	var pids []string
	var stopped []<-chan error
	for _, args := range [][]string{
		{"bash", "harness/tools/wtc-status-tui.sh"},
		{"sh", "harness/tools/wtc-status.sh", "--tui"},
		{"bash", filepath.Join(target, "harness/tools/wtc-status-tui.sh")},
	} {
		cmd, exited := watcherFixture(t, target, target, "idle", args...)
		identity, ok := readStatusWatcherProcess(cmd.Process.Pid)
		if !ok || !retiredStatusWatcherCommand(target, identity.cwd, identity.args) {
			t.Fatalf("fixture %v not identified: %+v (read=%v)", args, identity, ok)
		}
		pids = append(pids, fmt.Sprintf("%d %s", cmd.Process.Pid, strings.Join(args, " ")))
		stopped = append(stopped, exited)
	}
	var untouched []<-chan error
	for _, tc := range []struct {
		cwd  string
		args []string
	}{
		{other, []string{"wtc", "status", "--tui"}},
		{target, []string{"wtc", "status"}},
		{target, []string{"./taba", "status", "--tui"}},
		{target, []string{"wtc", "status", "other", "--tui"}},
		{target, []string{"echo", filepath.Join(target, "harness/tools/wtc-status-tui.sh")}},
		{target, []string{"sh", "harness/tools/wtc-status.sh"}},
	} {
		cmd, exited := watcherFixture(t, target, tc.cwd, "idle", tc.args...)
		pids = append(pids, fmt.Sprintf("%d %s", cmd.Process.Pid, strings.Join(tc.args, " ")))
		untouched = append(untouched, exited)
	}
	// The process inventory contains only our fixture PIDs, never host watchers.
	bin := t.TempDir()
	fixtureFile(t, filepath.Join(bin, "ps"), "#!/bin/sh\ncat <<'PIDS'\n"+strings.Join(pids, "\n")+"\nPIDS\n", 0700)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := stopRetiredStatusWatchers(target); err != nil {
		t.Fatal(err)
	}
	for _, exited := range stopped {
		waitWatcherExit(t, exited)
	}
	for _, exited := range untouched {
		select {
		case <-exited:
			t.Fatal("unrelated process was killed")
		default:
		}
	}
}

func TestRetireStopsWatchersBeforeRemovingTheirWorkingDirectories(t *testing.T) {
	c := newWorkspaceFixture(t)
	created, err := c.NewCollection(NewOptions{Slug: "watched"})
	if err != nil {
		t.Fatal(err)
	}
	target := created.Collection
	_, native := watcherFixture(t, target, c.Collection, "registered", "taba", "status", "watched", "--tui")
	legacyCmd, legacy := watcherFixture(t, target, filepath.Join(target, "harness"), "idle", "bash", "tools/wtc-status-tui.sh")
	bin := t.TempDir()
	fixtureFile(t, filepath.Join(bin, "ps"), fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' '%d bash tools/wtc-status-tui.sh'\n", legacyCmd.Process.Pid), 0700)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	retired, err := c.RetireCollection(RetireOptions{Name: "watched"})
	if err != nil {
		t.Fatal(err)
	}
	if !retired.FolderRemoved {
		t.Fatalf("watcher files kept collection alive: %+v", retired)
	}
	waitWatcherExit(t, native)
	waitWatcherExit(t, legacy)
}

func TestStatusWatcherRejectsRetiredAndSymlinkedRegistries(t *testing.T) {
	for _, retired := range []bool{false, true} {
		c := watcherCollection(t)
		target := c.Collection
		if retired {
			fixtureFile(t, filepath.Join(target, RetiredMarkerFile), "{}", 0600)
		} else if err := os.Symlink(t.TempDir(), filepath.Join(target, statusWatcherDir)); err != nil {
			t.Fatal(err)
		}
		if cleanup, err := c.RegisterStatusWatcher(func() {}); err == nil {
			cleanup()
			t.Fatal("watcher accepted an invalid collection registry")
		}
	}
}

func TestStatusWatcherRegistrationCannotReviveRetirement(t *testing.T) {
	c := watcherCollection(t)
	registered := make(chan error, 1)
	err := c.withRuntimeLock(func() error {
		go func() {
			cleanup, err := c.RegisterStatusWatcher(func() {})
			registered <- err
			if err == nil {
				cleanup()
			}
		}()
		select {
		case err := <-registered:
			t.Fatalf("registration bypassed retirement lock: %v", err)
		case <-time.After(100 * time.Millisecond):
		}
		// Model the final removal while retirement still owns its lock. The
		// waiting caller already discovered the collection before it vanished.
		return os.RemoveAll(c.Collection)
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-registered:
		if err == nil {
			t.Fatal("registration revived a removed collection")
		}
	case <-time.After(time.Second):
		t.Fatal("registration did not finish after retirement")
	}
	if _, err := os.Stat(c.Collection); !os.IsNotExist(err) {
		t.Fatalf("registration recreated collection files: %v", err)
	}
}

func TestStatusWatcherConcurrentRegistrationAndCleanup(t *testing.T) {
	c := watcherCollection(t)
	for range 20 {
		cleanup, err := c.RegisterStatusWatcher(func() {})
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan struct{})
		go func() { cleanup(); close(done) }()
		next, err := c.RegisterStatusWatcher(func() {})
		if err != nil {
			t.Fatal(err)
		}
		<-done
		next()
	}
}
