package wtc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// readPriorReviewManifest accepts native JSON and the small set of identifiers
// needed from older shell bundles. It never sources manifest.env as shell code.
func readPriorReviewManifest(dir string) (ReviewManifest, bool, bool) {
	var manifest ReviewManifest
	if data, err := os.ReadFile(filepath.Join(dir, "manifest.json")); err == nil {
		if json.Unmarshal(data, &manifest) == nil {
			return manifest, true, false
		}
		return ReviewManifest{}, false, false
	}
	data, err := os.ReadFile(filepath.Join(dir, "manifest.env"))
	if err != nil {
		return ReviewManifest{}, false, false
	}
	fields := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch key {
		case "REPO", "PR", "HEAD_BRANCH", "ROUND":
			// These are plain tokens in shell-generated bundles. Do not
			// interpret arbitrary shell escaping or execute the file.
			if strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") {
				value = strings.TrimSuffix(strings.TrimPrefix(value, "'"), "'")
			}
			fields[key] = value
		}
	}
	round, err := strconv.Atoi(fields["ROUND"])
	if err != nil || round < 1 || !prRepoName.MatchString(fields["REPO"]) || fields["PR"] != "" && !prNumber.MatchString(fields["PR"]) {
		return ReviewManifest{}, false, false
	}
	manifest = ReviewManifest{Repo: fields["REPO"], PR: fields["PR"], HeadBranch: fields["HEAD_BRANCH"], Round: round}
	return manifest, true, true
}
