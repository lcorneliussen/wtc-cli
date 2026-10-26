package branding

import "testing"

func TestInvocationNames(t *testing.T) {
	for argv0, want := range map[string]string{
		"wtc": "wtc", "/opt/bin/wtc": "wtc", "wtc.exe": "wtc",
		"taba": "taba", "/opt/bin/taba": "taba", "taba.exe": "taba",
		"wtc.test": "wtc", "taba-wrapper": "wtc", "": "wtc",
	} {
		if got := name(argv0); got != want {
			t.Errorf("name(%q) = %q, want %q", argv0, got, want)
		}
	}
}
