package envcompat

import (
	"strings"
	"sync"
	"testing"
)

func TestPrefixPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name   string
		values map[string]string
		want   string
		set    bool
		warn   bool
	}{
		{"unset", nil, "", false, false},
		{"legacy", map[string]string{"WTC_OPTION": "old"}, "old", true, false},
		{"new", map[string]string{"TABA_OPTION": "new"}, "new", true, false},
		{"same", map[string]string{"TABA_OPTION": "same", "WTC_OPTION": "same"}, "same", true, false},
		{"conflict", map[string]string{"TABA_OPTION": "new-secret", "WTC_OPTION": "old-secret"}, "new-secret", true, true},
		{"explicit empty", map[string]string{"TABA_OPTION": "", "WTC_OPTION": "old-secret"}, "", true, true},
		{"legacy empty", map[string]string{"WTC_OPTION": ""}, "", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var warnings []string
			r := resolver{warn: func(s string) { warnings = append(warnings, s) }}
			lookup := func(key string) (string, bool) { value, ok := tc.values[key]; return value, ok }
			for _, spelling := range []string{"WTC_OPTION", "TABA_OPTION", "WTC_OPTION"} {
				if got, set := r.lookup(spelling, lookup); got != tc.want || set != tc.set {
					t.Fatalf("%s: got (%q, %v), want (%q, %v)", spelling, got, set, tc.want, tc.set)
				}
			}
			wantWarnings := 0
			if tc.warn {
				wantWarnings = 1
			}
			if len(warnings) != wantWarnings {
				t.Fatalf("warnings = %v, want %d", warnings, wantWarnings)
			}
			for _, warning := range warnings {
				if strings.Contains(warning, "secret") || !strings.Contains(warning, "using TABA_OPTION") {
					t.Fatalf("warning leaks a value or omits the chosen name: %s", warning)
				}
			}
		})
	}
}

func TestConflictWarningIsConcurrentAndPerSetting(t *testing.T) {
	var warnings []string
	var mu sync.Mutex
	r := resolver{warn: func(s string) { mu.Lock(); defer mu.Unlock(); warnings = append(warnings, s) }}
	lookup := func(key string) (string, bool) { return key, true }
	var group sync.WaitGroup
	for i := 0; i < 32; i++ {
		group.Add(1)
		go func() { defer group.Done(); r.lookup("WTC_ONE", lookup); r.lookup("TABA_TWO", lookup) }()
	}
	group.Wait()
	if len(warnings) != 2 {
		t.Fatalf("got %d warnings, want one per setting", len(warnings))
	}
}

func TestUnbrandedAndInternalNamesStayLiteral(t *testing.T) {
	r := resolver{warn: func(s string) { t.Fatalf("unexpected warning: %s", s) }}
	for _, name := range []string{"PATH", "HARNESS_STATUS_WATCH", "WTC_TEST_DEKIT", "WTC_DEV_BUILD", "TABA_TEST_DEKIT", "TABA_DEV_BUILD"} {
		var keys []string
		value, set := r.lookup(name, func(key string) (string, bool) { keys = append(keys, key); return key, true })
		if value != name || !set || len(keys) != 1 || keys[0] != name {
			t.Fatalf("%s was aliased: value=%q, keys=%v", name, value, keys)
		}
	}
}

func TestLiteralAssignments(t *testing.T) {
	values := Assignments([]byte("# settings\nTABA_OPTION='new'\nexport WTC_OPTION=old\nTABA_OPTION=\"last\"\nCOMMAND='$(no-evaluation)'\nEMPTY=\n"))
	r := resolver{warn: func(string) {}}
	lookup := func(key string) (string, bool) { value, ok := values[key]; return value, ok }
	if value, _ := r.lookup("WTC_OPTION", lookup); value != "last" {
		t.Fatalf("prefix precedence or repeated assignments lost: %q", value)
	}
	if values["COMMAND"] != "$(no-evaluation)" {
		t.Fatalf("shell expression changed: %q", values["COMMAND"])
	}
	if value, ok := values["EMPTY"]; !ok || value != "" {
		t.Fatal("explicit empty assignment lost")
	}
}

func TestProcessEnvironment(t *testing.T) {
	t.Setenv("WTC_ENVCOMPAT_OPTION", "legacy")
	t.Setenv("TABA_ENVCOMPAT_OPTION", "current")
	if value := Getenv("WTC_ENVCOMPAT_OPTION"); value != "current" {
		t.Fatalf("process environment precedence: %q", value)
	}
	if value, set := LookupEnv("TABA_ENVCOMPAT_OPTION"); value != "current" || !set {
		t.Fatalf("process lookup: (%q, %v)", value, set)
	}
}
