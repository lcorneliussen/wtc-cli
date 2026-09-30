package wtc

import (
	"fmt"
	"os"
	"strings"
	"time"
)

func statusString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func (c *Context) statusRecordForge(record PRRecord, rows []StatusRepo) (slug, forge string) {
	harnessName, _ := c.HarnessRepoName()
	for _, repo := range c.Registry.Repos {
		if repo.Name == record.Repo || (record.Repo == "harness" && repo.Name == harnessName) {
			if slug, forge = catchUpForge(repo.Remote); forge != "" {
				return slug, forge
			}
		}
	}
	for _, row := range rows {
		if row.Repo == record.Repo || (record.Repo == "harness" && row.Dir == "harness") {
			if row.Slug != "" {
				remote, err := catchUpGit(row.Worktree, "remote", "get-url", "origin")
				if err == nil {
					return catchUpForge(remote)
				}
			}
		}
	}
	return "", ""
}

func statusUnknownDetail(record PRRecord) statusPRDetail {
	return statusPRDetail{Number: record.Number, State: "UNKNOWN", Checks: "NONE", Merge: "UNKNOWN", Review: "none", Title: record.Title}
}

func statusParseForgeDetail(forge string, raw []byte, record PRRecord) (statusPRDetail, error) {
	var detail statusPRDetail
	var err error
	switch forge {
	case "github.com":
		detail, err = statusGHDetail(raw, record)
	case "bitbucket.org":
		detail, err = statusBBDetail(raw, record)
	default:
		return statusPRDetail{}, fmt.Errorf("unsupported forge")
	}
	if err == nil && detail.Number != record.Number {
		err = fmt.Errorf("forge returned a different PR number")
	}
	return detail, err
}

func statusEnrichRecord(record PRRecord, slug, forge string) statusPRDetail {
	if slug == "" || forge == "" {
		return statusUnknownDetail(record)
	}
	if cached, ok := statusReadForgeCache(forge, slug, record.Number); ok {
		if detail, err := statusParseForgeDetail(forge, cached, record); err == nil {
			return detail
		}
	}
	var raw []byte
	var err error
	switch forge {
	case "github.com":
		raw, err = catchUpJSON("gh", "pr", "view", record.Number, "--repo", slug, "--json", "number,state,title,isDraft,statusCheckRollup,reviewDecision,mergeStateStatus,reviewRequests,latestReviews,mergedAt,updatedAt")
	case "bitbucket.org":
		parts := strings.SplitN(slug, "/", 2)
		raw, err = catchUpJSON("bb", "pr", "view", record.Number, "--workspace", parts[0], "--repo", parts[1], "--json")
	}
	if err == nil {
		if detail, parseErr := statusParseForgeDetail(forge, raw, record); parseErr == nil {
			statusWriteForgeCache(forge, slug, record.Number, raw)
			return detail
		}
	}
	return statusUnknownDetail(record)
}

// StatusForgePreview adds enlisted PRs and discovers open PRs on active
// branches. Build facts, snapshot persistence, and the live renderer are
// supplied by later stages. A forge failure never becomes a merged claim.
func (c *Context) StatusForgePreview() (StatusSnapshot, error) {
	snapshot, err := c.StatusLocalSnapshot(false)
	if err != nil {
		return snapshot, err
	}
	records, err := c.ListPRs()
	if err != nil {
		return snapshot, err
	}
	if len(records) == 0 {
		_, err := os.Stat(c.PRFile())
		snapshot.PRsEmptyHint = os.IsNotExist(err)
		if err != nil && !os.IsNotExist(err) {
			return snapshot, err
		}
	}
	enlistedBranches := map[string]bool{}
	for _, record := range records {
		if err := ValidatePRIdentity(record.Repo, record.Number); err != nil {
			return snapshot, fmt.Errorf("invalid enlisted PR: %w", err)
		}
		slug, forge := c.statusRecordForge(record, snapshot.Repos)
		detail := statusEnrichRecord(record, slug, forge)
		worktreeDir := record.Repo
		harnessName, _ := c.HarnessRepoName()
		if record.Repo == "harness" || record.Repo == harnessName {
			worktreeDir = "harness"
		}
		if record.Branch != "" {
			enlistedBranches[worktreeDir+"\x00"+record.Branch] = true
		}
		branchRow := -1
		for i, row := range snapshot.Repos {
			if row.Dir == worktreeDir && row.BranchKind == "branch" && row.Branch == record.Branch {
				branchRow = i
				break
			}
		}
		if detail.State == "OPEN" || detail.State == "DRAFT" {
			if branchRow >= 0 && snapshot.Repos[branchRow].PR == nil {
				snapshot.Repos[branchRow].PR = &StatusPRFacts{Number: detail.Number, Checks: detail.Checks,
					Merge: detail.Merge, Review: detail.Review, Draft: detail.State == "DRAFT"}
			}
		}
		if detail.State == "CLOSED" || detail.State == "DECLINED" || detail.State == "SUPERSEDED" {
			if branchRow >= 0 {
				snapshot.Orphans = append(snapshot.Orphans, StatusOrphan{Repo: worktreeDir, Branch: record.Branch, State: detail.State})
			}
			continue
		}
		url := record.URL
		if url == "" && forge != "" {
			url = prURL("https://"+forge+"/"+slug, record.Number)
		}
		row := StatusPRRow{Repo: record.Repo, Number: detail.Number, Checks: statusString(detail.Checks),
			Merge: statusString(detail.Merge), Review: statusString(detail.Review), Title: detail.Title,
			DisplayTitle: detail.Title, Slug: slug, URL: statusString(url), MergedOn: statusString(detail.MergedOn),
			Draft: detail.State == "DRAFT", OnBranch: detail.State == "MERGED" && branchRow >= 0,
			Archived: detail.State == "MERGED" && branchRow < 0 && statusArchived(detail.MergedOn, time.Now()),
		}
		if row.OnBranch {
			row.DisplayTitle = "MERGED — still on " + record.Branch + "; catch-up  " + detail.Title
		}
		snapshot.PRs = append(snapshot.PRs, row)
	}
	for i, row := range snapshot.Repos {
		if row.BranchKind != "branch" || row.PR != nil || enlistedBranches[row.Dir+"\x00"+row.Branch] {
			continue
		}
		slug, forge := c.statusRecordForge(PRRecord{Repo: row.Repo}, snapshot.Repos)
		number, err := statusDiscoverBranch(forge, slug, row.Branch)
		if err != nil || number == "" {
			continue
		}
		detail := statusEnrichRecord(PRRecord{Repo: row.Repo, Number: number}, slug, forge)
		if detail.State == "OPEN" || detail.State == "DRAFT" {
			snapshot.Repos[i].PR = &StatusPRFacts{Number: detail.Number, Checks: detail.Checks,
				Merge: detail.Merge, Review: detail.Review, Draft: detail.State == "DRAFT"}
		}
	}
	return snapshot, nil
}
