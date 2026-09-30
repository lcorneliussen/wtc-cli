package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lcorneliussen/wtc-cli/internal/wtc"
)

func TestStatusSettingsUseControlRootAndEnvironmentOverride(t *testing.T) {
	root := t.TempDir()
	config := "WTC_STATUS_WATCH='17'\nexport WTC_STATUS_WATCH_BG=123\nWTC_STATUS_NO_CLICK=yes\n"
	if err := os.WriteFile(filepath.Join(root, "wtc.env"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	c := &wtc.Context{ConfigRoot: root}
	t.Setenv("WTC_STATUS_WATCH", "")
	t.Setenv("WTC_STATUS_WATCH_BG", "")
	t.Setenv("WTC_STATUS_NO_CLICK", "")
	if got := statusIntervalSetting(c, "WTC_STATUS_WATCH", 30); got != 17 {
		t.Fatalf("control-root interval: %d", got)
	}
	if got := statusIntervalSetting(c, "WTC_STATUS_WATCH_BG", 300); got != 123 {
		t.Fatalf("background interval: %d", got)
	}
	if !statusTrue(statusSetting(c, "WTC_STATUS_NO_CLICK")) {
		t.Fatal("control-root no-click setting ignored")
	}
	t.Setenv("WTC_STATUS_WATCH", "9")
	if got := statusIntervalSetting(c, "WTC_STATUS_WATCH", 30); got != 9 {
		t.Fatalf("environment did not override file: %d", got)
	}
}
