package wtc

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// The unreleased directory always holds README.md, so the pattern matches
// even when no change is pending. Embedding the directory takes every file
// name except dot and underscore files, so a fragment without the .md suffix
// fails reads the same way it fails cut and check, and dot or underscore
// drafts and OS files never reach the binary. A subdirectory would be
// embedded too, so reads reject one.
//
//go:embed release-notes/*.md release-notes/unreleased
var releaseNotes embed.FS

// UnreleasedDir holds one fragment per pending change, relative to the
// release-notes directory.
const UnreleasedDir = "unreleased"

const unreleasedReadme = "README.md"

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
	root, err := fs.Sub(releaseNotes, "release-notes")
	if err != nil {
		return nil, err
	}
	entries, err := fs.ReadDir(root, ".")
	if err != nil {
		return nil, err
	}
	notes := make([]ReleaseNote, 0, len(entries)+1)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".md")
		if _, err := releaseVersion(name); err != nil {
			return nil, err
		}
		content, err := fs.ReadFile(root, entry.Name())
		if err != nil {
			return nil, err
		}
		notes = append(notes, ReleaseNote{Version: name, Content: string(content)})
	}
	sort.Slice(notes, func(i, j int) bool {
		a, _ := releaseVersion(notes[i].Version)
		b, _ := releaseVersion(notes[j].Version)
		return a.LessThan(b)
	})
	fragments, err := ReadReleaseFragments(root)
	if err != nil {
		return nil, err
	}
	pending := "# wtc unreleased\n\nNo unreleased changes.\n"
	if len(fragments) > 0 {
		pending = AssembleReleaseNotes("unreleased", fragments)
	}
	return append(notes, ReleaseNote{Version: "unreleased", Content: pending}), nil
}

// ReleaseFragment is one pending change's notes: level-two topic headings,
// each followed by its entries.
type ReleaseFragment struct {
	Name     string
	Sections []ReleaseSection
}

// ReleaseSection is one topic heading and the Markdown body below it.
type ReleaseSection struct {
	Heading string
	Body    string
}

// ReadReleaseFragments reads and validates the unreleased fragments of a
// release-notes directory, ordered by issue number, then file name.
func ReadReleaseFragments(root fs.FS) ([]ReleaseFragment, error) {
	entries, err := fs.ReadDir(root, UnreleasedDir)
	if err != nil {
		return nil, err
	}
	var fragments []ReleaseFragment
	for _, entry := range entries {
		name := entry.Name()
		// Dot and underscore files (.DS_Store, editor swap files, drafts) are
		// not embedded; skipping them on disk keeps cut and check in step.
		if name == unreleasedReadme || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
			continue
		}
		path := UnreleasedDir + "/" + name
		if entry.IsDir() {
			// Its files would be embedded but never assembled, cut or checked.
			return nil, fmt.Errorf("%s: fragments live directly in %s/, not in subdirectories", path, UnreleasedDir)
		}
		if !strings.HasSuffix(name, ".md") {
			return nil, fmt.Errorf("%s: release note fragments are Markdown files", path)
		}
		content, err := fs.ReadFile(root, path)
		if err != nil {
			return nil, err
		}
		fragment, err := ParseReleaseFragment(name, string(content))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		fragments = append(fragments, fragment)
	}
	sort.SliceStable(fragments, func(i, j int) bool {
		a, b := fragmentNumber(fragments[i].Name), fragmentNumber(fragments[j].Name)
		if a != b {
			return a < b
		}
		return fragments[i].Name < fragments[j].Name
	})
	return fragments, nil
}

// fragmentNumber orders "<issue>-<slug>.md" fragments by issue number, so #117
// follows #46. Names without a leading number sort after numbered ones.
func fragmentNumber(name string) int {
	n, digits := 0, 0
	for _, r := range name {
		if r < '0' || r > '9' {
			break
		}
		n, digits = n*10+int(r-'0'), digits+1
	}
	if digits == 0 {
		return int(^uint(0) >> 1)
	}
	return n
}

// ParseReleaseFragment splits a fragment into its topic sections. A fragment
// starts with a "## " heading and every heading has at least one entry.
func ParseReleaseFragment(name, content string) (ReleaseFragment, error) {
	fragment := ReleaseFragment{Name: name}
	var current *ReleaseSection
	var body []string
	finish := func() error {
		if current == nil {
			return nil
		}
		current.Body = strings.Trim(strings.Join(body, "\n"), "\n")
		if strings.TrimSpace(current.Body) == "" {
			return fmt.Errorf("section %q has no entries", current.Heading)
		}
		fragment.Sections = append(fragment.Sections, *current)
		return nil
	}
	fence := ""
	for _, line := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		if fence != "" || codeFence(line) != "" {
			// Headings inside a fenced code block are example text.
			if fence == "" {
				fence = codeFence(line)
			} else if closesFence(line, fence) {
				fence = ""
			}
			if current == nil {
				return ReleaseFragment{}, fmt.Errorf("start with a \"## <topic>\" heading")
			}
			body = append(body, line)
			continue
		}
		if strings.HasPrefix(line, "# ") {
			return ReleaseFragment{}, fmt.Errorf("use \"## <topic>\" headings; the release heading is added when notes are assembled")
		}
		if strings.HasPrefix(line, "## ") {
			if err := finish(); err != nil {
				return ReleaseFragment{}, err
			}
			heading := strings.TrimSpace(strings.TrimPrefix(line, "## "))
			if heading == "" {
				return ReleaseFragment{}, fmt.Errorf("empty section heading")
			}
			current, body = &ReleaseSection{Heading: heading}, nil
			continue
		}
		if current == nil {
			if strings.TrimSpace(line) != "" {
				return ReleaseFragment{}, fmt.Errorf("start with a \"## <topic>\" heading")
			}
			continue
		}
		body = append(body, line)
	}
	if fence != "" {
		return ReleaseFragment{}, fmt.Errorf("unclosed %s code fence", fence)
	}
	if err := finish(); err != nil {
		return ReleaseFragment{}, err
	}
	if len(fragment.Sections) == 0 {
		return ReleaseFragment{}, fmt.Errorf("no \"## <topic>\" section")
	}
	return fragment, nil
}

// sectionJoin separates two bodies merged under one heading. A list that
// follows a list (any marker) stays tight; anything else gets a blank line, so
// prose is not read as a continuation of the previous list item.
func sectionJoin(previous, next string) string {
	lines := strings.Split(previous, "\n")
	last := lines[len(lines)-1]
	if listMarker.MatchString(next) && (listMarker.MatchString(last) || strings.HasPrefix(last, " ")) {
		return "\n"
	}
	return "\n\n"
}

// codeFence returns the opening delimiter of a fenced code block (three or
// more backticks or tildes), or "". The fence may follow indentation or a
// list marker, as in "- ~~~sh".
func codeFence(line string) string {
	trimmed := listMarker.ReplaceAllString(strings.TrimLeft(line, " "), "")
	if len(trimmed) < 3 {
		return ""
	}
	marker := trimmed[0]
	if marker != '`' && marker != '~' {
		return ""
	}
	n := 0
	for n < len(trimmed) && trimmed[n] == marker {
		n++
	}
	if n < 3 || (marker == '`' && strings.ContainsRune(trimmed[n:], '`')) {
		return ""
	}
	return trimmed[:n]
}

var listMarker = regexp.MustCompile(`^(?:(?:[-*+]|[0-9]+[.)])[ ]+)+`)

// closesFence reports whether line ends the block opened by fence: the same
// character, at least as many times, and nothing else. A fence inside a list
// item closes at the item's indentation, so any indentation is accepted.
func closesFence(line, fence string) bool {
	trimmed := strings.Trim(line, " \t")
	return len(trimmed) >= len(fence) && strings.Trim(trimmed, fence[:1]) == ""
}

// AssembleReleaseNotes renders fragments as one release document headed
// "# wtc <version>". Sections with the same heading are merged; headings keep
// the order in which fragments, in ReadReleaseFragments order, first use them.
func AssembleReleaseNotes(version string, fragments []ReleaseFragment) string {
	var order []string
	bodies := map[string][]string{}
	for _, fragment := range fragments {
		for _, section := range fragment.Sections {
			if _, seen := bodies[section.Heading]; !seen {
				order = append(order, section.Heading)
			}
			bodies[section.Heading] = append(bodies[section.Heading], section.Body)
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# wtc %s\n", version)
	for _, heading := range order {
		fmt.Fprintf(&b, "\n## %s\n\n", heading)
		for i, body := range bodies[heading] {
			if i > 0 {
				b.WriteString(sectionJoin(bodies[heading][i-1], body))
			}
			b.WriteString(body)
		}
		b.WriteString("\n")
	}
	return b.String()
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

// CutReleaseNotes writes <version>.md in dir from the unreleased fragments and
// deletes them in one working-tree change, for the caller to commit. It
// refuses an existing release file and an empty fragment set.
func CutReleaseNotes(dir, version string) (string, error) {
	v, err := releaseVersion(version)
	if err != nil {
		return "", err
	}
	fragments, err := ReadReleaseFragments(os.DirFS(dir))
	if err != nil {
		return "", err
	}
	if len(fragments) == 0 {
		return "", fmt.Errorf("no unreleased fragments in %s", filepath.Join(dir, UnreleasedDir))
	}
	path := filepath.Join(dir, v.String()+".md")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", err
	}
	_, err = file.WriteString(AssembleReleaseNotes(v.String(), fragments))
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	for _, fragment := range fragments {
		if err := os.Remove(filepath.Join(dir, UnreleasedDir, fragment.Name)); err != nil {
			return "", err
		}
	}
	return path, nil
}
