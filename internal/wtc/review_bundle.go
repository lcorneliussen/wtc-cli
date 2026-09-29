package wtc

import (
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

type ReviewBundleOptions struct {
	Repo  string
	PR    string
	Base  string
	Head  string
	Dir   string
	Round int
}

type ReviewManifest struct {
	Repo       string `json:"repo"`
	PR         string `json:"pr"`
	Forge      string `json:"forge"`
	Slug       string `json:"slug"`
	URL        string `json:"url"`
	BaseRef    string `json:"base_ref"`
	BaseSHA    string `json:"base_sha"`
	HeadSHA    string `json:"head_sha"`
	HeadBranch string `json:"head_branch"`
	Round      int    `json:"round"`
	RepoDir    string `json:"repo_dir"`
	Collection string `json:"collection"`
}

type ReviewBundle struct {
	Dir      string         `json:"dir"`
	Manifest ReviewManifest `json:"manifest"`
	Files    int            `json:"files"`
	Concerns int            `json:"concerns"`
}

func reviewGit(worktree string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", append([]string{"-C", worktree}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

func reviewRef(worktree, ref string) (string, error) {
	out, err := reviewGit(worktree, "rev-parse", "--verify", ref+"^{commit}")
	return strings.TrimSpace(string(out)), err
}

func (c *Context) BuildPublicReviewBundle(opt ReviewBundleOptions) (ReviewBundle, error) {
	worktree, slug, forge, err := c.ReviewRepo(opt.Repo)
	if err != nil {
		return ReviewBundle{}, err
	}
	if opt.PR != "" && !prNumber.MatchString(opt.PR) {
		return ReviewBundle{}, fmt.Errorf("invalid PR number %q", opt.PR)
	}
	if opt.Head == "" {
		opt.Head = "HEAD"
	}
	branchBytes, err := reviewGit(worktree, "branch", "--show-current")
	if err != nil {
		return ReviewBundle{}, err
	}
	branch := strings.TrimSpace(string(branchBytes))
	if opt.PR == "" && branch != "" {
		if enlisted, e := c.EnlistedReviewPR(opt.Repo); e == nil {
			opt.PR = enlisted
		}
	}
	prTitle, prBody, prBase, prHead, prBranch, prURL := "", "", "", "", "", ""
	if opt.PR != "" {
		var raw []byte
		if forge == "github" {
			raw, err = exec.Command("gh", "pr", "view", opt.PR, "--repo", slug, "--json", "title,body,baseRefName,headRefOid,headRefName,url").Output()
		} else {
			cmd := exec.Command("bb", "pr", "view", opt.PR, "--json")
			cmd.Dir = worktree
			raw, err = cmd.Output()
		}
		if err != nil {
			return ReviewBundle{}, fmt.Errorf("read PR #%s: %w", opt.PR, err)
		}
		var p struct {
			Title       string `json:"title"`
			Body        string `json:"body"`
			Description string `json:"description"`
			BaseRefName string `json:"baseRefName"`
			HeadRefOid  string `json:"headRefOid"`
			HeadRefName string `json:"headRefName"`
			URL         string `json:"url"`
			Destination struct {
				Branch struct {
					Name string `json:"name"`
				} `json:"branch"`
			} `json:"destination"`
			Source struct {
				Branch struct {
					Name string `json:"name"`
				} `json:"branch"`
				Commit struct {
					Hash string `json:"hash"`
				} `json:"commit"`
			} `json:"source"`
			Links struct {
				HTML struct {
					Href string `json:"href"`
				} `json:"html"`
			} `json:"links"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return ReviewBundle{}, err
		}
		prTitle, prBody, prBase, prHead, prBranch, prURL = p.Title, p.Body, p.BaseRefName, p.HeadRefOid, p.HeadRefName, p.URL
		if prBody == "" {
			prBody = p.Description
		}
		if prBase == "" {
			prBase = p.Destination.Branch.Name
		}
		if prHead == "" {
			prHead = p.Source.Commit.Hash
		}
		if prBranch == "" {
			prBranch = p.Source.Branch.Name
		}
		if prURL == "" {
			prURL = p.Links.HTML.Href
		}
		if opt.Head == "HEAD" && prBranch != "" && branch != prBranch {
			return ReviewBundle{}, fmt.Errorf("worktree is on %q, PR #%s is on %q; check out the PR branch first", branch, opt.PR, prBranch)
		}
	}
	headSHA, err := reviewRef(worktree, opt.Head)
	if err != nil {
		return ReviewBundle{}, err
	}
	if opt.Head != "HEAD" {
		branch = opt.Head
	}
	if prHead != "" && !SameReviewSHA(headSHA, prHead) {
		fmt.Fprintf(os.Stderr, "wtc: warning: local head %s differs from PR head %s\n", headSHA[:7], prHead[:min(7, len(prHead))])
	}
	baseRef := opt.Base
	if baseRef == "" && prBase != "" {
		baseRef = "origin/" + prBase
	}
	if baseRef == "" {
		baseRef = "origin/main"
		for _, r := range c.Registry.Repos {
			if r.Name == opt.Repo || opt.Repo == "harness" && r.Name == c.Config.Harness.Name {
				if r.DefaultRef != "" {
					baseRef = r.DefaultRef
				}
				break
			}
		}
	}
	if _, err := reviewRef(worktree, baseRef); err != nil {
		return ReviewBundle{}, fmt.Errorf("base ref %q not found: %w", baseRef, err)
	}
	baseBytes, err := reviewGit(worktree, "merge-base", baseRef, headSHA)
	if err != nil {
		return ReviewBundle{}, err
	}
	baseSHA := strings.TrimSpace(string(baseBytes))
	diff, err := reviewGit(worktree, "diff", "--no-color", baseSHA, headSHA)
	if err != nil {
		return ReviewBundle{}, err
	}
	changed, err := reviewGit(worktree, "diff", "--name-only", baseSHA, headSHA)
	if err != nil {
		return ReviewBundle{}, err
	}
	log, err := reviewGit(worktree, "log", "--oneline", baseSHA+".."+headSHA)
	if err != nil {
		return ReviewBundle{}, err
	}
	root := filepath.Join(c.Collection, ".wtc-reviews")
	round := opt.Round
	if round == 0 {
		round = nextReviewRound(root, opt.Repo, opt.PR, branch)
	}
	if round < 1 {
		return ReviewBundle{}, fmt.Errorf("round must be positive")
	}
	dir := opt.Dir
	if dir == "" {
		name := opt.Repo + "-br-" + headSHA[:7]
		if opt.PR != "" {
			name = opt.Repo + "-pr" + opt.PR + "-" + headSHA[:7]
		}
		dir = filepath.Join(root, fmt.Sprintf("%s-r%d", name, round))
	}
	dir, err = filepath.Abs(dir)
	if err != nil {
		return ReviewBundle{}, err
	}
	if entries, e := os.ReadDir(dir); e == nil && len(entries) > 0 {
		return ReviewBundle{}, fmt.Errorf("bundle directory is not empty: %s", dir)
	} else if e != nil && !os.IsNotExist(e) {
		return ReviewBundle{}, e
	}
	if err := os.MkdirAll(filepath.Join(dir, "concerns"), 0755); err != nil {
		return ReviewBundle{}, err
	}
	manifest := ReviewManifest{Repo: opt.Repo, PR: opt.PR, Forge: forge, Slug: slug, URL: prURL, BaseRef: baseRef, BaseSHA: baseSHA, HeadSHA: headSHA, HeadBranch: branch, Round: round, RepoDir: worktree, Collection: filepath.Base(c.Collection)}
	prText := fmt.Sprintf("# %s\n\n%s\n", prTitle, prBody)
	if opt.PR == "" {
		prText = fmt.Sprintf("# %s\n\n(branch-only review)\n", branch)
	}
	for rel, body := range map[string][]byte{"pr.md": []byte(prText), "diff.patch": diff, "changed-files.txt": changed, "log.txt": log} {
		if err := os.WriteFile(filepath.Join(dir, rel), body, 0644); err != nil {
			return ReviewBundle{}, err
		}
	}
	if err := writeReviewManifest(dir, manifest); err != nil {
		return ReviewBundle{}, err
	}
	concerns, err := c.copyPublicReviewConcerns(dir, worktree, baseSHA, headSHA, string(changed))
	if err != nil {
		return ReviewBundle{}, err
	}
	files := 0
	for _, path := range strings.Split(strings.TrimSuffix(string(changed), "\n"), "\n") {
		if path != "" {
			files++
		}
	}
	return ReviewBundle{Dir: dir, Manifest: manifest, Files: files, Concerns: concerns}, nil
}

func nextReviewRound(root, repo, pr, branch string) int {
	maxRound := 0
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		var m ReviewManifest
		data, err := os.ReadFile(filepath.Join(root, e.Name(), "manifest.json"))
		if err != nil || json.Unmarshal(data, &m) != nil {
			continue
		}
		if m.Repo == repo && m.PR == pr && (pr != "" || m.HeadBranch == branch) && m.Round > maxRound {
			maxRound = m.Round
		}
	}
	return maxRound + 1
}

func writeReviewManifest(dir string, m ReviewManifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), append(data, '\n'), 0644); err != nil {
		return err
	}
	fields := [][2]string{{"REPO", m.Repo}, {"PR", m.PR}, {"FORGE", m.Forge}, {"SLUG", m.Slug}, {"URL", m.URL}, {"BASE_REF", m.BaseRef}, {"BASE_SHA", m.BaseSHA}, {"HEAD_SHA", m.HeadSHA}, {"HEAD_BRANCH", m.HeadBranch}, {"ROUND", strconv.Itoa(m.Round)}, {"REPO_DIR", m.RepoDir}, {"COLLECTION", m.Collection}, {"DOWNSTREAM", ""}, {"UPSTREAM", ""}}
	var b strings.Builder
	b.WriteString("# review bundle manifest (sourceable)\n")
	for _, field := range fields {
		fmt.Fprintf(&b, "%s='%s'\n", field[0], strings.ReplaceAll(field[1], "'", "'\\''"))
	}
	return os.WriteFile(filepath.Join(dir, "manifest.env"), []byte(b.String()), 0644)
}

var reviewConcernID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func (c *Context) copyPublicReviewConcerns(dir, worktree, baseSHA, headSHA, changed string) (int, error) {
	// A public bundle reads only generic concerns from the review base. Local
	// overlays and other repository snapshots may contain private identities.
	sourceSHA := baseSHA
	paths, err := reviewGit(worktree, "ls-tree", "-r", "--name-only", sourceSHA, "review/concerns")
	if err != nil {
		return 0, err
	}
	if len(strings.TrimSpace(string(paths))) == 0 {
		sourceSHA = headSHA
		paths, err = reviewGit(worktree, "ls-tree", "-r", "--name-only", sourceSHA, "review/concerns")
		if err != nil {
			return 0, err
		}
		fmt.Fprintln(os.Stderr, "wtc: warning: review base has no generic concerns; using head concerns for bootstrap")
	}
	var names []string
	for _, path := range strings.Split(strings.TrimSpace(string(paths)), "\n") {
		if strings.HasPrefix(path, "review/concerns/") && strings.HasSuffix(path, ".md") {
			names = append(names, path)
		}
	}
	if len(names) == 0 {
		return 0, fmt.Errorf("review base has no generic concerns; public bundle cannot use local overlays")
	}
	sort.Strings(names)
	count := 0
	for _, path := range names {
		body, err := reviewGit(worktree, "show", sourceSHA+":"+path)
		if err != nil {
			return count, err
		}
		id := strings.TrimSuffix(filepath.Base(path), ".md")
		for _, line := range strings.Split(string(body), "\n") {
			if strings.HasPrefix(line, "id:") {
				id = strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "id:")), `"'`)
				break
			}
		}
		if !reviewConcernID.MatchString(id) {
			return count, fmt.Errorf("invalid concern id in %s", path)
		}
		if !reviewConcernApplies(string(body), changed) {
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, "concerns", id+".md"), body, 0644); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func reviewConcernApplies(body, changed string) bool {
	applies := "always"
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "applies:") {
			applies = strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "applies:")), `"'`)
			break
		}
	}
	if applies == "" || applies == "always" {
		return true
	}
	if applies == "never" {
		return false
	}
	for _, pattern := range strings.Fields(applies) {
		pattern = strings.TrimPrefix(pattern, "/")
		for _, file := range strings.Fields(changed) {
			if matchReviewGlob(pattern, file) || !strings.Contains(pattern, "/") && matchReviewGlob(pattern, filepath.Base(file)) {
				return true
			}
		}
	}
	return false
}

func matchReviewGlob(pattern, file string) bool {
	var b strings.Builder
	b.WriteByte('^')
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				b.WriteString(".*")
				i++
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(pattern[i])))
		}
	}
	b.WriteByte('$')
	return regexp.MustCompile(b.String()).MatchString(file)
}
