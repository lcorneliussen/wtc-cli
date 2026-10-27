package wtc

import (
	"encoding/json"
	"fmt"
)

func statusParseBranchList(forge string, raw []byte, branch string) (string, error) {
	switch forge {
	case "github.com":
		var prs []struct {
			Number      int    `json:"number"`
			HeadRefName string `json:"headRefName"`
		}
		if err := json.Unmarshal(raw, &prs); err != nil {
			return "", err
		}
		for _, pr := range prs {
			if pr.Number > 0 && pr.HeadRefName == branch {
				return fmt.Sprint(pr.Number), nil
			}
		}
	case "bitbucket.org":
		var result struct {
			PullRequests []struct {
				ID     int `json:"id"`
				Source struct {
					Branch struct {
						Name string `json:"name"`
					} `json:"branch"`
				} `json:"source"`
			} `json:"pullRequests"`
		}
		if err := json.Unmarshal(raw, &result); err != nil {
			return "", err
		}
		for _, pr := range result.PullRequests {
			if pr.ID > 0 && pr.Source.Branch.Name == branch {
				return fmt.Sprint(pr.ID), nil
			}
		}
	default:
		return "", fmt.Errorf("unsupported forge")
	}
	return "", nil
}

func statusDiscoverBranch(forge, slug, branch string) (string, error) {
	if forge == "" || slug == "" || branch == "" {
		return "", nil
	}
	listed, truncated, err := statusOpenPRs(forge, slug)
	if err != nil {
		return "", err
	}
	for _, pr := range listed {
		if pr.Number != "" && pr.Branch == branch {
			return pr.Number, nil
		}
	}
	switch forge {
	case "github.com":
		if truncated || len(listed) >= statusOpenListLimit {
			return statusDiscoverGitHubBranch(slug, branch)
		}
	case "bitbucket.org":
		if truncated {
			return statusDiscoverBitbucketBranch(slug, branch)
		}
	}
	return "", nil
}

func statusDiscoverGitHubBranch(slug, branch string) (string, error) {
	raw, err := statusSingleFlight("branch", "github.com", slug, branch, func() ([]byte, error) {
		return statusJSON("gh", "pr", "list", "--repo", slug, "--head", branch, "--state", "open", "--limit", "20", "--json", "number,headRefName")
	})
	if err != nil {
		return "", err
	}
	return statusParseBranchList("github.com", raw, branch)
}

func statusDiscoverBitbucketBranch(slug, branch string) (string, error) {
	raw, err := statusSingleFlight("branch", "bitbucket.org", slug, branch, func() ([]byte, error) {
		return statusFetchBBBranch(slug, branch)
	})
	if err != nil {
		return "", err
	}
	return statusParseBBBranchList(raw, branch)
}
