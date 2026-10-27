package wtc

import (
	"encoding/hex"
	"strings"

	"github.com/lcorneliussen/wtc-cli/internal/next"
)

// statusPredictBBMerge supplements Bitbucket's unknown mergeability with a
// local prediction for the exact commit pair reported by the forge. It never
// fetches missing objects or substitutes HEAD or a tracking branch for them.
func statusPredictBBMerge(detail statusPRDetail, forge, slug string, rows []StatusRepo) statusPRDetail {
	if forge != "bitbucket.org" || (detail.State != "OPEN" && detail.State != "DRAFT") ||
		(detail.Merge != "" && detail.Merge != "UNKNOWN") || !statusCommitID(detail.HeadCommit) || !statusCommitID(detail.BaseCommit) {
		return detail
	}
	for _, row := range rows {
		if row.Forge != forge || row.Slug != slug {
			continue
		}
		conflicts, ok := next.PredictMergeAt(row.Worktree, detail.HeadCommit, detail.BaseCommit)
		if !ok {
			continue
		}
		detail.MergeSource = "local"
		detail.MergeConflicts = conflicts
		switch {
		case len(conflicts) > 0:
			detail.Merge = "CONFLICTING"
		case catchUpGitOK(row.Worktree, "merge-base", "--is-ancestor", detail.BaseCommit, detail.HeadCommit):
			detail.Merge = "CLEAN"
		default:
			detail.Merge = "BEHIND"
		}
		return detail
	}
	return detail
}

func statusCommitID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && value == strings.ToLower(value)
}

func statusMergeProvenance(detail statusPRDetail) (head, base string) {
	if detail.MergeSource == "local" {
		return detail.HeadCommit, detail.BaseCommit
	}
	return "", ""
}
