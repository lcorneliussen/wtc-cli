package wtc

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// StatusSnapshot is the canonical status schema shared by the one-shot view
// and, once ported, the live renderer. This is the local Git portion; forge
// and process facts are added by later collection stages.
type StatusSnapshot struct {
	Schema               int            `json:"schema"`
	Collection           string         `json:"collection"`
	GeneratedAt          string         `json:"generated_at"`
	ShowCollectionColumn bool           `json:"show_collection_column"`
	StaleCount           int            `json:"stale_count"`
	Repos                []StatusRepo   `json:"repos"`
	PRs                  []StatusPRRow  `json:"prs"`
	Orphans              []StatusOrphan `json:"orphans"`
	PRsEmptyHint         bool           `json:"prs_empty_hint,omitempty"`
}

type StatusRepo struct {
	Collection    string         `json:"collection"`
	Dir           string         `json:"dir"`
	Repo          string         `json:"repo"`
	Worktree      string         `json:"worktree"`
	Slug          string         `json:"slug"`
	Forge         string         `json:"forge,omitempty"`
	BranchKind    string         `json:"branch_kind"`
	Branch        string         `json:"branch"`
	BranchDisplay string         `json:"branch_display"`
	Ahead         int            `json:"ahead"`
	Behind        int            `json:"behind"`
	Tree          string         `json:"tree"`
	Changed       int            `json:"changed"`
	Conflict      bool           `json:"conflict,omitempty"`
	Operation     string         `json:"operation,omitempty"`
	PR            *StatusPRFacts `json:"pr"`
	Tip           *StatusBuild   `json:"tip"`
	Prod          *StatusBuild   `json:"prod"`
}

type StatusPRFacts struct {
	Number string `json:"number"`
	URL    string `json:"url,omitempty"`
	Checks string `json:"checks"`
	Merge  string `json:"merge"`
	Review string `json:"review"`
	Draft  bool   `json:"draft"`
}

type StatusBuild struct {
	Branch string  `json:"branch"`
	Checks *string `json:"checks"`
	Build  *string `json:"build"`
	URL    *string `json:"url"`
}

type StatusPRRow struct {
	Repo            string  `json:"repo"`
	Number          string  `json:"number"`
	State           string  `json:"state,omitempty"`
	Checks          *string `json:"checks"`
	Merge           *string `json:"merge"`
	Review          *string `json:"review"`
	Title           string  `json:"title"`
	DisplayTitle    string  `json:"display_title"`
	Slug            string  `json:"slug"`
	URL             *string `json:"url"`
	FollowTipBuild  *string `json:"follow_tip_build"`
	FollowProdBuild *string `json:"follow_prod_build"`
	Archived        bool    `json:"archived"`
	MergedOn        *string `json:"merged_on"`
	Draft           bool    `json:"draft"`
	OnBranch        bool    `json:"on_branch"`
}

type StatusOrphan struct {
	Repo   string `json:"repo"`
	Branch string `json:"branch"`
	State  string `json:"state"`
}

func (c *Context) statusProgress(message string) {
	if c.StatusProgress != nil {
		c.StatusProgress(message)
	}
}

func statusCount(worktree, rangeSpec string) (int, error) {
	value, err := statusGit(worktree, "rev-list", "--count", rangeSpec)
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid rev-list count %q: %w", value, err)
	}
	return n, nil
}

func statusLocalRepo(t catchUpTarget) (StatusRepo, error) {
	dir := filepath.Base(t.path)
	row := StatusRepo{Collection: t.collection, Dir: dir, Repo: t.repo,
		Worktree: t.path, Tree: "clean"}
	remote, err := statusGit(t.path, "remote", "get-url", "origin")
	if err == nil {
		row.Slug, row.Forge = catchUpForge(remote)
	}
	branch, err := statusGit(t.path, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err == nil {
		row.BranchKind = "branch"
		row.Branch = branch
		row.BranchDisplay = branch
	} else {
		row.BranchKind = "detached"
		row.Branch = strings.TrimPrefix(t.ref, "origin/")
		row.BranchDisplay = "⌂ " + row.Branch
	}
	upstream := t.ref
	if row.BranchKind == "branch" {
		if found, err := statusGit(t.path, "rev-parse", "--abbrev-ref", "@{u}"); err == nil {
			upstream = found
		}
	}
	row.Ahead, err = statusCount(t.path, upstream+"..HEAD")
	if err != nil {
		return StatusRepo{}, fmt.Errorf("%s ahead count: %w", t.path, err)
	}
	row.Behind, err = statusCount(t.path, "HEAD.."+t.ref)
	if err != nil {
		return StatusRepo{}, fmt.Errorf("%s development-tip count: %w", t.path, err)
	}
	porcelain, err := statusGit(t.path, "status", "--porcelain")
	if err != nil {
		return StatusRepo{}, fmt.Errorf("%s working tree: %w", t.path, err)
	}
	if porcelain != "" {
		row.Changed = len(strings.Split(porcelain, "\n"))
		row.Tree = fmt.Sprintf("±%d", row.Changed)
	}
	row.Operation = catchUpOperation(t)
	if unmerged, err := statusGit(t.path, "diff", "--name-only", "--diff-filter=U"); err == nil && unmerged != "" {
		row.Conflict = true
	}
	return row, nil
}

// StatusLocalSnapshot reads worktrees and local refs without contacting a
// forge, updating refs, or writing a cache. All is an explicit workspace sweep.
func (c *Context) StatusLocalSnapshot(all bool) (StatusSnapshot, error) {
	snapshot := StatusSnapshot{Schema: 1,
		GeneratedAt:          time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		ShowCollectionColumn: all,
		Repos:                []StatusRepo{}, PRs: []StatusPRRow{}, Orphans: []StatusOrphan{}}
	if !all {
		snapshot.Collection = filepath.Base(c.Collection)
	}
	report, targets, err := c.CatchUpInventory(CatchUpOptions{All: all, DryRun: true})
	if err != nil {
		return snapshot, err
	}
	if report.ExitStatus != 0 {
		return snapshot, fmt.Errorf("cannot read %d collection(s) in status inventory", len(report.Outcomes))
	}
	for i, target := range targets {
		c.statusProgress(fmt.Sprintf("Reading worktrees %d/%d", i+1, len(targets)))
		row, err := statusLocalRepo(target)
		if err != nil {
			return snapshot, err
		}
		if row.Behind != 0 {
			snapshot.StaleCount++
		}
		snapshot.Repos = append(snapshot.Repos, row)
	}
	return snapshot, nil
}
