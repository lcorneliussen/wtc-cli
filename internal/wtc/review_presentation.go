package wtc

import (
	"html"
	"net/url"
	"regexp"
	"strings"
)

var reviewSourceLocation = regexp.MustCompile("`([^`\\n]+):([1-9][0-9]*)`")

// Presentation changes only the posted copy; the reviewer output and gate
// record remain available verbatim in the bundle.
func presentReviewSummary(body string, m ReviewManifest, files map[string]bool) string {
	if m.Forge != "github" {
		return body
	}
	mark := ""
	if match := reviewStatusLine.FindStringSubmatch(body); len(match) > 2 {
		switch match[2] {
		case "pass":
			mark = "🟢 "
		case "pass-with-notes":
			mark = "🟡 "
		case "changes-requested", "error":
			mark = "🔴 "
		}
	}
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	var out []string
	fence, folded := "", false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		fenceRun := ""
		if strings.HasPrefix(trimmed, "`") || strings.HasPrefix(trimmed, "~") {
			marker := trimmed[0]
			for i := 0; i < len(trimmed) && trimmed[i] == marker; i++ {
				fenceRun += string(marker)
			}
		}
		if fence != "" {
			out = append(out, line)
			if strings.HasPrefix(fenceRun, fence) && strings.TrimSpace(strings.TrimPrefix(trimmed, fenceRun)) == "" {
				fence = ""
			}
			continue
		}
		if len(fenceRun) >= 3 {
			fence = fenceRun
			out = append(out, line)
			continue
		}
		if strings.HasPrefix(line, "### ") || strings.Contains(line, "wtc-review v1 head=") {
			if folded {
				out = append(out, "", "</details>", "")
				folded = false
			}
		}
		title := strings.TrimPrefix(line, "### ")
		if strings.HasPrefix(line, "### ") {
			switch title {
			case "Concerns", "Addressed since previous round", "Run stats":
				out = append(out, "<details>", "<summary>"+html.EscapeString(title)+"</summary>", "")
				folded = true
				continue
			}
		}
		if strings.HasPrefix(line, "**Local review:") {
			line = mark + line
		}
		line = reviewSourceLocation.ReplaceAllStringFunc(line, func(location string) string {
			parts := reviewSourceLocation.FindStringSubmatch(location)
			if !files[parts[1]] {
				return location
			}
			var segments []string
			for _, part := range strings.Split(parts[1], "/") {
				segments = append(segments, url.PathEscape(part))
			}
			link := "https://github.com/" + m.Slug + "/blob/" + m.HeadSHA + "/" + strings.Join(segments, "/") + "#L" + parts[2]
			label := strings.NewReplacer("[", `\[`, "]", `\]`).Replace(location)
			return "[" + label + "](" + link + ")"
		})
		out = append(out, line)
	}
	if folded {
		out = append(out, "", "</details>")
	}
	return strings.Join(out, "\n") + "\n"
}

func reviewSourceFiles(m ReviewManifest) map[string]bool {
	files := map[string]bool{}
	if m.Forge == "github" {
		if raw, err := gitOutput("-C", m.RepoDir, "ls-tree", "-r", "-z", "--name-only", m.HeadSHA); err == nil {
			for _, path := range strings.Split(raw, "\x00") {
				files[path] = true
			}
		}
	}
	return files
}
