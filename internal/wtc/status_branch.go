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
	var raw []byte
	var err error
	switch forge {
	case "github.com":
		raw, err = statusSingleFlight("branch", forge, slug, branch, func() ([]byte, error) {
			return statusJSON("gh", "pr", "list", "--repo", slug, "--head", branch, "--state", "open", "--limit", "50", "--json", "number,headRefName")
		})
	case "bitbucket.org":
		parts := strings.SplitN(slug, "/", 2)
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid Bitbucket repository slug")
		}
		raw, err = statusSingleFlight("bblist", forge, slug, "OPEN", func() ([]byte, error) {
			return statusJSON("bb", "pr", "list", "-w", parts[0], "-r", parts[1], "--state", "OPEN", "--limit", "50", "--json")
		})
		if err != nil {
			return "", err
		}
		number, parseErr := statusParseBranchList(forge, raw, branch)
		if parseErr != nil || number != "" {
			return number, parseErr
		}
		count, countErr := statusBitbucketListCount(raw)
		if countErr != nil {
			return "", countErr
		}
		if count < 50 {
			return "", nil
		}
		// The installed bb CLI paginates up to --limit. Ask for every page
		// only when the first page has no branch match. One saved list serves
		// every branch of the repository.
		raw, err = statusSingleFlight("bblist", forge, slug, "OPEN-ALL", func() ([]byte, error) {
			return statusJSON("bb", "pr", "list", "-w", parts[0], "-r", parts[1], "--state", "OPEN", "--limit", "2147483647", "--json")
		})
	default:
		return "", fmt.Errorf("unsupported forge")
	}
	if err != nil {
		return "", err
	}
	return statusParseBranchList(forge, raw, branch)
}
