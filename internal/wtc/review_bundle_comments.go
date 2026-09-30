package wtc

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type bundleComment struct {
	Body   string `json:"body"`
	When   string `json:"createdAt"`
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
	User struct {
		Login string `json:"login"`
	} `json:"user"`
	CreatedAt string `json:"created_at"`
}

func (c bundleComment) author() string {
	if c.Author.Login != "" {
		return c.Author.Login
	}
	if c.User.Login != "" {
		return c.User.Login
	}
	return "unknown"
}

func (c bundleComment) when() string {
	if c.When != "" {
		return c.When
	}
	return c.CreatedAt
}

func isBundleStatusComment(body string) bool {
	lines := strings.Split(strings.TrimSpace(body), "\n")
	if len(lines) < 2 || !strings.Contains(lines[0], "**Local review:") {
		return false
	}
	status := strings.TrimSpace(lines[len(lines)-1])
	return strings.HasPrefix(status, "`wtc-review v1 ") && strings.HasSuffix(status, "`") && reviewStatusLine.MatchString(status)
}

// Forge comments are optional review context. An unavailable forge leaves a
// usable local bundle, while posting still verifies the current remote head.
func copyPublicReviewComments(dir string, manifest ReviewManifest) {
	if manifest.PR == "" || manifest.Forge != "github" {
		return
	}
	conversation, err := reviewForgeCommand(manifest.RepoDir, "gh", []string{"pr", "view", manifest.PR, "--repo", manifest.Slug, "--json", "comments"}, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "wtc: warning: review comments unavailable: %v\n", err)
		return
	}
	inline, err := reviewForgeCommand(manifest.RepoDir, "gh", []string{"api", fmt.Sprintf("repos/%s/pulls/%s/comments?per_page=100", manifest.Slug, manifest.PR), "--paginate", "--slurp"}, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "wtc: warning: inline review comments unavailable: %v\n", err)
		return
	}
	comments, err := parseBundleComments(conversation, inline)
	if err != nil {
		fmt.Fprintf(os.Stderr, "wtc: warning: review comments could not be parsed: %v\n", err)
		return
	}
	if err := writeBundleComments(dir, comments); err != nil {
		fmt.Fprintf(os.Stderr, "wtc: warning: review comments could not be saved: %v\n", err)
	}
}

func parseBundleComments(conversation, inline []byte) ([]bundleComment, error) {
	var outer struct {
		Comments []bundleComment `json:"comments"`
	}
	if err := json.Unmarshal(conversation, &outer); err != nil {
		return nil, err
	}
	comments := outer.Comments
	var pages []json.RawMessage
	if err := json.Unmarshal(inline, &pages); err != nil {
		return nil, err
	}
	for _, page := range pages {
		var rows []bundleComment
		if err := json.Unmarshal(page, &rows); err != nil {
			return nil, err
		}
		comments = append(comments, rows...)
	}
	sort.SliceStable(comments, func(i, j int) bool { return comments[i].when() < comments[j].when() })
	return comments, nil
}

func writeBundleComments(dir string, comments []bundleComment) error {
	if len(comments) == 0 {
		return nil
	}
	prior := filepath.Join(dir, "prior")
	if err := os.MkdirAll(prior, 0755); err != nil {
		return err
	}
	latestReview := -1
	keys := map[string]bool{}
	for i, comment := range comments {
		if isBundleStatusComment(comment.Body) {
			latestReview = i
		}
		for _, match := range reviewInlineMarker.FindAllStringSubmatch(comment.Body, -1) {
			keys[match[1]] = true
		}
	}
	if len(keys) > 0 {
		data, err := os.ReadFile(filepath.Join(prior, "inline-keys.txt"))
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		for _, key := range strings.Fields(string(data)) {
			if len(key) == 10 {
				keys[key] = true
			}
		}
		var list []string
		for key := range keys {
			list = append(list, key)
		}
		sort.Strings(list)
		if err := os.WriteFile(filepath.Join(prior, "inline-keys.txt"), []byte(strings.Join(list, "\n")+"\n"), 0644); err != nil {
			return err
		}
	}
	var transcript strings.Builder
	for i, comment := range comments {
		if i <= latestReview || isBundleStatusComment(comment.Body) || strings.TrimSpace(comment.Body) == "" {
			continue
		}
		fmt.Fprintf(&transcript, "### %s, %s\n\n%s\n\n", comment.author(), comment.when(), strings.TrimSpace(comment.Body))
	}
	if transcript.Len() > 0 {
		return os.WriteFile(filepath.Join(prior, "comments.md"), []byte(transcript.String()), 0644)
	}
	return nil
}
