package wtc

import (
	"os"
	"path/filepath"
	"sort"
)

// WorkspaceCollections returns direct child collections with a harness directory.
// Each caller opens the target independently so an invalid collection does not
// prevent the rest of a workspace sweep. Folders a retire left behind are not
// collections, whatever they still hold.
func WorkspaceCollections(workspace string) ([]string, error) {
	entries, err := os.ReadDir(workspace)
	if err != nil {
		return nil, err
	}
	var collections []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(workspace, entry.Name())
		info, err := os.Lstat(filepath.Join(dir, "harness"))
		if err == nil && info.IsDir() && !RetiredFolder(dir) {
			collections = append(collections, dir)
		}
	}
	sort.Strings(collections)
	return collections, nil
}

// RetiredFolders returns direct child folders that carry a retire marker.
func RetiredFolders(workspace string) ([]string, error) {
	entries, err := os.ReadDir(workspace)
	if err != nil {
		return nil, err
	}
	var folders []string
	for _, entry := range entries {
		dir := filepath.Join(workspace, entry.Name())
		if entry.IsDir() && RetiredFolder(dir) {
			folders = append(folders, dir)
		}
	}
	sort.Strings(folders)
	return folders, nil
}
