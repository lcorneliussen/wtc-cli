package wtc

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed defaults/*.md defaults/skills/*/SKILL.md defaults/instructions/*.md
var defaults embed.FS

func DefaultPaths() ([]string, error) {
	var paths []string
	err := fs.WalkDir(defaults, "defaults", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			paths = append(paths, strings.TrimPrefix(path, "defaults/"))
		}
		return nil
	})
	return paths, err
}
func Eject(harness string, patterns []string) ([]string, error) {
	paths, err := DefaultPaths()
	if err != nil {
		return nil, err
	}
	var selected []string
	for _, pattern := range patterns {
		matched := false
		for _, path := range paths {
			ok, err := filepath.Match(pattern, path)
			if err != nil {
				return nil, err
			}
			if !ok {
				ok, err = filepath.Match(pattern, filepath.Dir(path))
				if err != nil {
					return nil, err
				}
			}
			if !ok {
				ok, err = filepath.Match(pattern, filepath.Base(path))
				if err != nil {
					return nil, err
				}
			}
			if ok {
				selected = append(selected, path)
				matched = true
			}
		}
		if !matched {
			return nil, fmt.Errorf("no embedded default matches %q", pattern)
		}
	}
	seen := map[string]bool{}
	var written []string
	for _, path := range selected {
		if seen[path] {
			continue
		}
		seen[path] = true
		dst := filepath.Join(harness, path)
		if _, err := os.Lstat(dst); err == nil {
			return written, fmt.Errorf("refusing to overwrite %s", dst)
		} else if !os.IsNotExist(err) {
			return written, err
		}
		data, err := defaults.ReadFile("defaults/" + path)
		if err != nil {
			return written, err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return written, err
		}
		if err := os.WriteFile(dst, data, 0644); err != nil {
			return written, err
		}
		written = append(written, dst)
	}
	return written, nil
}
