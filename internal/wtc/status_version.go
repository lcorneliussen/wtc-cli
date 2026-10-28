package wtc

import (
	"strings"

	"github.com/Masterminds/semver/v3"
)

// RunningCLIVersion observes this process's build version without consulting
// collection pins, installed binaries or cached status producers.
func RunningCLIVersion() string {
	return StatusVersionLabel(CLIVersion)
}

// StatusVersionLabel keeps missing versions honest and build labels terminal-safe.
func StatusVersionLabel(version string) string {
	version = strings.TrimSpace(runtimeTextSafe(version))
	if version == "" {
		return "unknown"
	}
	return version
}

// StatusVersionNotesURL links stable build versions to their release notes.
// Development, prerelease and custom builds have no known published release.
func StatusVersionNotesURL(version string) string {
	v, err := semver.StrictNewVersion(strings.TrimPrefix(version, "v"))
	if err != nil || v.Prerelease() != "" || v.Metadata() != "" {
		return ""
	}
	return "https://github.com/" + cliReleaseRepo + "/releases/tag/v" + v.String()
}
