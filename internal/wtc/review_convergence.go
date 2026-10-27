package wtc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const reviewScopeGrowthThreshold = 50

// reviewConvergence records observations without replacing the current-head gate. Missing older artifacts leave comparisons unknown.
type reviewConvergence struct {
	Round                 int      `json:"round"`
	PatchLines            *int     `json:"patch_lines,omitempty"`
	PreviousRound         int      `json:"previous_round,omitempty"`
	PreviousPatchLines    *int     `json:"previous_patch_lines,omitempty"`
	GrowthPercent         *float64 `json:"growth_percent,omitempty"`
	GrowthThreshold       int      `json:"growth_threshold_percent"`
	ScopeGrew             bool     `json:"scope_grew"`
	PreviousMajorConcerns []string `json:"previous_major_concerns,omitempty"`
	DesignCheckpoints     []string `json:"design_checkpoint_concerns,omitempty"`
}

func reviewPatchLines(dir string) (*int, error) {
	data, err := os.ReadFile(filepath.Join(dir, "diff.patch"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	n := bytes.Count(data, []byte{'\n'})
	if len(data) > 0 && data[len(data)-1] != '\n' {
		n++
	}
	return &n, nil
}

// prepareReviewConvergence compares only the audience-filtered completed
// round selected by copyReviewPrior. Patch size is a signal, not proof of new
// behavior: moving the comparison base or changing generated files can grow it.
func prepareReviewConvergence(dir string, current ReviewManifest, previous string) error {
	lines, err := reviewPatchLines(dir)
	if err != nil {
		return err
	}
	context := reviewConvergence{Round: current.Round, PatchLines: lines, GrowthThreshold: reviewScopeGrowthThreshold}
	if previous != "" {
		m, _, _ := readPriorReviewManifest(previous)
		context.PreviousRound = m.Round
		context.PreviousPatchLines, err = reviewPatchLines(previous)
		if err != nil {
			return err
		}
		context.PreviousMajorConcerns = reviewOpenMajorConcerns(previous)
		if lines != nil && context.PreviousPatchLines != nil {
			before, after := *context.PreviousPatchLines, *lines
			if before > 0 {
				growth := float64(after-before) * 100 / float64(before)
				context.GrowthPercent = &growth
				context.ScopeGrew = growth > reviewScopeGrowthThreshold
			} else {
				context.ScopeGrew = after > 0
			}
		}
	}
	return writeReviewConvergence(dir, context)
}

var reviewContextConcernID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

func reviewOpenMajorConcerns(dir string) []string {
	paths, _ := filepath.Glob(filepath.Join(dir, "findings", "*.json"))
	var concerns []string
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var file reviewFindingFile
		if json.Unmarshal(data, &file) != nil || (file.Status != "issues" && file.Status != "ok") ||
			file.Concern != strings.TrimSuffix(filepath.Base(path), ".json") || !reviewContextConcernID.MatchString(file.Concern) {
			continue
		}
		for _, finding := range file.Findings {
			if finding.Prior != "addressed" && (finding.Severity == "major" || finding.Severity == "blocker") {
				concerns = append(concerns, file.Concern)
				break
			}
		}
	}
	sort.Strings(concerns)
	return concerns
}

// refreshReviewConvergence runs before and after lead dispositions, so a
// refuted major does not leave a spurious design checkpoint in the report.
func refreshReviewConvergence(dir string, manifest ReviewManifest) (reviewConvergence, error) {
	var context reviewConvergence
	data, err := os.ReadFile(filepath.Join(dir, "review-context.json"))
	if os.IsNotExist(err) {
		if err := prepareReviewConvergence(dir, manifest, ""); err != nil {
			return context, err
		}
		data, err = os.ReadFile(filepath.Join(dir, "review-context.json"))
	}
	if err != nil {
		return context, err
	}
	if err := json.Unmarshal(data, &context); err != nil {
		return context, err
	}
	context.Round = manifest.Round
	context.DesignCheckpoints = nil
	if manifest.Round >= 3 {
		for _, current := range reviewOpenMajorConcerns(dir) {
			for _, previous := range context.PreviousMajorConcerns {
				if current == previous {
					context.DesignCheckpoints = append(context.DesignCheckpoints, current)
					break
				}
			}
		}
	}
	return context, writeReviewConvergence(dir, context)
}

func writeReviewConvergence(dir string, context reviewConvergence) error {
	data, err := json.MarshalIndent(context, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "review-context.json"), append(data, '\n'), 0644)
}

func reviewConvergenceReport(context reviewConvergence) string {
	var notes []string
	if context.ScopeGrew && context.PatchLines != nil && context.PreviousPatchLines != nil {
		notes = append(notes, fmt.Sprintf("Scope checkpoint: the patch grew from %d to %d lines since round %d (more than %d%%). Confirm whether the PR description still covers it; split additional behavior or explicitly restart the review scope when needed.", *context.PreviousPatchLines, *context.PatchLines, context.PreviousRound, context.GrowthThreshold))
	}
	if len(context.DesignCheckpoints) > 0 {
		notes = append(notes, fmt.Sprintf("Design checkpoint: major findings remain in %s at round %d after the preceding completed round also reported majors. Does the design converge around a shared invariant, or should the work be split? Answer before starting another round of point fixes.", strings.Join(context.DesignCheckpoints, ", "), context.Round))
	}
	if len(notes) == 0 {
		return ""
	}
	return "### Review checkpoints\n\n" + strings.Join(notes, "\n\n")
}
