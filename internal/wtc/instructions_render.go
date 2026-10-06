package wtc

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Default skills resolve ../../instructions inside .wtc. Render that tree from
// the pinned CLI while linking repo-owned overrides to their original source.
func (c *Context) renderInstructions(dryRun bool, r *SkillRenderResult) error {
	paths, err := DefaultPaths()
	if err != nil {
		return err
	}
	for _, path := range paths {
		if !strings.HasPrefix(path, "instructions/") {
			continue
		}
		dest := filepath.Join(c.Collection, ".wtc", filepath.FromSlash(path))
		override := filepath.Join(c.Harness, filepath.FromSlash(path))
		link := "../../" + filepath.Base(c.Harness) + "/" + path
		if err := safeGeneratedParent(c.Collection, filepath.Dir(dest)); err != nil {
			return err
		}
		if info, err := os.Stat(override); err == nil && info.Mode().IsRegular() {
			if target, err := os.Readlink(dest); err == nil && target != link {
				return fmt.Errorf("unmanaged instruction symlink: %s", dest)
			}
			// A previously generated default becomes a link when a project takes ownership.
			if info, err := os.Lstat(dest); err == nil && info.Mode().IsRegular() {
				r.Actions = append(r.Actions, "replace default instruction: "+path)
				if dryRun {
					// The default is still on disk, so the link step would misread it as an override.
					r.Linked++
					r.Actions = append(r.Actions, "would link: "+path+" -> "+link)
					continue
				}
				if err := os.Remove(dest); err != nil {
					return err
				}
			}
			if err := renderSkillLink(c.Collection, dest, link, path, dryRun, r); err != nil {
				return err
			}
			continue
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
		if target, err := os.Readlink(dest); err == nil {
			if target != link {
				return fmt.Errorf("unmanaged instruction symlink: %s", dest)
			}
			r.Actions = append(r.Actions, "restore default instruction: "+path)
			if !dryRun {
				if err := os.Remove(dest); err != nil {
					return err
				}
			}
		}
		data, err := ReadDefault(path)
		if err != nil {
			return err
		}
		old, err := os.ReadFile(dest)
		if err == nil && bytes.Equal(old, data) {
			continue
		}
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := safeGeneratedParent(c.Collection, filepath.Dir(dest)); err != nil {
			return err
		}
		if info, err := os.Lstat(dest); err == nil && !info.Mode().IsRegular() {
			if dryRun && info.Mode()&os.ModeSymlink != 0 {
				continue
			}
			return fmt.Errorf("generated instruction target must be regular: %s", dest)
		}
		r.Actions = append(r.Actions, "render default instruction: "+path)
		if !dryRun {
			if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
				return err
			}
			if err := os.WriteFile(dest, data, 0644); err != nil {
				return err
			}
		}
	}
	return nil
}
