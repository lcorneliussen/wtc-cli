package wtc

import (
	"embed"
	"fmt"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
)

//go:embed release-notes/*.md
var releaseNotes embed.FS

// ReleaseNote is a committed release document embedded in this binary.
type ReleaseNote struct {
	Version string `json:"version"`
	Content string `json:"content"`
}

func releaseVersion(value string) (*semver.Version, error) {
	v, err := semver.StrictNewVersion(strings.TrimPrefix(value, "v"))
	if err != nil {
		return nil, fmt.Errorf("invalid release version %q: use a full semantic version such as 0.1.38", value)
	}
	return v, nil
}

// EmbeddedReleaseNotes returns releases in semantic version order, followed by
// unreleased changes. It neither discovers a collection nor contacts a forge.
func EmbeddedReleaseNotes() ([]ReleaseNote, error) {
	entries, err := releaseNotes.ReadDir("release-notes")
	if err != nil {
		return nil, err
	}
	notes := make([]ReleaseNote, 0, len(entries))
	var pending *ReleaseNote
	for _, entry := range entries {
		name := strings.TrimSuffix(entry.Name(), ".md")
		if name != "unreleased" {
			if _, err := releaseVersion(name); err != nil {
				return nil, err
			}
		}
		content, err := releaseNotes.ReadFile("release-notes/" + entry.Name())
		if err != nil {
			return nil, err
		}
		note := ReleaseNote{Version: name, Content: string(content)}
		if name == "unreleased" {
			pending = &note
		} else {
			notes = append(notes, note)
		}
	}
	sort.Slice(notes, func(i, j int) bool {
		a, _ := releaseVersion(notes[i].Version)
		b, _ := releaseVersion(notes[j].Version)
		return a.LessThan(b)
	})
	if pending != nil {
		notes = append(notes, *pending)
	}
	return notes, nil
}

// SelectReleaseNotes selects one version (latest by default), or all published
// notes newer than since. Unreleased changes require an explicit selection.
func SelectReleaseNotes(version, since string) ([]ReleaseNote, error) {
	if version != "" && since != "" {
		return nil, fmt.Errorf("choose a version or --since, not both")
	}
	var baseline *semver.Version
	if since != "" {
		var err error
		baseline, err = releaseVersion(since)
		if err != nil {
			return nil, err
		}
	}
	if version != "" && version != "unreleased" {
		v, err := releaseVersion(version)
		if err != nil {
			return nil, err
		}
		version = v.String()
	}
	notes, err := EmbeddedReleaseNotes()
	if err != nil {
		return nil, err
	}
	selected := make([]ReleaseNote, 0)
	for _, note := range notes {
		if version != "" {
			if note.Version == version {
				return []ReleaseNote{note}, nil
			}
			continue
		}
		if note.Version == "unreleased" {
			continue
		}
		if baseline != nil {
			v, _ := releaseVersion(note.Version)
			if v.GreaterThan(baseline) {
				selected = append(selected, note)
			}
		} else {
			selected = []ReleaseNote{note}
		}
	}
	if version != "" {
		return nil, fmt.Errorf("release notes for %q are not embedded in this binary; run wtc release-notes --list", version)
	}
	return selected, nil
}
