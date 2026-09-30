package wtc

import (
	"strings"
	"testing"
	"time"
)

func TestStatusCommandTimesOut(t *testing.T) {
	started := time.Now()
	_, err := statusCommand(20*time.Millisecond, "sh", "-c", "sleep 2")
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("hung command did not time out: %v", err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("timeout did not release the caller promptly")
	}
}
