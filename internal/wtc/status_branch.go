package wtc

import (
	"encoding/json"
	"fmt"
	"strings"
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

func statusBitbucketListCount(raw []byte) (int, error) {
	var result struct {
		PullRequests []json.RawMessage `json:"pullRequests"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return 0, err
	}
	return len(result.PullRequests), nil
}

// Discovery only asks for open PRs on the exact source branch. A failed call
// never becomes a cached "none" answer.
func statusDiscoverBranch(forge, slug, branch string) (string, error) {
	if forge == "" || slug == "" || branch == "" {
		return "", nil
	}
	if cached, ok := statusReadForgeEntry("branch", forge, slug, branch); ok {
		if number, err := statusParseBranchList(forge, cached, branch); err == nil {
			return number, nil
		}
	}
	var raw []byte
	var err error
	switch forge {
	case "github.com":
		raw, err = statusJSON("gh", "pr", "list", "--repo", slug, "--head", branch, "--state", "open", "--limit", "50", "--json", "number,headRefName")
	case "bitbucket.org":
		parts := strings.SplitN(slug, "/", 2)
		raw, err = statusJSON("bb", "pr", "list", "-w", parts[0], "-r", parts[1], "--state", "OPEN", "--limit", "50", "--json")
	default:
		return "", fmt.Errorf("unsupported forge")
	}
	if err != nil {
		return "", err
	}
	number, err := statusParseBranchList(forge, raw, branch)
	if err == nil && forge == "bitbucket.org" && number == "" {
		count, countErr := statusBitbucketListCount(raw)
		if countErr != nil {
			return "", countErr
		}
		if count == 50 {
			// The installed bb CLI paginates up to --limit. Ask for every
			// page only when the first page has no branch match.
			parts := strings.SplitN(slug, "/", 2)
			raw, err = statusJSON("bb", "pr", "list", "-w", parts[0], "-r", parts[1], "--state", "OPEN", "--limit", "2147483647", "--json")
			if err != nil {
				return "", err
			}
			number, err = statusParseBranchList(forge, raw, branch)
		}
	}
	if err == nil {
		statusWriteForgeEntry("branch", forge, slug, branch, raw)
	}
	return number, err
}
