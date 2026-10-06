package wtc

import (
	"strings"
	"testing"
)

func TestEmbeddedReleaseNotes(t *testing.T) {
	notes, err := EmbeddedReleaseNotes()
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) < 40 || notes[0].Version != "0.1.0" || notes[len(notes)-1].Version != "unreleased" {
		t.Fatalf("missing historical or upcoming notes: %d documents", len(notes))
	}
	for i, note := range notes {
		header := "# wtc " + note.Version + "\n"
		if !strings.HasPrefix(note.Content, header) || strings.TrimSpace(strings.TrimPrefix(note.Content, header)) == "" {
			t.Fatalf("missing heading or body for %s", note.Version)
		}
		if note.Version == "unreleased" {
			continue
		}
		v, err := releaseVersion(note.Version)
		if err != nil || v.String() != note.Version {
			t.Fatalf("noncanonical archive version %q", note.Version)
		}
		if i > 0 {
			previous, _ := releaseVersion(notes[i-1].Version)
			if !v.GreaterThan(previous) {
				t.Fatalf("archive out of order: %s, %s", previous, v)
			}
		}
	}
}

func TestSelectReleaseNotes(t *testing.T) {
	all, err := EmbeddedReleaseNotes()
	if err != nil {
		t.Fatal(err)
	}
	latest := all[len(all)-2].Version
	tests := []struct {
		name, version, since string
		want                 []string
	}{
		{"latest", "", "", []string{latest}},
		{"exact with v", "v0.1.9", "", []string{"0.1.9"}},
		{"unreleased explicit", "unreleased", "", []string{"unreleased"}},
		{"empty range", "", latest, []string{}},
		{"future baseline", "", "9.0.0", []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notes, err := SelectReleaseNotes(tt.version, tt.since)
			if err != nil {
				t.Fatal(err)
			}
			if notes == nil || len(notes) != len(tt.want) {
				t.Fatalf("got %v, want %v", notes, tt.want)
			}
			for i, want := range tt.want {
				if notes[i].Version != want {
					t.Fatalf("got %s, want %s", notes[i].Version, want)
				}
			}
		})
	}

	notes, err := SelectReleaseNotes("", "v0.1.8")
	if err != nil || len(notes) < 30 || notes[0].Version != "0.1.9" || notes[1].Version != "0.1.10" || notes[len(notes)-1].Version != latest {
		t.Fatal("upgrade range must exclude baseline and unreleased, and sort numerically", err)
	}
	for _, args := range [][2]string{
		{"0.1.38", "0.1.35"}, {"9.0.0", ""}, {"../README", ""},
		{"0.1", ""}, {"", "unreleased"}, {"", "garbage"}, {"", "0.1"},
	} {
		if _, err := SelectReleaseNotes(args[0], args[1]); err == nil {
			t.Fatalf("expected error for %q", args)
		}
	}
}
