package wtc

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// The built-in build providers answer the same question as a harness
// status.build.sh: what is the newest build of this branch, and which commit
// did it build. They are the default; a harness hook replaces them entirely.

const statusGHBuildQuery = `query($owner:String!,$name:String!,$ref:String!){repository(owner:$owner,name:$name){ref(qualifiedName:$ref){target{... on Commit{oid statusCheckRollup{state contexts(first:50){nodes{__typename ... on CheckRun{checkSuite{workflowRun{runNumber url}}} ... on StatusContext{targetUrl}}}}}}}}}`

func statusGHBuild(slug, branch string) (*StatusBuild, error) {
	parts := strings.SplitN(slug, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid GitHub repository slug")
	}
	raw, err := statusSingleFlight("build", "github.com", slug, branch, func() ([]byte, error) {
		return statusJSON("gh", "api", "graphql", "-f", "query="+statusGHBuildQuery,
			"-f", "owner="+parts[0], "-f", "name="+parts[1], "-f", "ref=refs/heads/"+branch)
	})
	if err != nil {
		return nil, err
	}
	return statusParseGHBuild(raw, slug, branch)
}

func statusParseGHBuild(raw []byte, slug, branch string) (*StatusBuild, error) {
	var response struct {
		Data struct {
			Repository struct {
				Ref *struct {
					Target struct {
						OID    string `json:"oid"`
						Rollup *struct {
							State    string `json:"state"`
							Contexts struct {
								Nodes []struct {
									Type       string `json:"__typename"`
									CheckSuite struct {
										WorkflowRun *struct {
											RunNumber int    `json:"runNumber"`
											URL       string `json:"url"`
										} `json:"workflowRun"`
									} `json:"checkSuite"`
									TargetURL string `json:"targetUrl"`
								} `json:"nodes"`
							} `json:"contexts"`
						} `json:"statusCheckRollup"`
					} `json:"target"`
				} `json:"ref"`
			} `json:"repository"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, fmt.Errorf("GitHub build JSON: %w", err)
	}
	ref := response.Data.Repository.Ref
	if ref == nil || ref.Target.OID == "" || ref.Target.Rollup == nil {
		// No branch, or a commit nothing reported on: no build to show.
		return nil, nil
	}
	build := &StatusBuild{Branch: branch, SHA: statusString(ref.Target.OID)}
	switch state := strings.ToUpper(ref.Target.Rollup.State); state {
	case "SUCCESS", "FAILURE", "PENDING", "ERROR", "EXPECTED":
		build.Checks = statusString(state)
	default:
		build.Checks = statusString("NONE")
	}
	link := "https://github.com/" + slug + "/commit/" + ref.Target.OID + "/checks"
	for _, node := range ref.Target.Rollup.Contexts.Nodes {
		if run := node.CheckSuite.WorkflowRun; node.Type == "CheckRun" && run != nil && run.RunNumber != 0 {
			build.Build = statusString(strconv.Itoa(run.RunNumber))
			if run.URL != "" {
				link = run.URL
			}
			break
		}
	}
	build.URL = statusString(link)
	return build, nil
}

// statusBBAPI is the Bitbucket REST root; tests point it at a local server.
var statusBBAPI = "https://api.bitbucket.org/2.0"

// bitbucketCredentials reads the API user and token from the environment
// first, then from the bb CLI's own login, so a working `bb` is enough.
func bitbucketCredentials() (string, string) {
	for _, pair := range [][2]string{{"BB_API_USER", "BB_API_PASS"}, {"BB_USERNAME", "BB_API_TOKEN"}, {"ATLASSIAN_ACCOUNT_EMAIL", "ATLASSIAN_API_TOKEN"}} {
		if user, pass := os.Getenv(pair[0]), os.Getenv(pair[1]); user != "" && pass != "" {
			return user, pass
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", ""
	}
	raw, err := os.ReadFile(filepath.Join(home, ".config", "bb", "config.json"))
	if err != nil {
		return "", ""
	}
	var config struct {
		Username string `json:"username"`
		APIToken string `json:"apiToken"`
	}
	if json.Unmarshal(raw, &config) != nil {
		return "", ""
	}
	return config.Username, config.APIToken
}

func statusBBBuild(slug, branch string) (*StatusBuild, error) {
	parts := strings.SplitN(slug, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid Bitbucket repository slug")
	}
	user, pass := bitbucketCredentials()
	if user == "" || pass == "" {
		return nil, fmt.Errorf("no Bitbucket API credentials (log in with bb, or set BB_USERNAME and BB_API_TOKEN)")
	}
	// One recent page covers every branch. A newer run for this branch is on
	// that page; an older saved lookup stays current when the branch is absent.
	raw, err := statusSingleFlight("pipelines", "bitbucket.org", slug, "recent", func() ([]byte, error) {
		return statusBBPipelines(parts[0], parts[1], "", user, pass)
	})
	if err != nil {
		return nil, err
	}
	build, err := statusParseBBBuild(raw, slug, branch)
	if err != nil {
		return nil, err
	}
	if build != nil {
		if audited := statusPreferAuditedBuild(slug, branch, build); audited != nil {
			return audited, nil
		}
		return statusConfirmFullBuild(slug, branch, build, "page", user, pass), nil
	}
	if saved, ok := statusReadForgeEntryAny("build", "bitbucket.org", slug, branch); ok {
		savedBuild, parseErr := statusParseBBBuild(saved, slug, branch)
		if parseErr != nil {
			return nil, parseErr
		}
		return statusConfirmFullBuild(slug, branch, savedBuild, "saved", user, pass), nil
	}
	raw, err = statusSingleFlight("build", "bitbucket.org", slug, branch, func() ([]byte, error) {
		return statusBBPipelines(parts[0], parts[1], branch, user, pass)
	})
	if err != nil {
		return nil, err
	}
	build, err = statusParseBBBuild(raw, slug, branch)
	if err != nil {
		return nil, err
	}
	statusMarkFullRefresh("bitbucket.org", slug, "build:"+branch)
	return build, nil
}

func statusBBPipelines(workspace, repo, branch, user, pass string) ([]byte, error) {
	query := url.Values{"sort": {"-created_on"}, "pagelen": {"30"}}
	if branch != "" {
		query.Set("target.ref_name", branch)
	}
	endpoint := statusBBAPI + "/repositories/" + url.PathEscape(workspace) + "/" + url.PathEscape(repo) + "/pipelines/?" + query.Encode()
	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.SetBasicAuth(user, pass)
	request.Header.Set("Accept", "application/json")
	response, err := (&http.Client{Timeout: 15 * time.Second}).Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode == http.StatusNotFound {
		// Pipelines are not enabled. Save an empty page so the next refresh
		// does not spend another request on the same answer.
		return []byte(`{"values":[]}`), nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body := strings.TrimSpace(string(raw))
		if len(body) > 200 {
			body = body[:200]
		}
		return nil, fmt.Errorf("Bitbucket pipelines API returned %s: %s", response.Status, body)
	}
	return raw, nil
}

func statusParseBBBuild(raw []byte, slug, branch string) (*StatusBuild, error) {
	var page struct {
		Values []struct {
			BuildNumber int `json:"build_number"`
			State       struct {
				Name   string `json:"name"`
				Result struct {
					Name string `json:"name"`
				} `json:"result"`
			} `json:"state"`
			Target struct {
				RefType  string `json:"ref_type"`
				RefName  string `json:"ref_name"`
				Selector struct {
					Type string `json:"type"`
				} `json:"selector"`
				Commit struct {
					Hash string `json:"hash"`
				} `json:"commit"`
			} `json:"target"`
		} `json:"values"`
	}
	if err := json.Unmarshal(raw, &page); err != nil {
		return nil, fmt.Errorf("Bitbucket pipelines JSON: %w", err)
	}
	for _, run := range page.Values {
		// Custom and pull-request pipelines also carry the branch name; only a
		// branch pipeline describes the branch itself.
		if run.Target.RefName != branch || (run.Target.Selector.Type != "" && run.Target.Selector.Type != "branches" && run.Target.Selector.Type != "default") {
			continue
		}
		build := &StatusBuild{Branch: branch, Build: statusString(strconv.Itoa(run.BuildNumber)),
			SHA: statusString(run.Target.Commit.Hash),
			URL: statusString(fmt.Sprintf("https://bitbucket.org/%s/pipelines/results/%d", slug, run.BuildNumber))}
		checks := "PENDING"
		if strings.EqualFold(run.State.Name, "COMPLETED") {
			switch strings.ToUpper(run.State.Result.Name) {
			case "SUCCESSFUL":
				checks = "SUCCESS"
			case "FAILED", "ERROR", "STOPPED":
				checks = "FAILURE"
			}
		}
		build.Checks = statusString(checks)
		return build, nil
	}
	return nil, nil
}

func statusForgeBuild(forge, slug, branch string) (*StatusBuild, error) {
	switch forge {
	case "github.com":
		return statusGHBuild(slug, branch)
	case "bitbucket.org":
		return statusBBBuild(slug, branch)
	}
	return nil, nil
}
