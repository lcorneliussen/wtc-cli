package wtc

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// PRRecord is the collection-local link from a branch to a forge pull request.
// The file format is shared with the shell status and catch-up commands.
type PRRecord struct {
	Repo   string `json:"repo"`
	Number string `json:"number"`
	Branch string `json:"branch,omitempty"`
	URL    string `json:"url,omitempty"`
	Title  string `json:"title,omitempty"`
}

const prHeader = "# Local PR enlistment for this collection (not committed; dies with retire).\n# Format: repo  number  [branch]  [url]  [title…]\n# Manage: wtc pr enlist|unlist|list\n"

var prNumber = regexp.MustCompile(`^[0-9]+$`)

func (c *Context) PRFile() string { return filepath.Join(c.Collection, ".wtc-prs") }

func cleanPRField(s string) string { return strings.Join(strings.Fields(s), " ") }

func prURL(remote, number string) string {
	remote = strings.TrimSuffix(strings.TrimSpace(remote), ".git")
	for _, forge := range []struct{ host, suffix string }{{"github.com", "pull"}, {"bitbucket.org", "pull-requests"}} {
		for _, prefix := range []string{"https://" + forge.host + "/", "http://" + forge.host + "/", "git@" + forge.host + ":", "ssh://git@" + forge.host + "/"} {
			if strings.HasPrefix(remote, prefix) {
				slug := strings.TrimPrefix(remote, prefix)
				if strings.Count(slug, "/") == 1 {
					return "https://" + forge.host + "/" + slug + "/" + forge.suffix + "/" + number
				}
			}
		}
	}
	return ""
}

func (c *Context) inferredPRURL(repo, number string) string {
	for _, r := range c.Registry.Repos {
		if r.Name == repo && r.Remote != "" {
			return prURL(r.Remote, number)
		}
	}
	command := exec.Command("git", "-C", filepath.Join(c.Collection, repo), "remote", "get-url", "origin")
	if b, err := command.Output(); err == nil {
		return prURL(string(b), number)
	}
	return ""
}

func parsePR(line string) (PRRecord, bool) {
	fields := strings.Fields(line)
	if len(fields) < 2 || strings.HasPrefix(fields[0], "#") {
		return PRRecord{}, false
	}
	r := PRRecord{Repo: fields[0], Number: fields[1]}
	if len(fields) > 2 && fields[2] != "-" {
		r.Branch = fields[2]
	}
	if len(fields) > 3 && fields[3] != "-" {
		r.URL = fields[3]
	}
	if len(fields) > 4 {
		r.Title = strings.Join(fields[4:], " ")
	}
	return r, true
}

func (c *Context) ListPRs() ([]PRRecord, error) {
	f, err := os.Open(c.PRFile())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var records []PRRecord
	s := bufio.NewScanner(f)
	for s.Scan() {
		if record, ok := parsePR(s.Text()); ok {
			records = append(records, record)
		}
	}
	return records, s.Err()
}

func formatPR(r PRRecord) string {
	parts := []string{r.Repo, r.Number}
	if r.Branch != "" || r.URL != "" || r.Title != "" {
		branch := r.Branch
		if branch == "" {
			branch = "-"
		}
		parts = append(parts, branch)
	}
	if r.URL != "" || r.Title != "" {
		url := r.URL
		if url == "" {
			url = "-"
		}
		parts = append(parts, url)
	}
	if r.Title != "" {
		parts = append(parts, r.Title)
	}
	return strings.Join(parts, " ")
}

func (c *Context) rewritePRs(removeRepo, removeNumber string, add *PRRecord) error {
	path := c.PRFile()
	info, err := os.Lstat(path)
	if err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("refusing non-regular PR file: %s", path)
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if add == nil {
			return nil
		}
		data = []byte(prHeader)
	} else if err != nil {
		return err
	}
	var out bytes.Buffer
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		if record, ok := parsePR(line); ok && record.Repo == removeRepo && record.Number == removeNumber {
			continue
		}
		fmt.Fprintln(&out, line)
	}
	if add != nil {
		fmt.Fprintln(&out, formatPR(*add))
	}
	tmp, err := os.CreateTemp(c.Collection, ".wtc-prs.tmp-")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(out.Bytes()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func (c *Context) EnlistPR(r PRRecord) (PRRecord, error) {
	r.Repo = cleanPRField(r.Repo)
	r.Number = cleanPRField(r.Number)
	r.Branch = cleanPRField(r.Branch)
	r.URL = cleanPRField(r.URL)
	r.Title = cleanPRField(r.Title)
	if r.Repo == "" || strings.Contains(r.Repo, " ") || !prNumber.MatchString(r.Number) {
		return r, fmt.Errorf("expected a repository name and numeric PR number")
	}
	if r.Branch == "" {
		command := exec.Command("git", "-C", filepath.Join(c.Collection, r.Repo), "symbolic-ref", "-q", "--short", "HEAD")
		if b, err := command.Output(); err == nil {
			r.Branch = strings.TrimSpace(string(b))
		}
	}
	if r.URL == "" {
		r.URL = c.inferredPRURL(r.Repo, r.Number)
	}
	if err := c.rewritePRs(r.Repo, r.Number, &r); err != nil {
		return r, err
	}
	return r, nil
}

func (c *Context) UnlistPR(repo, number string) error {
	if repo == "" || !prNumber.MatchString(number) {
		return fmt.Errorf("expected a repository name and numeric PR number")
	}
	return c.rewritePRs(repo, number, nil)
}
