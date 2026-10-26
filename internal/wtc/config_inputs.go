package wtc

import (
	"os"
	"path/filepath"
)

// Legacy inputs stay authoritative during phase 0: older clients still write
// them. Only an absent legacy path permits the new spelling; invalid paths
// must not silently select a different configuration. No path is created here.
func legacyInput(dir, legacy, current string) (string, error) {
	for _, name := range []string{legacy, current} {
		path := filepath.Join(dir, name)
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return path, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if _, err := os.Stat(path); err != nil {
				return path, err
			}
		}
		return path, nil
	}
	return filepath.Join(dir, legacy), nil
}

func configInput(harness string) (string, error) {
	return legacyInput(harness, "wtc.toml", "taba.toml")
}

func pinInput(harness string) (string, error) {
	return legacyInput(harness, ".wtc-cli-version", ".taba-version")
}

func skillBaseInput(dir string) (string, error) {
	return legacyInput(dir, ".wtc-base.sha256", ".taba-base.sha256")
}

// MachineEnvFile selects the workspace defaults file without reading its
// values. It retains wtc.env as the default for fresh control roots.
func MachineEnvFile(root string) (string, error) {
	return legacyInput(root, "wtc.env", "taba.env")
}
