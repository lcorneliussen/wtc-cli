package wtc

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Check evidence is separate from code-review judgment and required-check policy.
// Only normalized states leave the collector; forge names and prose are untrusted.
type headCheckEvidence struct {
	Head         string `json:"head"`
	Result       string `json:"result"`
	Complete     bool   `json:"complete"`
	ObservedAt   string `json:"observed_at"`
	Requiredness string `json:"requiredness"`
}

func unknownHeadChecks(head string) headCheckEvidence {
	return headCheckEvidence{Head: head, Result: "UNKNOWN", ObservedAt: time.Now().UTC().Format(time.RFC3339), Requiredness: "unknown"}
}

func validCheckHead(head string) bool {
	return len(head) == 40 && validCheckHeadPrefix(head)
}

func checkAggregate(states []string, complete bool) string {
	pending, passed := false, false
	for _, state := range states {
		switch state {
		case "FAILURE":
			return "FAILURE"
		case "SUCCESS":
			passed = true
		default:
			pending = true
		}
	}
	if !complete {
		return "UNKNOWN"
	}
	if pending {
		return "PENDING"
	}
	if passed {
		return "SUCCESS"
	}
	return "NONE"
}

func bbCheckState(state string) string {
	switch strings.ToUpper(state) {
	case "SUCCESSFUL":
		return "SUCCESS"
	case "FAILED", "ERROR", "STOPPED":
		return "FAILURE"
	default:
		return "PENDING"
	}
}

// Every page spends the shared bitbucket.org forge allowance. Partial collections
// retain observed failures but cannot claim success or absence of checks.
func fetchBBCheckPages(endpoint, user, pass string, consume func(json.RawMessage) error) bool {
	seen := map[string]bool{}
	for page := 0; endpoint != "" && page < 10; page++ {
		if seen[endpoint] || !statusSameOrigin(statusBBAPI, endpoint) {
			return false
		}
		seen[endpoint] = true
		raw, _, err := statusWithForgeBudget("bitbucket.org", nil, func() ([]byte, error) { return statusBBGET(endpoint, user, pass) })
		if err != nil {
			return false
		}
		var body struct {
			Values []json.RawMessage `json:"values"`
			Next   string            `json:"next"`
		}
		if json.Unmarshal(raw, &body) != nil || body.Values == nil {
			return false
		}
		for _, item := range body.Values {
			if consume(item) != nil {
				return false
			}
		}
		endpoint = body.Next
	}
	return endpoint == ""
}

func statusBBHeadChecks(slug, head string, fresh bool) headCheckEvidence {
	evidence := unknownHeadChecks(head)
	parts := strings.Split(slug, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || !validCheckHeadPrefix(head) {
		return evidence
	}
	user, pass := bitbucketCredentials()
	if user == "" || pass == "" {
		return evidence
	}
	if len(head) < 40 {
		resolved, err := resolveBBCheckHead(slug, head, user, pass)
		if err != nil {
			return evidence
		}
		head = resolved
		evidence = unknownHeadChecks(head)
	}
	fetch := func() ([]byte, error) {
		result := unknownHeadChecks(head)
		root := statusBBAPI + "/repositories/" + url.PathEscape(parts[0]) + "/" + url.PathEscape(parts[1])
		var states []string
		statusesComplete := fetchBBCheckPages(root+"/commit/"+head+"/statuses?pagelen=100", user, pass, func(raw json.RawMessage) error {
			var status struct {
				State string `json:"state"`
			}
			if err := json.Unmarshal(raw, &status); err != nil {
				return err
			}
			states = append(states, bbCheckState(status.State))
			return nil
		})
		query := url.Values{"target.commit.hash": {head}, "sort": {"-created_on"}, "pagelen": {"100"}}
		seen := map[string]bool{}
		pipelinesComplete := fetchBBCheckPages(root+"/pipelines/?"+query.Encode(), user, pass, func(raw json.RawMessage) error {
			var run struct {
				State struct {
					Name   string `json:"name"`
					Result struct {
						Name string `json:"name"`
					} `json:"result"`
				} `json:"state"`
				Target struct {
					Commit struct {
						Hash string `json:"hash"`
					} `json:"commit"`
					Selector struct {
						Type    string `json:"type"`
						Pattern string `json:"pattern"`
					} `json:"selector"`
				} `json:"target"`
			}
			if err := json.Unmarshal(raw, &run); err != nil {
				return err
			}
			if run.Target.Commit.Hash != head {
				return nil
			}
			selector := run.Target.Selector.Type
			if !statusHeadPipelineSelector(selector) {
				return nil
			}
			// The query sorts by creation time descending, so the first rerun wins.
			key := selector + ":" + run.Target.Selector.Pattern
			if seen[key] {
				return nil
			}
			seen[key] = true
			state := "PENDING"
			if strings.EqualFold(run.State.Name, "COMPLETED") {
				state = bbCheckState(run.State.Result.Name)
			}
			states = append(states, state)
			return nil
		})
		result.Complete = statusesComplete && pipelinesComplete
		result.Result = checkAggregate(states, result.Complete)
		return json.Marshal(result)
	}
	// The collector budgets each HTTP request itself; avoid charging an extra
	// token for its aggregate.
	if fresh {
		raw, _ := fetch()
		_ = json.Unmarshal(raw, &evidence)
		return evidence
	}
	// Status uses a head-keyed cache under the existing lock.
	unlock, err := statusLockCache(statusForgeEntryPath("checks", "bitbucket.org", slug, head))
	if err == nil {
		defer unlock()
	}
	if raw, ok := statusReadForgeEntry("checks", "bitbucket.org", slug, head); ok {
		if json.Unmarshal(raw, &evidence) == nil && evidence.Head == head {
			return evidence
		}
	}
	started := time.Now()
	raw, err := fetch()
	if err == nil {
		if json.Unmarshal(raw, &evidence) == nil {
			statusWriteForgeEntrySince("checks", "bitbucket.org", slug, head, raw, started)
		}
	}
	return evidence
}

func (e headCheckEvidence) report() string {
	conclusion := "Observed checks: " + e.Result + "."
	if e.Result == "FAILURE" {
		conclusion = "Confirmed failing check on the reviewed head."
	}
	if !e.Complete {
		conclusion += " Evidence is incomplete; unavailable queries are not green."
	}
	return fmt.Sprintf("### Forge checks\n\n%s Head `%s`; observed %s. Required-check policy is unknown. This evidence is separate from the code-review verdict.\n", conclusion, e.Head, e.ObservedAt)
}

func validCheckHeadPrefix(head string) bool {
	if len(head) < 12 || len(head) > 40 {
		return false
	}
	for _, c := range head {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// Resolve abbreviations through the immutable commit endpoint before caching
// or filtering pipelines. Never attach prefix-scoped evidence to another head.
func resolveBBCheckHead(slug, prefix, user, pass string) (string, error) {
	parts := strings.Split(slug, "/")
	raw, err := statusSingleFlight("checkhead", "bitbucket.org", slug, prefix, func() ([]byte, error) {
		endpoint := statusBBAPI + "/repositories/" + url.PathEscape(parts[0]) + "/" + url.PathEscape(parts[1]) + "/commit/" + prefix
		return statusBBGET(endpoint, user, pass)
	})
	if err != nil {
		return "", err
	}
	var commit struct {
		Hash string `json:"hash"`
	}
	if json.Unmarshal(raw, &commit) != nil || !validCheckHead(commit.Hash) || !strings.HasPrefix(commit.Hash, prefix) {
		return "", fmt.Errorf("cannot resolve exact check head")
	}
	return commit.Hash, nil
}
