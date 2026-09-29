package wtc

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type ReviewPostOptions struct {
	Mode   string
	Body   string
	Reason string
	Force  bool
}

type ReviewPostResult struct {
	CommentID    string `json:"comment_id"`
	URL          string `json:"url"`
	InlinePosted int    `json:"inline_posted"`
	InlineFailed int    `json:"inline_failed"`
}

type ReviewInlineRecord struct {
	Key      string `json:"key"`
	Concern  string `json:"concern"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	Severity string `json:"severity"`
	Title    string `json:"title"`
	ID       string `json:"id"`
	URL      string `json:"url"`
	Error    string `json:"error"`
	Resolved bool   `json:"resolved"`
}

func readReviewManifest(bundle string) (ReviewManifest, error) {
	data, err := os.ReadFile(filepath.Join(bundle, "manifest.json"))
	if err != nil {
		return ReviewManifest{}, err
	}
	var manifest ReviewManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return ReviewManifest{}, err
	}
	if manifest.PR == "" || !prNumber.MatchString(manifest.PR) || len(manifest.HeadSHA) < 12 {
		return ReviewManifest{}, fmt.Errorf("review bundle has no valid PR or head")
	}
	return manifest, nil
}

func reviewForgeCommand(worktree, name string, args []string, input []byte) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = worktree
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if input != nil {
		cmd.Stdin = bytes.NewReader(input)
	}
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

func reviewPRHead(manifest ReviewManifest) (string, error) {
	var out []byte
	var err error
	if manifest.Forge == "github" {
		out, err = reviewForgeCommand(manifest.RepoDir, "gh", []string{"pr", "view", manifest.PR, "--repo", manifest.Slug, "--json", "headRefOid"}, nil)
	} else if manifest.Forge == "bitbucket" {
		out, err = reviewForgeCommand(manifest.RepoDir, "bb", []string{"pr", "view", manifest.PR, "--json"}, nil)
	} else {
		return "", fmt.Errorf("unknown forge %q", manifest.Forge)
	}
	if err != nil {
		return "", err
	}
	var p struct {
		HeadRefOid string `json:"headRefOid"`
		Source     struct {
			Commit struct {
				Hash string `json:"hash"`
			} `json:"commit"`
		} `json:"source"`
	}
	if err := json.Unmarshal(out, &p); err != nil {
		return "", err
	}
	if p.HeadRefOid != "" {
		return p.HeadRefOid, nil
	}
	if p.Source.Commit.Hash != "" {
		return p.Source.Commit.Hash, nil
	}
	return "", fmt.Errorf("PR has no head commit")
}

func (c *Context) PostReviewBundle(bundle string, opt ReviewPostOptions) (ReviewPostResult, error) {
	bundle, err := filepath.Abs(bundle)
	if err != nil {
		return ReviewPostResult{}, err
	}
	manifest, err := readReviewManifest(bundle)
	if err != nil {
		return ReviewPostResult{}, err
	}
	worktree, slug, forge, err := c.ReviewRepo(manifest.Repo)
	if err != nil {
		return ReviewPostResult{}, err
	}
	if worktree != manifest.RepoDir || slug != manifest.Slug || forge != manifest.Forge {
		return ReviewPostResult{}, fmt.Errorf("bundle repository does not match this collection")
	}
	mode := opt.Mode
	if mode == "" {
		mode = "summary"
	}
	var body string
	switch mode {
	case "summary":
		data, err := os.ReadFile(filepath.Join(bundle, "summary.md"))
		if err != nil || len(bytes.TrimSpace(data)) == 0 {
			return ReviewPostResult{}, fmt.Errorf("summary.md is missing or empty")
		}
		body = string(data)
		matches := reviewStatusLine.FindAllStringSubmatch(body, -1)
		if len(matches) != 1 || matches[0][1] != manifest.HeadSHA {
			return ReviewPostResult{}, fmt.Errorf("summary needs exactly one status line for the bundle head")
		}
	case "progress":
		if opt.Body != "" {
			data, err := os.ReadFile(opt.Body)
			if err != nil || len(bytes.TrimSpace(data)) == 0 {
				return ReviewPostResult{}, fmt.Errorf("progress body is missing or empty")
			}
			body = string(data)
		} else {
			body = fmt.Sprintf("⏳ **Local review: in progress**\n\nRound %d, head `%s`. Separate reviewers are running.\n\n`wtc-review v1 head=%s verdict=pending blockers=0 round=%d lead=pending`\n", manifest.Round, manifest.HeadSHA[:7], manifest.HeadSHA, manifest.Round)
		}
		matches := reviewStatusLine.FindAllStringSubmatch(body, -1)
		if len(matches) != 1 || matches[0][1] != manifest.HeadSHA || matches[0][2] != "pending" {
			return ReviewPostResult{}, fmt.Errorf("progress body needs one pending status line for the bundle head")
		}
	case "failed":
		body = fmt.Sprintf("❌ **Local review: failed**\n\nRound %d, head `%s`. The run did not finish; the gate stays closed.\n", manifest.Round, manifest.HeadSHA[:7])
		if opt.Reason != "" {
			body += "\nReason: " + strings.ReplaceAll(opt.Reason, "\n", " ") + "\n"
		}
		body += fmt.Sprintf("\n`wtc-review v1 head=%s verdict=error blockers=0 round=%d lead=error`\n", manifest.HeadSHA, manifest.Round)
		opt.Force = true
	default:
		return ReviewPostResult{}, fmt.Errorf("invalid review post mode %q", mode)
	}
	prHead, err := reviewPRHead(manifest)
	if err != nil && !opt.Force {
		return ReviewPostResult{}, fmt.Errorf("cannot verify PR head: %w", err)
	}
	if err == nil && !SameReviewSHA(manifest.HeadSHA, prHead) && !opt.Force {
		return ReviewPostResult{}, fmt.Errorf("stale bundle: reviewed %s, PR head %s", manifest.HeadSHA[:7], prHead[:min(7, len(prHead))])
	}
	idPath := filepath.Join(bundle, "comment.id")
	idBytes, _ := os.ReadFile(idPath)
	cid := strings.TrimSpace(string(idBytes))
	if cid != "" && !prNumber.MatchString(cid) {
		return ReviewPostResult{}, fmt.Errorf("invalid saved comment id")
	}
	var posted ReviewPostResult
	if cid != "" {
		posted, err = postReviewComment(manifest, cid, body)
		if err != nil {
			fmt.Fprintf(os.Stderr, "wtc: warning: update of review comment %s failed; creating a new comment: %v\n", cid, err)
		}
	}
	if cid == "" || err != nil {
		posted, err = postReviewComment(manifest, "", body)
		if err != nil {
			return ReviewPostResult{}, err
		}
	}
	if posted.CommentID != "" {
		if err := os.WriteFile(idPath, []byte(posted.CommentID+"\n"), 0644); err != nil {
			return posted, err
		}
	}
	if mode == "summary" {
		if posted.CommentID != "" {
			receipt := reviewReceiptPath(c.Collection, manifest.Forge, manifest.Slug, manifest.PR, manifest.HeadSHA, posted.CommentID, reviewStatusLine.FindStringSubmatch(body)[2])
			if err := os.MkdirAll(filepath.Dir(receipt), 0755); err != nil {
				return posted, err
			}
			if err := os.WriteFile(receipt, nil, 0600); err != nil {
				return posted, err
			}
		}
		posted.InlinePosted, posted.InlineFailed = postReviewInline(manifest, bundle)
	}
	return posted, nil
}

func postReviewComment(m ReviewManifest, cid, body string) (ReviewPostResult, error) {
	if m.Forge == "github" {
		payload, _ := json.Marshal(map[string]string{"body": body})
		method := "POST"
		endpoint := fmt.Sprintf("repos/%s/issues/%s/comments", m.Slug, m.PR)
		if cid != "" {
			method = "PATCH"
			endpoint = fmt.Sprintf("repos/%s/issues/comments/%s", m.Slug, cid)
		}
		out, err := reviewForgeCommand(m.RepoDir, "gh", []string{"api", "-X", method, endpoint, "--input", "-"}, payload)
		if err != nil {
			return ReviewPostResult{}, err
		}
		var result struct {
			ID      int64  `json:"id"`
			HTMLURL string `json:"html_url"`
		}
		if err := json.Unmarshal(out, &result); err != nil || result.ID == 0 {
			return ReviewPostResult{}, fmt.Errorf("GitHub comment response has no id: %v", err)
		}
		return ReviewPostResult{CommentID: strconv.FormatInt(result.ID, 10), URL: result.HTMLURL}, nil
	}
	var args []string
	if cid == "" {
		args = []string{"pr", "comments", "add", m.PR, body, "--json"}
	} else {
		args = []string{"pr", "comments", "edit", m.PR, cid, body}
	}
	out, err := reviewForgeCommand(m.RepoDir, "bb", args, nil)
	if err != nil {
		return ReviewPostResult{}, err
	}
	if cid != "" {
		return ReviewPostResult{CommentID: cid, URL: m.URL}, nil
	}
	var result struct {
		ID    int64 `json:"id"`
		Links struct {
			HTML struct {
				Href string `json:"href"`
			} `json:"html"`
		} `json:"links"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return ReviewPostResult{}, err
	}
	if result.ID == 0 {
		return ReviewPostResult{}, fmt.Errorf("Bitbucket comment response has no id")
	}
	return ReviewPostResult{CommentID: strconv.FormatInt(result.ID, 10), URL: result.Links.HTML.Href}, nil
}

func inlineReviewKey(concern, path string, line int, title string) string {
	text := fmt.Sprintf("%s\n%s\n%d\n%s", concern, path, line, title)
	h := sha1.Sum([]byte(text))
	return hex.EncodeToString(h[:])[:10]
}

func readInlineRecords(bundle string) []ReviewInlineRecord {
	data, err := os.ReadFile(filepath.Join(bundle, "inline-comments.json"))
	if err != nil {
		return nil
	}
	var rows []ReviewInlineRecord
	_ = json.Unmarshal(data, &rows)
	return rows
}

func writeInlineRecords(bundle string, rows []ReviewInlineRecord) error {
	data, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(bundle, "inline-comments.json"), append(data, '\n'), 0644)
}

var reviewInlineMarker = regexp.MustCompile(`wtc-review-inline v1 key=([0-9a-f]{10})`)

func priorReviewInlineKeys(m ReviewManifest, bundle string) map[string]bool {
	keys := map[string]bool{}
	data, _ := os.ReadFile(filepath.Join(bundle, "prior", "inline-keys.txt"))
	for _, line := range strings.Split(string(data), "\n") {
		if len(line) == 10 {
			keys[line] = true
		}
	}
	if m.Forge == "github" {
		out, err := reviewForgeCommand(m.RepoDir, "gh", []string{"api", fmt.Sprintf("repos/%s/pulls/%s/comments?per_page=100", m.Slug, m.PR), "--paginate", "--slurp"}, nil)
		if err == nil {
			for _, match := range reviewInlineMarker.FindAllStringSubmatch(string(out), -1) {
				keys[match[1]] = true
			}
		}
	} else {
		out, err := reviewForgeCommand(m.RepoDir, "bb", []string{"pr", "comments", "list", m.PR, "--all", "--json"}, nil)
		if err == nil {
			for _, match := range reviewInlineMarker.FindAllStringSubmatch(string(out), -1) {
				keys[match[1]] = true
			}
		}
	}
	return keys
}

func postReviewInline(m ReviewManifest, bundle string) (posted, failed int) {
	prior := priorReviewInlineKeys(m, bundle)
	rows := readInlineRecords(bundle)
	for _, row := range rows {
		if row.ID != "" {
			prior[row.Key] = true
		}
	}
	paths, _ := filepath.Glob(filepath.Join(bundle, "findings", "*.json"))
	sort.Strings(paths)
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			failed++
			continue
		}
		var file struct {
			Concern  string `json:"concern"`
			Findings []struct {
				Severity   string `json:"severity"`
				File       string `json:"file"`
				Line       int    `json:"line"`
				Title      string `json:"title"`
				Detail     string `json:"detail"`
				Suggestion string `json:"suggestion"`
				Prior      string `json:"prior"`
			} `json:"findings"`
		}
		if json.Unmarshal(data, &file) != nil {
			failed++
			continue
		}
		for _, f := range file.Findings {
			if f.Prior == "addressed" || f.Line < 1 || f.File == "" || filepath.IsAbs(f.File) || strings.Contains(f.File, "..") || f.Title == "" {
				continue
			}
			key := inlineReviewKey(file.Concern, f.File, f.Line, f.Title)
			if prior[key] {
				continue
			}
			prior[key] = true
			body := fmt.Sprintf("**%s** · `%s` — %s\n\n%s\n", f.Severity, file.Concern, f.Title, f.Detail)
			if f.Suggestion != "" {
				body += "\nSuggestion: " + f.Suggestion + "\n"
			}
			body += fmt.Sprintf("\nRound %d.\n\n`wtc-review-inline v1 key=%s concern=%s file=%s line=%d`\n", m.Round, key, file.Concern, f.File, f.Line)
			row := ReviewInlineRecord{Key: key, Concern: file.Concern, File: f.File, Line: f.Line, Severity: f.Severity, Title: f.Title}
			if m.Forge == "github" {
				payload, _ := json.Marshal(map[string]any{"body": body, "commit_id": m.HeadSHA, "path": f.File, "line": f.Line, "side": "RIGHT"})
				out, err := reviewForgeCommand(m.RepoDir, "gh", []string{"api", "-X", "POST", fmt.Sprintf("repos/%s/pulls/%s/comments", m.Slug, m.PR), "--input", "-"}, payload)
				if err == nil {
					var result struct {
						ID      int64  `json:"id"`
						HTMLURL string `json:"html_url"`
					}
					if json.Unmarshal(out, &result) == nil && result.ID != 0 {
						row.ID = strconv.FormatInt(result.ID, 10)
						row.URL = result.HTMLURL
					}
				}
				if row.ID == "" {
					row.Error = "post failed"
				}
			} else {
				out, err := reviewForgeCommand(m.RepoDir, "bb", []string{"pr", "comments", "add", m.PR, body, "--file", f.File, "--line-to", strconv.Itoa(f.Line), "--json"}, nil)
				if err == nil {
					var result struct {
						ID int64 `json:"id"`
					}
					if json.Unmarshal(out, &result) == nil && result.ID != 0 {
						row.ID = strconv.FormatInt(result.ID, 10)
					}
				}
				if row.ID == "" {
					row.Error = "post failed"
				}
			}
			if row.ID == "" {
				failed++
			} else {
				posted++
			}
			rows = append(rows, row)
			_ = writeInlineRecords(bundle, rows)
		}
	}
	return posted, failed
}
