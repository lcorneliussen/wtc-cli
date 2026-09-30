package wtc

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type statusPRDetail struct {
	Number   string
	State    string
	Checks   string
	Merge    string
	Review   string
	Title    string
	MergedOn string
}

func statusCheckResult(rollup []struct {
	Conclusion string `json:"conclusion"`
	State      string `json:"state"`
	Status     string `json:"status"`
}) string {
	failed, pending, passed := false, false, false
	for _, check := range rollup {
		conclusion := strings.ToUpper(check.Conclusion)
		if conclusion == "" {
			conclusion = strings.ToUpper(check.State)
		}
		switch conclusion {
		case "FAILURE", "TIMED_OUT", "CANCELLED", "ACTION_REQUIRED", "STARTUP_FAILURE", "ERROR":
			failed = true
		case "SUCCESS", "NEUTRAL", "SKIPPED":
			passed = true
		}
		if check.Status != "" && !strings.EqualFold(check.Status, "COMPLETED") {
			pending = true
		}
	}
	switch {
	case failed:
		return "FAILURE"
	case pending:
		return "PENDING"
	case passed:
		return "SUCCESS"
	default:
		return "NONE"
	}
}

func statusGHDetail(raw []byte, fallback PRRecord) (statusPRDetail, error) {
	var p struct {
		Number            int               `json:"number"`
		State             string            `json:"state"`
		Title             string            `json:"title"`
		IsDraft           bool              `json:"isDraft"`
		ReviewDecision    string            `json:"reviewDecision"`
		MergeStateStatus  string            `json:"mergeStateStatus"`
		MergedAt          string            `json:"mergedAt"`
		UpdatedAt         string            `json:"updatedAt"`
		ReviewRequests    []json.RawMessage `json:"reviewRequests"`
		LatestReviews     []json.RawMessage `json:"latestReviews"`
		StatusCheckRollup []struct {
			Conclusion string `json:"conclusion"`
			State      string `json:"state"`
			Status     string `json:"status"`
		} `json:"statusCheckRollup"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return statusPRDetail{}, err
	}
	if p.Number == 0 || p.State == "" {
		return statusPRDetail{}, fmt.Errorf("missing GitHub PR identity or state")
	}
	d := statusPRDetail{Number: fmt.Sprint(p.Number), State: strings.ToUpper(p.State),
		Checks: statusCheckResult(p.StatusCheckRollup), Merge: p.MergeStateStatus,
		Title: cleanPRField(p.Title), MergedOn: p.MergedAt}
	if d.Title == "" {
		d.Title = fallback.Title
	}
	if d.State == "OPEN" && p.IsDraft {
		d.State = "DRAFT"
	}
	if d.State == "MERGED" {
		d.Merge = "MERGED"
		d.Review = "merged"
		if d.MergedOn == "" {
			d.MergedOn = p.UpdatedAt
		}
	} else {
		switch strings.ToUpper(p.ReviewDecision) {
		case "APPROVED":
			d.Review = "approved"
		case "CHANGES_REQUESTED":
			d.Review = "changes"
		default:
			switch {
			case p.IsDraft && len(p.ReviewRequests) == 0 && len(p.LatestReviews) == 0:
				d.Review = "none"
			case len(p.LatestReviews) != 0:
				d.Review = "commented"
			case len(p.ReviewRequests) != 0 || strings.EqualFold(p.ReviewDecision, "REVIEW_REQUIRED"):
				d.Review = "waiting"
			case p.IsDraft:
				d.Review = "none"
			default:
				d.Review = "noreviewers"
			}
		}
	}
	if p.IsDraft && d.Checks == "NONE" {
		d.Checks = "draft"
	}
	return d, nil
}

func statusBBDetail(raw []byte, fallback PRRecord) (statusPRDetail, error) {
	var p struct {
		ID           int    `json:"id"`
		State        string `json:"state"`
		Draft        bool   `json:"draft"`
		Title        string `json:"title"`
		UpdatedOn    string `json:"updated_on"`
		Participants []struct {
			Approved bool   `json:"approved"`
			State    string `json:"state"`
		} `json:"participants"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return statusPRDetail{}, err
	}
	if p.ID == 0 || p.State == "" {
		return statusPRDetail{}, fmt.Errorf("missing Bitbucket PR identity or state")
	}
	d := statusPRDetail{Number: fmt.Sprint(p.ID), State: strings.ToUpper(p.State),
		Checks: "NONE", Merge: "UNKNOWN", Review: "none", Title: cleanPRField(p.Title)}
	if d.Title == "" {
		d.Title = fallback.Title
	}
	if p.Draft && d.State == "OPEN" {
		d.State = "DRAFT"
		d.Checks = "draft"
	}
	for _, participant := range p.Participants {
		if participant.Approved {
			d.Review = "approved"
			break
		}
		if strings.EqualFold(participant.State, "changes_requested") {
			d.Review = "changes"
		}
	}
	if d.State == "MERGED" {
		d.Merge = "MERGED"
		d.Review = "merged"
		d.MergedOn = p.UpdatedOn
	}
	return d, nil
}

func statusWeekdayHours(start, end time.Time) float64 {
	if !end.After(start) {
		return 0
	}
	var hours float64
	for at := start.UTC(); at.Before(end); {
		next := time.Date(at.Year(), at.Month(), at.Day()+1, 0, 0, 0, 0, time.UTC)
		if next.After(end) {
			next = end
		}
		if at.Weekday() != time.Saturday && at.Weekday() != time.Sunday {
			hours += next.Sub(at).Hours()
		}
		at = next
	}
	return hours
}

func statusArchived(mergedOn string, now time.Time) bool {
	when, err := time.Parse(time.RFC3339, mergedOn)
	hours := 48.0
	if configured, parseErr := strconv.ParseFloat(os.Getenv("WTC_PR_ARCHIVE_HOURS"), 64); parseErr == nil && configured > 0 {
		hours = configured
	}
	return err == nil && statusWeekdayHours(when, now) >= hours
}
