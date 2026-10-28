package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lcorneliussen/wtc-cli/internal/wtc"
	"github.com/spf13/cobra"
)

func hoursTestLocal(t *testing.T) {
	t.Helper()
	t.Setenv("OPENCLAW_STATE_DIR", "")
	t.Setenv("OPENCLAW_SHELL", "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
}
func TestWorkingHoursCLIRequiresExplicitSetupAndSharesProject(t *testing.T) {
	hoursTestLocal(t)
	c := &wtc.Context{Collection: t.TempDir(), ConfigRoot: t.TempDir()}
	invoke := func(args ...string) error {
		root := &cobra.Command{Use: "wtc", SilenceUsage: true, SilenceErrors: true}
		asJSON := true
		addWorkingHoursCommand(root, &asJSON, func() (*wtc.Context, error) { return c, nil })
		root.SetArgs(append([]string{"hours"}, args...))
		return root.Execute()
	}
	if err := invoke("setup"); err == nil {
		t.Fatal("setup guessed a schedule")
	}
	path := filepath.Join(t.TempDir(), "synthetic.json")
	if err := os.WriteFile(path, []byte(`{"timezone":"Etc/UTC","windows":[{"days":[1,3],"start":"10:00","end":"16:00"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := invoke("setup", "--file", path); err != nil {
		t.Fatal(err)
	}
	if err := invoke("extend"); err == nil {
		t.Fatal("extension guessed a duration")
	}
	if err := invoke("extend", "--for", "5h"); err == nil {
		t.Fatal("unbounded extension")
	}
	if err := invoke("extend", "--for", "30m"); err != nil {
		t.Fatal(err)
	}
	sibling := &wtc.Context{Collection: t.TempDir(), ConfigRoot: c.ConfigRoot}
	f, err := localHoursFacts(sibling, time.Now())
	if err != nil || f.Phase != "extended" {
		t.Fatal(f, err)
	}
	if err := invoke("end-extension"); err != nil {
		t.Fatal(err)
	}
	if err := invoke("edit", "--project", "--file", path); err != nil {
		t.Fatal(err)
	}
	if err := invoke("clear", "--project"); err != nil {
		t.Fatal(err)
	}
	if err := invoke("clear"); err != nil {
		t.Fatal(err)
	}
	f, err = localHoursFacts(c, time.Now())
	if err != nil || f.Configured {
		t.Fatal(f, err)
	}
}
func TestWorkingHoursOpenClawNeverUsesGatewayPerson(t *testing.T) {
	hoursTestLocal(t)
	c := &wtc.Context{Collection: t.TempDir(), ConfigRoot: t.TempDir()}
	store, err := localHoursStore(c)
	if err != nil {
		t.Fatal(err)
	}
	p := wtc.WorkingSchedule{Timezone: "Etc/UTC", Windows: []wtc.WorkingWindow{{Days: []int{1}, Start: "10:00", End: "16:00"}}}
	if err := store.Setup(p, false); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"OPENCLAW_STATE_DIR", "OPENCLAW_SHELL"} {
		t.Run(key, func(t *testing.T) {
			value := "synthetic"
			if key == "OPENCLAW_SHELL" {
				value = "exec"
			}
			t.Setenv(key, value)
			if _, err := localHoursStore(c); err == nil {
				t.Fatal("gateway identity accepted")
			}
			f, err := localHoursFacts(c, time.Now())
			if err != nil || f.Configured || f.Phase != "unconfigured" || f.Timezone != "" {
				t.Fatal("leaked gateway schedule", f, err)
			}
		})
	}
}
func TestWorkingHoursTUIRefreshesPhaseWithoutForgeRefresh(t *testing.T) {
	hoursTestLocal(t)
	c := &wtc.Context{Collection: t.TempDir(), ConfigRoot: t.TempDir()}
	m := statusTUIModel{context: c, width: 120}
	if got := strings.Join(m.contentLines(), "\n"); !strings.Contains(got, "Set working hours") {
		t.Fatal(got)
	}
	store, err := localHoursStore(c)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Setup(wtc.WorkingSchedule{Timezone: "Etc/UTC", Windows: []wtc.WorkingWindow{{Days: []int{1}, Start: "10:00", End: "16:00"}}}, false); err != nil {
		t.Fatal(err)
	}
	if err := store.Extend(time.Now(), time.Hour); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(m.contentLines(), "\n"); !strings.Contains(got, "extended") {
		t.Fatal(got)
	}
	if err := store.Clear(false); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(m.contentLines(), "\n"); !strings.Contains(got, "Set working hours") {
		t.Fatal(got)
	}
}
