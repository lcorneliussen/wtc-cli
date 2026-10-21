package wtc

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// RetiredMarkerFile marks a folder that retire emptied of worktrees but could
// not delete. Other commands treat it as leftovers, never as a collection.
const RetiredMarkerFile = ".wtc-retired"

// CLIVersion is the running CLI's version, recorded in retire markers.
var CLIVersion = "dev"

type RetiredMarker struct {
	SchemaVersion int      `json:"schema_version"`
	RetiredAt     string   `json:"retired_at"`
	CLIVersion    string   `json:"cli_version"`
	BlockingPaths []string `json:"blocking_paths"`
}

// RetiredFolder reports whether dir carries a retire marker.
func RetiredFolder(dir string) bool {
	info, err := os.Lstat(filepath.Join(dir, RetiredMarkerFile))
	return err == nil && info.Mode().IsRegular()
}

// ReadRetiredMarker reads the marker of a retired folder. A marker that
// cannot be parsed still marks the folder; its fields are then empty.
func ReadRetiredMarker(dir string) (RetiredMarker, error) {
	var marker RetiredMarker
	data, err := os.ReadFile(filepath.Join(dir, RetiredMarkerFile))
	if err != nil {
		return marker, err
	}
	if err := json.Unmarshal(data, &marker); err != nil {
		return RetiredMarker{}, fmt.Errorf("unreadable %s: %w", RetiredMarkerFile, err)
	}
	return marker, nil
}

func writeRetiredMarker(dir string, blocking []string) error {
	if blocking == nil {
		blocking = []string{}
	}
	data, err := json.MarshalIndent(RetiredMarker{SchemaVersion: 1, RetiredAt: time.Now().UTC().Format(time.RFC3339),
		CLIVersion: CLIVersion, BlockingPaths: blocking}, "", "  ")
	if err != nil {
		return err
	}
	return writeCatchUpFile(filepath.Join(dir, RetiredMarkerFile), append(data, '\n'))
}

func retiredFolderError(dir string) error {
	return fmt.Errorf("%s holds the leftovers of a retired collection (see %s); run `wtc retire %s` once they are gone",
		dir, RetiredMarkerFile, filepath.Base(dir))
}

// retireLeftovers lists what keeps a folder alive, except the marker itself.
func retireLeftovers(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	leftovers := []string{}
	for _, entry := range entries {
		if entry.Name() != RetiredMarkerFile {
			leftovers = append(leftovers, entry.Name())
		}
	}
	return leftovers, nil
}

// finishRetiredFolder completes a retire that stopped on unexpected files.
// It refuses a folder that holds a harness or worktree again, since only a
// full retire may remove those. Otherwise no hooks run: it removes the
// generated files a lingering process may have written since, and the folder
// once nothing else is left. If anything remains, the marker is refreshed.
func (c *Context) finishRetiredFolder(target string) (RetireResult, error) {
	result := RetireResult{Collection: target, Removed: []string{}}
	entries, err := os.ReadDir(target)
	if err != nil {
		return result, err
	}
	for _, entry := range entries {
		if _, err := os.Lstat(filepath.Join(target, entry.Name(), ".git")); entry.Name() == "harness" || err == nil {
			return result, fmt.Errorf("%s carries %s but holds %s again; inspect it before retiring", target, RetiredMarkerFile, entry.Name())
		}
	}
	stopRetiredStatusWatchers(target)
	if err := removeRetiredGeneratedFiles(target, false); err != nil {
		return result, err
	}
	leftovers, err := retireLeftovers(target)
	if err != nil {
		return result, err
	}
	if len(leftovers) == 0 {
		if err := os.Remove(filepath.Join(target, RetiredMarkerFile)); err != nil {
			return result, err
		}
		if err := os.Remove(target); err != nil {
			return result, fmt.Errorf("remove collection folder: %w", err)
		}
		result.FolderRemoved = true
		return result, nil
	}
	result.Leftovers = leftovers
	if err := writeRetiredMarker(target, leftovers); err != nil {
		result.Warnings = append(result.Warnings, "could not refresh "+RetiredMarkerFile+": "+err.Error())
	}
	return result, nil
}
