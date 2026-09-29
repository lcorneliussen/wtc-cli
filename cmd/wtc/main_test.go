package main

import "testing"

func TestCompatibility(t *testing.T) {
	cases := []struct {
		requirement, version string
		want                 bool
	}{
		{">=0.4,<0.5", "0.4.2", true},
		{">=0.4,<0.5", "0.5.0", false},
		{">=0.4,<0.5", "dev", false},
		{"invalid", "0.4.2", false},
	}
	for _, c := range cases {
		if got := compatible(c.requirement, c.version); got != c.want {
			t.Errorf("compatible(%q,%q)=%v", c.requirement, c.version, got)
		}
	}
}
