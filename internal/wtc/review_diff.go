package wtc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type reviewLineRange struct{ start, end int }
type reviewDiffLines map[string][]reviewLineRange

var reviewHunkHeader = regexp.MustCompile(`^@@ -[0-9]+(?:,[0-9]+)? \+([0-9]+)(?:,([0-9]+))? @@`)

// Review comments on the RIGHT side can anchor only inside a new-side hunk,
// including its context lines. Binary files and deleted files have no such lines.
func parseReviewDiff(data []byte) (reviewDiffLines, error) {
	lines := reviewDiffLines{}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	scanner.Buffer(make([]byte, 4096), 16<<20)
	path := ""
	inHunk := false
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "diff --git "):
			path = ""
			inHunk = false
		case !inHunk && strings.HasPrefix(line, "+++ "):
			path = strings.TrimPrefix(line, "+++ ")
			path, _, _ = strings.Cut(path, "\t")
			if strings.HasPrefix(path, `"`) {
				var err error
				path, err = strconv.Unquote(path)
				if err != nil {
					return nil, fmt.Errorf("invalid quoted diff path: %w", err)
				}
			}
			if path == "/dev/null" {
				path = ""
			} else if !strings.HasPrefix(path, "b/") {
				return nil, fmt.Errorf("diff new-side path has no b/ prefix")
			} else {
				path = strings.TrimPrefix(path, "b/")
			}
		case strings.HasPrefix(line, "@@"):
			inHunk = true
			match := reviewHunkHeader.FindStringSubmatch(line)
			if match == nil {
				return nil, fmt.Errorf("invalid diff hunk header")
			}
			start, err := strconv.Atoi(match[1])
			if err != nil {
				return nil, fmt.Errorf("invalid diff hunk start: %w", err)
			}
			count := 1
			if match[2] != "" {
				count, err = strconv.Atoi(match[2])
				if err != nil {
					return nil, fmt.Errorf("invalid diff hunk length: %w", err)
				}
			}
			end := start + count
			if end < start || count > 0 && start == 0 {
				return nil, fmt.Errorf("invalid diff hunk range")
			}
			if path != "" && count > 0 {
				lines[path] = append(lines[path], reviewLineRange{start, end})
			}
		}
	}
	return lines, scanner.Err()
}

func (lines reviewDiffLines) contains(path string, line int) bool {
	for _, span := range lines[path] {
		if line >= span.start && line < span.end {
			return true
		}
	}
	return false
}

// A validation rejection is skippable only when GitHub identifies an invalid
// diff position. Other 422s (body validation, abuse limits) remain real failures.
var reviewHTTP422 = regexp.MustCompile(`(?i)(?:HTTP(?:/[0-9.]+)?\s*|status(?: code)?[:= ]+)422\b`)

func reviewUnresolvablePosition(raw []byte, err error) bool {
	if err == nil || !reviewHTTP422.MatchString(err.Error()) {
		return false
	}
	var response struct {
		Errors []json.RawMessage `json:"errors"`
	}
	if json.Unmarshal(raw, &response) == nil {
		for _, rawError := range response.Errors {
			var detail struct{ Field, Code string }
			if json.Unmarshal(rawError, &detail) == nil && detail.Code == "invalid" {
				switch detail.Field {
				case "line", "position", "path":
					return true
				}
			}
		}
	}
	text := strings.ToLower(string(raw) + "\n" + err.Error())
	for _, phrase := range []string{"line could not be resolved", "position could not be resolved", "line must be part of the diff", "line must be part of diff", "not part of the diff", "not in the diff", "diff hunk can't be blank"} {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}
