package wtc

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

func statusBBMergeEventTime(raw []byte) string {
	var log struct {
		Activities []struct {
			Update struct {
				State string `json:"state"`
				Date  string `json:"date"`
			} `json:"update"`
		} `json:"activities"`
	}
	if json.Unmarshal(raw, &log) != nil {
		return ""
	}
	for _, activity := range log.Activities {
		if !strings.EqualFold(activity.Update.State, "MERGED") {
			continue
		}
		if when, err := time.Parse(time.RFC3339Nano, activity.Update.Date); err == nil {
			return when.UTC().Format(time.RFC3339)
		}
	}
	return ""
}

func statusBBCompleteMergeTime(detail statusPRDetail, slug string) statusPRDetail {
	if detail.State != "MERGED" || detail.MergedOn != "" {
		return detail
	}
	parts := strings.SplitN(slug, "/", 2)
	if len(parts) != 2 {
		return detail
	}
	raw, err := statusJSON("bb", "pr", "activity", detail.Number, "--workspace", parts[0], "--repo", parts[1], "--json")
	if err == nil {
		detail.MergedOn = statusBBMergeEventTime(raw)
	}
	return detail
}

func statusString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func prURLForStatus(forge, slug, number string) string {
	if forge == "" || slug == "" {
		return ""
	}
	return prURL("https://"+forge+"/"+slug, number)
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
				remote, err := statusGit(row.Worktree, "remote", "get-url", "origin")
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
			if forge == "bitbucket.org" {
				detail = statusBBCompleteMergeTime(detail, slug)
			}
			return detail
		}
	}
	// Reuse merged forge detail until its final facts are written to the
	// collection registry; the registry then avoids later forge calls.
	if cached, ok := statusReadMergedForgeCache(forge, slug, record.Number); ok {
		if detail, err := statusParseForgeDetail(forge, cached, record); err == nil && detail.State == "MERGED" && detail.MergedOn != "" && !detail.ChecksUnsettled && detail.Checks != "PENDING" {
			return detail
		}
	}
	var raw []byte
	var err error
	switch forge {
	case "github.com":
		raw, err = statusJSON("gh", "pr", "view", record.Number, "--repo", slug, "--json", "number,state,title,isDraft,statusCheckRollup,reviewDecision,mergeStateStatus,reviewRequests,latestReviews,mergedAt")
	case "bitbucket.org":
		parts := strings.SplitN(slug, "/", 2)
		raw, err = statusJSON("bb", "pr", "view", record.Number, "--workspace", parts[0], "--repo", parts[1], "--json")
	}
	if err == nil {
		if detail, parseErr := statusParseForgeDetail(forge, raw, record); parseErr == nil {
			statusWriteForgeCache(forge, slug, record.Number, raw)
			if forge == "bitbucket.org" {
				detail = statusBBCompleteMergeTime(detail, slug)
			}
			return detail
		}
	}
	return statusUnknownDetail(record)
}

// Forge CLIs spend most of their time waiting on network responses. Bound the
// fan-out so a large enlistment refreshes promptly without flooding a forge.
func (c *Context) statusEnrichRecords(records []PRRecord, rows []StatusRepo) ([]statusPRDetail, error) {
	details := make([]statusPRDetail, len(records))
	pending := make([]int, 0, len(records))
	recorded := 0
	for _, record := range records {
		if err := ValidatePRIdentity(record.Repo, record.Number); err != nil {
			return nil, fmt.Errorf("invalid enlisted PR: %w", err)
		}
	}
	for i, record := range records {
		if record.MergedOn == "" {
			pending = append(pending, i)
			continue
		}
		checks := record.FinalChecks
		if checks == "" {
			checks = "NONE"
		}
		details[i] = statusPRDetail{Number: record.Number, State: "MERGED", Checks: checks,
			Merge: "MERGED", Review: "merged", Title: record.Title, MergedOn: record.MergedOn}
		recorded++
	}
	if recorded != 0 {
		c.statusProgress(fmt.Sprintf("Using %d recorded merges", recorded))
	}
	if len(pending) != 0 {
		c.statusProgress(fmt.Sprintf("Checking %d live pull requests", len(pending)))
	}
	const parallel = 4
	limit := make(chan struct{}, parallel)
	done := make(chan struct{}, len(pending))
	var workers sync.WaitGroup
	for _, i := range pending {
		record := records[i]
		workers.Add(1)
		go func() {
			defer workers.Done()
			limit <- struct{}{}
			defer func() { <-limit }()
			slug, forge := c.statusRecordForge(record, rows)
			details[i] = statusEnrichRecord(record, slug, forge)
			done <- struct{}{}
		}()
	}
	for i := range pending {
		<-done
		c.statusProgress(fmt.Sprintf("Checked live pull requests %d/%d", i+1, len(pending)))
	}
	workers.Wait()
	return details, nil
}

// StatusForgePreview adds enlisted PRs and discovers open PRs on active
// branches. Build facts, snapshot persistence, and the live renderer are
// supplied by later stages. A forge failure never becomes a merged claim.
func (c *Context) StatusForgePreview() (StatusSnapshot, error) {
	return c.statusForgePreview(true, true)
}

func (c *Context) statusForgePreview(includeBuild, recordMerges bool) (StatusSnapshot, error) {
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
	details, err := c.statusEnrichRecords(records, snapshot.Repos)
	if err != nil {
		return snapshot, err
	}
	if recordMerges {
		if count, err := c.recordMergedPRs(records, details); err != nil {
			c.statusProgress(fmt.Sprintf("Could not record merged pull requests: %v", err))
		} else if count != 0 {
			c.statusProgress(fmt.Sprintf("Recorded %d final merges", count))
		}
	}
	forgeByRepo := map[string]struct{ slug, forge string }{}
	for i, record := range records {
		origin, ok := forgeByRepo[record.Repo]
		if !ok {
			origin.slug, origin.forge = c.statusRecordForge(record, snapshot.Repos)
			forgeByRepo[record.Repo] = origin
		}
		slug, forge := origin.slug, origin.forge
		detail := details[i]
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
				prURL := record.URL
				if prURL == "" && forge != "" {
					prURL = prURLForStatus(forge, slug, record.Number)
				}
				snapshot.Repos[branchRow].PR = &StatusPRFacts{Number: detail.Number, Checks: detail.Checks,
					URL: prURL, Merge: detail.Merge, Review: detail.Review, Draft: detail.State == "DRAFT"}
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
		c.statusProgress(fmt.Sprintf("Finding branch pull requests %d/%d", i+1, len(snapshot.Repos)))
		number, err := statusDiscoverBranch(forge, slug, row.Branch)
		if err != nil || number == "" {
			continue
		}
		detail := statusEnrichRecord(PRRecord{Repo: row.Repo, Number: number}, slug, forge)
		if detail.State == "OPEN" || detail.State == "DRAFT" {
			snapshot.Repos[i].PR = &StatusPRFacts{Number: detail.Number, Checks: detail.Checks,
				URL: prURLForStatus(forge, slug, number), Merge: detail.Merge, Review: detail.Review, Draft: detail.State == "DRAFT"}
		}
	}
	if includeBuild {
		c.statusProgress("Checking build providers")
		if err := c.statusBuildFacts(&snapshot); err != nil {
			return snapshot, err
		}
	}
	return snapshot, nil
}
