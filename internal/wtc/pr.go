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
	"time"

	"golang.org/x/sys/unix"
)

// PRRecord is the collection-local link from a branch to a forge pull request.
// The file format is shared with the shell status and catch-up commands.
type PRRecord struct {
	Repo        string `json:"repo"`
	Number      string `json:"number"`
	Branch      string `json:"branch,omitempty"`
	URL         string `json:"url,omitempty"`
	Title       string `json:"title,omitempty"`
	MergedOn    string `json:"merged_on,omitempty"`
	FinalChecks string `json:"final_checks,omitempty"`
}

const prHeader = "# Local PR enlistment for this collection (not committed; dies with retire).\n# Format: repo  number  [branch]  [url]  [title…]\n# Final merges: # merged-pr repo number merged-at checks\n# Manage: wtc pr enlist|unlist|list\n"

var prNumber = regexp.MustCompile(`^[0-9]+$`)
var prRepoName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func ValidatePRIdentity(repo, number string) error {
	if !prRepoName.MatchString(repo) || repo == "." || repo == ".." || strings.Contains(repo, "..") || !prNumber.MatchString(number) {
		return fmt.Errorf("expected a single repository name and numeric PR number")
	}
	return nil
}

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

func prRecordKey(repo, number string) string { return repo + "\x00" + number }

func parseMergedPR(line string) (repo, number, mergedOn, checks string, ok bool) {
	fields := strings.Fields(line)
	if len(fields) != 6 || fields[0] != "#" || fields[1] != "merged-pr" || ValidatePRIdentity(fields[2], fields[3]) != nil {
		return "", "", "", "", false
	}
	if _, err := time.Parse(time.RFC3339, fields[4]); err != nil {
		return "", "", "", "", false
	}
	switch fields[5] {
	case "SUCCESS", "FAILURE", "NONE":
		return fields[2], fields[3], fields[4], fields[5], true
	}
	return "", "", "", "", false
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
	merged := map[string]PRRecord{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		if record, ok := parsePR(s.Text()); ok {
			records = append(records, record)
		} else if repo, number, when, checks, ok := parseMergedPR(s.Text()); ok {
			merged[prRecordKey(repo, number)] = PRRecord{MergedOn: when, FinalChecks: checks}
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	for i := range records {
		if final, ok := merged[prRecordKey(records[i].Repo, records[i].Number)]; ok {
			records[i].MergedOn, records[i].FinalChecks = final.MergedOn, final.FinalChecks
		}
	}
	return records, nil
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
	return c.withPRLock(func() error { return c.rewritePRsLocked(removeRepo, removeNumber, add) })
}

func (c *Context) withPRLock(action func() error) error {
	path := c.PRFile() + ".lock"
	fd, err := unix.Open(path, unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return fmt.Errorf("open PR registry lock: %w", err)
	}
	lock := os.NewFile(uintptr(fd), path)
	defer lock.Close()
	info, err := lock.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("PR registry lock is not a regular file")
	}
	if err := unix.Flock(fd, unix.LOCK_EX); err != nil {
		return fmt.Errorf("lock PR registry: %w", err)
	}
	defer unix.Flock(fd, unix.LOCK_UN)
	return action()
}

func (c *Context) rewritePRsLocked(removeRepo, removeNumber string, add *PRRecord) error {
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
		if repo, number, _, _, ok := parseMergedPR(line); ok && repo == removeRepo && number == removeNumber && add == nil {
			continue
		}
		fmt.Fprintln(&out, line)
	}
	if add != nil {
		fmt.Fprintln(&out, formatPR(*add))
	}
	return c.writePRFile(out.Bytes())
}

func (c *Context) writePRFile(data []byte) error {
	tmp, err := os.CreateTemp(c.Collection, ".wtc-prs.tmp-")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
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
	return os.Rename(tmp.Name(), c.PRFile())
}

// recordMergedPRs freezes facts that cannot change after a merge. The extra
// comment rows keep the registry readable by older shell tools, which ignore
// comments and continue to parse the ordinary PR rows.
func (c *Context) recordMergedPRs(records []PRRecord, details []statusPRDetail) (int, error) {
	if len(records) != len(details) {
		return 0, fmt.Errorf("PR records and details differ in length")
	}
	updates := map[string]statusPRDetail{}
	for i, record := range records {
		detail := details[i]
		if record.MergedOn != "" || detail.State != "MERGED" || detail.ChecksUnsettled {
			continue
		}
		if _, err := time.Parse(time.RFC3339, detail.MergedOn); err != nil {
			continue
		}
		switch detail.Checks {
		case "PENDING":
			// Checks can still settle after the merge; retry later.
			continue
		case "SUCCESS", "FAILURE", "NONE":
		default:
			continue
		}
		updates[prRecordKey(record.Repo, record.Number)] = detail
	}
	if len(updates) == 0 {
		return 0, nil
	}
	count := 0
	err := c.withPRLock(func() error {
		var err error
		count, err = c.recordMergedPRsLocked(records, updates)
		return err
	})
	return count, err
}

func (c *Context) recordMergedPRsLocked(records []PRRecord, updates map[string]statusPRDetail) (int, error) {
	path := c.PRFile()
	info, err := os.Lstat(path)
	if err != nil {
		return 0, err
	}
	if !info.Mode().IsRegular() {
		return 0, fmt.Errorf("refusing non-regular PR file: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if repo, number, _, _, ok := parseMergedPR(line); ok {
			delete(updates, prRecordKey(repo, number))
		}
	}
	if len(updates) == 0 {
		return 0, nil
	}
	var out bytes.Buffer
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		if record, ok := parsePR(line); ok {
			key := prRecordKey(record.Repo, record.Number)
			if detail, update := updates[key]; update {
				if detail.Title != "" {
					record.Title = cleanPRField(detail.Title)
				}
				fmt.Fprintln(&out, formatPR(record))
				seen[key] = true
				continue
			}
		}
		if repo, number, _, _, ok := parseMergedPR(line); ok && updates[prRecordKey(repo, number)].State == "MERGED" {
			continue
		}
		fmt.Fprintln(&out, line)
	}
	count := 0
	for _, record := range records {
		key := prRecordKey(record.Repo, record.Number)
		if detail, ok := updates[key]; ok && seen[key] {
			fmt.Fprintf(&out, "# merged-pr %s %s %s %s\n", record.Repo, record.Number, detail.MergedOn, detail.Checks)
			count++
		}
	}
	if count == 0 {
		return 0, nil
	}
	return count, c.writePRFile(out.Bytes())
}

func (c *Context) EnlistPR(r PRRecord) (PRRecord, error) {
	r.Repo = cleanPRField(r.Repo)
	r.Number = cleanPRField(r.Number)
	r.Branch = cleanPRField(r.Branch)
	r.URL = cleanPRField(r.URL)
	r.Title = cleanPRField(r.Title)
	if err := ValidatePRIdentity(r.Repo, r.Number); err != nil {
		return r, err
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
	if err := ValidatePRIdentity(repo, number); err != nil {
		return err
	}
	return c.rewritePRs(repo, number, nil)
}
