package wtc

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type catchUpPR struct {
	state, mergeCommit, head, base string
}

func catchUpGit(path string, args ...string) (string, error) {
	return gitOutput(append([]string{"-C", path}, args...)...)
}

func catchUpGitOK(path string, args ...string) bool {
	_, err := catchUpGit(path, args...)
	return err == nil
}

func (c *Context) catchUpDefaultRef(t catchUpTarget) (string, error) {
	target, err := OpenCollection(filepath.Join(c.Workspace, t.collection))
	if err != nil {
		return "", err
	}
	repo, err := target.Repository(t.repo)
	if err != nil || repo.DefaultRef == "" {
		return "origin/main", nil
	}
	return repo.DefaultRef, nil
}

func catchUpJSON(args ...string) ([]byte, error) {
	cmd := exec.Command(args[0], args[1:]...)
	return cmd.Output()
}

func catchUpForge(remote string) (string, string) {
	remote = normalizeCatchUpRemote(remote)
	for _, host := range []string{"github.com", "bitbucket.org"} {
		if strings.HasPrefix(remote, host+"/") {
			slug := strings.TrimPrefix(remote, host+"/")
			if strings.Count(slug, "/") == 1 {
				return slug, host
			}
		}
	}
	return "", ""
}

func catchUpValidPRState(state string) bool {
	switch state {
	case "OPEN", "DRAFT", "MERGED", "CLOSED", "DECLINED", "SUPERSEDED":
		return true
	}
	return false
}

func catchUpRefForPR(worktree, defaultRef string, pr catchUpPR) (string, error) {
	if pr.state != "OPEN" && pr.state != "DRAFT" {
		return defaultRef, nil
	}
	if pr.base == "" || !catchUpGitOK(worktree, "check-ref-format", "--branch", pr.base) {
		return "", fmt.Errorf("open PR merge target unavailable or invalid")
	}
	return "origin/" + pr.base, nil
}

func (c *Context) catchUpPRState(t catchUpTarget, branch string) catchUpPR {
	target, err := OpenCollection(filepath.Join(c.Workspace, t.collection))
	if err != nil {
		return catchUpPR{state: "UNKNOWN"}
	}
	records, err := target.ListPRs()
	if err != nil {
		return catchUpPR{state: "UNKNOWN"}
	}
	selected := []PRRecord{}
	for _, record := range records {
		if (record.Repo == t.repo || t.harness && record.Repo == "harness") && record.Branch == branch {
			selected = append(selected, record)
		}
	}
	remote, err := catchUpGit(t.path, "remote", "get-url", "origin")
	if err != nil {
		return catchUpPR{state: "UNKNOWN"}
	}
	slug, forge := catchUpForge(remote)
	if forge == "" {
		return catchUpPR{state: "UNKNOWN"}
	}
	if len(selected) == 0 {
		if forge != "github.com" {
			return catchUpPR{state: "UNKNOWN"} // No bounded branch lookup on this forge.
		}
		out, err := catchUpJSON("gh", "pr", "list", "--repo", slug, "--head", branch, "--state", "all", "--json", "state,isDraft,baseRefName")
		if err != nil {
			return catchUpPR{state: "UNKNOWN"}
		}
		var rows []struct {
			State       string `json:"state"`
			IsDraft     bool   `json:"isDraft"`
			BaseRefName string `json:"baseRefName"`
		}
		if json.Unmarshal(out, &rows) != nil {
			return catchUpPR{state: "UNKNOWN"}
		}
		if len(rows) == 0 {
			return catchUpPR{state: "NONE"}
		}
		chosen := catchUpPR{state: "NONE"}
		for _, row := range rows {
			state := row.State
			if !catchUpValidPRState(state) {
				return catchUpPR{state: "UNKNOWN"}
			}
			if row.IsDraft && state == "OPEN" {
				state = "DRAFT"
			}
			if state == "OPEN" || state == "DRAFT" {
				if chosen.state == "OPEN" || chosen.state == "DRAFT" {
					if chosen.base != row.BaseRefName {
						return catchUpPR{state: "UNKNOWN"}
					}
				} else {
					chosen = catchUpPR{state: state, base: row.BaseRefName}
				}
			} else if chosen.state == "NONE" {
				chosen = catchUpPR{state: state}
			}
		}
		return chosen
	}
	chosen := catchUpPR{state: "UNKNOWN"}
	liveState := ""
	liveBase := ""
	for _, record := range selected {
		if forge == "bitbucket.org" {
			parts := strings.SplitN(slug, "/", 2)
			out, err := catchUpJSON("bb", "pr", "view", record.Number, "--workspace", parts[0], "--repo", parts[1], "--json")
			if err != nil {
				return catchUpPR{state: "UNKNOWN"}
			}
			var facts struct {
				State string `json:"state"`
				Draft bool   `json:"draft"`
				Merge struct {
					Hash string `json:"hash"`
				} `json:"merge_commit"`
				Source struct {
					Commit struct {
						Hash string `json:"hash"`
					} `json:"commit"`
				} `json:"source"`
				Destination struct {
					Branch struct {
						Name string `json:"name"`
					} `json:"branch"`
				} `json:"destination"`
			}
			if json.Unmarshal(out, &facts) != nil {
				return catchUpPR{state: "UNKNOWN"}
			}
			state := strings.ToUpper(facts.State)
			if facts.Draft && state == "OPEN" {
				state = "DRAFT"
			}
			if !catchUpValidPRState(state) {
				return catchUpPR{state: "UNKNOWN"}
			}
			if state == "OPEN" || state == "DRAFT" {
				base := facts.Destination.Branch.Name
				if liveState != "" && liveBase != base {
					return catchUpPR{state: "UNKNOWN"}
				}
				liveBase = base
				if state == "DRAFT" || liveState == "" {
					liveState = state
				}
				continue
			}
			if chosen.state == "UNKNOWN" || chosen.state == "NONE" {
				chosen = catchUpPR{state: state, head: facts.Source.Commit.Hash, mergeCommit: facts.Merge.Hash}
			}
			continue
		}
		out, err := catchUpJSON("gh", "pr", "view", record.Number, "--repo", slug, "--json", "state,isDraft,headRefOid,mergeCommit,baseRefName")
		if err != nil {
			return catchUpPR{state: "UNKNOWN"}
		}
		var facts struct {
			State       string `json:"state"`
			IsDraft     bool   `json:"isDraft"`
			HeadRefOid  string `json:"headRefOid"`
			BaseRefName string `json:"baseRefName"`
			Merge       struct {
				OID string `json:"oid"`
			} `json:"mergeCommit"`
		}
		if json.Unmarshal(out, &facts) != nil || !catchUpValidPRState(facts.State) {
			return catchUpPR{state: "UNKNOWN"}
		}
		state := facts.State
		if facts.IsDraft && state == "OPEN" {
			state = "DRAFT"
		}
		if state == "OPEN" || state == "DRAFT" {
			if liveState != "" && liveBase != facts.BaseRefName {
				return catchUpPR{state: "UNKNOWN"}
			}
			liveBase = facts.BaseRefName
			if state == "DRAFT" || liveState == "" {
				liveState = state
			}
			continue
		}
		if chosen.state == "UNKNOWN" || chosen.state == "NONE" {
			chosen = catchUpPR{state: state, head: facts.HeadRefOid, mergeCommit: facts.Merge.OID}
		}
	}
	if liveState != "" {
		return catchUpPR{state: liveState, base: liveBase}
	}
	return chosen
}

func catchUpLanded(t catchUpTarget, target string, pr catchUpPR) bool {
	extra, err := catchUpGit(t.path, "rev-list", "--count", target+"..HEAD")
	if err != nil {
		return false
	}
	if extra == "0" {
		return true
	}
	if pr.mergeCommit != "" && pr.head != "" && catchUpGitOK(t.path, "merge-base", "--is-ancestor", pr.mergeCommit, target) {
		prHead, err := catchUpGit(t.path, "rev-parse", "--verify", pr.head+"^{commit}")
		if head, headErr := catchUpGit(t.path, "rev-parse", "HEAD"); err == nil && headErr == nil && head == prHead {
			return true
		}
	}
	cherry, err := catchUpGit(t.path, "cherry", target, "HEAD")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(cherry, "\n") {
		if strings.HasPrefix(line, "+") {
			return false
		}
	}
	return true
}

func catchUpOperation(t catchUpTarget) string {
	for _, marker := range []string{"MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD", "rebase-merge", "rebase-apply", "sequencer"} {
		path, err := catchUpGit(t.path, "rev-parse", "--git-path", marker)
		if err != nil {
			return marker
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(t.path, path)
		}
		if _, err := os.Lstat(path); err == nil {
			return marker
		}
	}
	return ""
}

func catchUpStashRef(t catchUpTarget) string {
	return "refs/wtc-catch-up/" + t.collection + "/" + t.repo
}

func catchUpRestoreStash(t catchUpTarget, stash string) (string, bool) {
	if stash == "" {
		return "", true
	}
	if !catchUpGitOK(t.path, "stash", "apply", stash) {
		return "stash restore conflicted; retained " + stash + " pinned under refs/wtc-catch-up/", false
	}
	latest, _ := catchUpGit(t.path, "rev-parse", "-q", "--verify", "refs/stash")
	if latest != stash || !catchUpGitOK(t.path, "stash", "drop", "stash@{0}") {
		return "changes restored; retained stash " + stash + " because stack changed or drop failed", false
	}
	_, _ = catchUpGit(t.path, "update-ref", "-d", catchUpStashRef(t))
	return "", true
}

func (c *Context) catchUpReconcile(t catchUpTarget, ref, target string, pr catchUpPR, opt CatchUpOptions) (string, string) {
	if marker := catchUpOperation(t); marker != "" {
		reason := "in-progress " + marker + "; untouched"
		if marker == "MERGE_HEAD" {
			if paths, err := catchUpGit(t.path, "diff", "--name-only", "--diff-filter=U"); err == nil && paths != "" {
				reason += "; unmerged: " + strings.ReplaceAll(paths, "\n", ", ")
			}
		}
		return "needs-owner", reason
	}
	status, err := catchUpGit(t.path, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return "failed", "worktree status unreadable"
	}
	if (opt.All || opt.CleanOnly) && status != "" {
		return "needs-owner", "dirty worktree; clean-only leaves it untouched"
	}
	branch, _ := catchUpGit(t.path, "symbolic-ref", "-q", "--short", "HEAD")
	if branch == "" && !catchUpGitOK(t.path, "merge-base", "--is-ancestor", "HEAD", target) {
		return "needs-owner", "detached commits not contained in default tip; preserve them on an owner branch"
	}
	switch pr.state {
	case "UNKNOWN":
		return "needs-owner", "PR state unavailable; branch left untouched"
	case "CLOSED", "DECLINED", "SUPERSEDED":
		return "needs-owner", "closed PR branch; owner must decide continuation"
	case "MERGED":
		if !catchUpLanded(t, target, pr) {
			return "needs-owner", "merged PR landing not established; owner must inspect remaining commits or unavailable Git evidence"
		}
	default:
		if branch != "" && catchUpGitOK(t.path, "merge-base", "--is-ancestor", target, "HEAD") {
			return "current", "already contains " + ref
		}
	}
	if branch == "" {
		if head, err := catchUpGit(t.path, "rev-parse", "HEAD"); err == nil && head == target {
			return "current", "already contains default tip"
		}
	}
	if opt.DryRun {
		return "planned", "would update against local " + ref + " (remote not fetched)"
	}
	stash := ""
	if status != "" {
		before, _ := catchUpGit(t.path, "rev-parse", "-q", "--verify", "refs/stash")
		if !catchUpGitOK(t.path, "stash", "push", "--include-untracked", "-m", "wtc-catch-up "+t.collection+"/"+t.repo) {
			return "needs-owner", "stash failed; update skipped"
		}
		after, _ := catchUpGit(t.path, "rev-parse", "-q", "--verify", "refs/stash")
		if after != before {
			stash = after
			if !catchUpGitOK(t.path, "update-ref", catchUpStashRef(t), stash) {
				if restoreReason, ok := catchUpRestoreStash(t, stash); !ok {
					return "needs-owner", "recovery stash pin failed; " + restoreReason
				}
				return "needs-owner", "recovery stash pin failed; update skipped"
			}
		}
	}
	outcome, reason := "updated", ""
	if branch == "" || pr.state == "MERGED" {
		if !catchUpGitOK(t.path, "checkout", "--detach", target) {
			outcome, reason = "needs-owner", "checkout refused"
		} else {
			reason = "detached at default tip"
			if branch != "" && !catchUpGitOK(t.path, "branch", "-d", branch) {
				reason += "; local branch retained because safe pruning refused"
			}
		}
	} else if branch == strings.TrimPrefix(ref, "origin/") {
		if catchUpGitOK(t.path, "merge", "--ff-only", target) {
			reason = "fast-forwarded default branch"
		} else {
			outcome, reason = "needs-owner", "default branch could not fast-forward"
		}
	} else if !catchUpGitOK(t.path, "merge", "--no-edit", target) {
		outcome, reason = "needs-owner", "merge refused before creating merge state"
		if catchUpGitOK(t.path, "rev-parse", "-q", "--verify", "MERGE_HEAD") {
			paths, _ := catchUpGit(t.path, "diff", "--name-only", "--diff-filter=U")
			if catchUpGitOK(t.path, "merge", "--abort") {
				reason = "merge conflict; aborted to original tree"
				if paths != "" {
					reason += "; paths: " + strings.ReplaceAll(paths, "\n", ", ")
				}
			} else {
				reason = "merge abort failed; owner must restore worktree"
			}
		}
	} else {
		reason = "merged " + ref + " into " + branch
		if (pr.state == "OPEN" || pr.state == "DRAFT") && !catchUpGitOK(t.path, "push") {
			outcome, reason = "needs-owner", "merge succeeded but push refused"
		}
	}
	if restoreReason, ok := catchUpRestoreStash(t, stash); !ok {
		return "needs-owner", restoreReason
	}
	return outcome, reason
}

// CatchUp performs a collected sweep. Its report is returned even when one
// target fails, so callers can emit the full result before a nonzero exit.
func (c *Context) CatchUp(opt CatchUpOptions) (CatchUpReport, error) {
	report := CatchUpReport{SchemaVersion: 1, Initiator: c.Collection, ExitStatus: 1, Outcomes: []CatchUpRow{}}
	if opt.Report == "" && !opt.DryRun {
		opt.Report = filepath.Join(c.Collection, ".wtc-catch-up.json")
	}
	var err error
	if opt.Report != "" {
		opt.Report, err = filepath.Abs(opt.Report)
		if err != nil {
			return report, err
		}
		if info, statErr := os.Stat(filepath.Dir(opt.Report)); statErr != nil || !info.IsDir() {
			return report, fmt.Errorf("report parent directory does not exist")
		}
	}
	report, targets, inventoryErr := c.CatchUpInventory(opt)
	if inventoryErr != nil {
		report.ExitStatus = 1
		if err := SaveCatchUpReport(&report, opt.Report); err != nil {
			return report, fmt.Errorf("%w; report persistence failed: %v", inventoryErr, err)
		}
		return report, inventoryErr
	}
	fetched := map[string]string{}
	for _, t := range targets {
		if _, ok := fetched[t.owner]; ok {
			continue
		}
		state := "planned"
		if !opt.DryRun {
			if _, err := gitOutput("--git-dir="+t.owner, "fetch", "--prune", "origin"); err != nil {
				state = "failed"
			} else {
				state = "ok"
			}
		}
		fetched[t.owner] = state
		report.add("fetch", t.collection, t.repo, state, t.owner, "", "", "")
	}
	for _, t := range targets {
		source, err := catchUpGit(t.path, "rev-parse", "--verify", "HEAD")
		if err != nil {
			report.add("repo", t.collection, t.repo, "failed", "worktree HEAD unreadable; target may have moved or disappeared", "", "", "")
			continue
		}
		ref, refErr := c.catchUpDefaultRef(t)
		if refErr != nil {
			report.add("repo", t.collection, t.repo, "failed", "target harness registry unavailable after update", source, "", source)
			continue
		}
		pr := catchUpPR{state: "NONE"}
		branch, _ := catchUpGit(t.path, "symbolic-ref", "-q", "--short", "HEAD")
		if branch != "" && branch != strings.TrimPrefix(ref, "origin/") && catchUpOperation(t) == "" {
			pr = c.catchUpPRState(t, branch)
		}
		ref, refErr = catchUpRefForPR(t.path, ref, pr)
		if refErr != nil {
			report.add("repo", t.collection, t.repo, "needs-owner", refErr.Error()+"; branch left untouched", source, "", source)
			continue
		}
		target, targetErr := catchUpGit(t.path, "rev-parse", "--verify", ref+"^{commit}")
		outcome, reason := "failed", ref+" unavailable"
		if fetched[t.owner] == "failed" {
			reason = "owner fetch failed; stale refs not used"
		} else if targetErr != nil && (pr.state == "OPEN" || pr.state == "DRAFT") {
			outcome, reason = "needs-owner", "open PR merge target "+ref+" is unavailable; branch left untouched"
		} else if targetErr == nil {
			outcome, reason = c.catchUpReconcile(t, ref, target, pr, opt)
		}
		result, resultErr := catchUpGit(t.path, "rev-parse", "--verify", "HEAD")
		if resultErr != nil {
			outcome, reason = "failed", "worktree HEAD unreadable after update"
		}
		report.add("repo", t.collection, t.repo, outcome, reason, source, target, result)
		row := &report.Outcomes[len(report.Outcomes)-1]
		row.TargetRef = ref
		if strings.HasPrefix(reason, "merge conflict;") {
			row.NextAction = strings.Replace(row.NextAction, "merge the target ref", "merge "+ref, 1)
		}
		if outcome == "updated" || outcome == "current" || outcome == "planned" {
			c.catchUpHooks(&report, t, opt)
		} else if t.harness && opt.ReloadStatus {
			report.add("pane", t.collection, "status", "skipped", "harness update needs attention; pane left running", "", "", "")
		}
	}
	if err := SaveCatchUpReport(&report, opt.Report); err != nil {
		return report, fmt.Errorf("report persistence failed: %w", err)
	}
	if report.ExitStatus != 0 {
		return report, fmt.Errorf("catch-up needs attention; see report")
	}
	return report, nil
}

func (c *Context) catchUpHooks(report *CatchUpReport, t catchUpTarget, opt CatchUpOptions) {
	targetDir := filepath.Join(c.Workspace, t.collection)
	// The harness updates first and may have changed registry membership, so
	// decide from the current target registry.
	managed := t.managed
	target, openErr := OpenCollection(targetDir)
	if openErr == nil {
		_, repoErr := target.Repository(t.repo)
		managed = t.harness || repoErr == nil
	}
	run := func(label string, action func(*Context) error) {
		if openErr != nil {
			report.add("hook", t.collection, label, "failed", "cannot open target collection: "+openErr.Error(), "", "", "")
			return
		}
		if opt.DryRun {
			report.add("hook", t.collection, label, "planned", "would run native collection action", "", "", "")
			return
		}
		if err := action(target); err != nil {
			report.add("hook", t.collection, label, "failed", "native collection action failed: "+err.Error(), "", "", "")
		} else {
			report.add("hook", t.collection, label, "ok", "completed", "", "", "")
		}
	}
	if !opt.NoSecrets {
		if managed {
			run("secrets:"+t.repo, func(target *Context) error {
				values := map[string]string{"repo": t.repo, "include_prod": "false"}
				if err := target.RunHook("secrets.link.pre", values); err != nil {
					return err
				}
				if _, err := target.LinkSecrets(SecretLinkOptions{Repo: t.repo}); err != nil {
					return err
				}
				return target.RunHook("secrets.link.post", values)
			})
		} else {
			report.add("hook", t.collection, "secrets:"+t.repo, "skipped", "unmanaged sibling; no registry secrets to link", "", "", "")
		}
	}
	if !t.harness {
		return
	}
	if !opt.NoEnv {
		run("env", func(target *Context) error {
			data, err := target.RenderEnv()
			if err != nil {
				return err
			}
			if err := target.ValidateEnvSupport(); err != nil {
				return err
			}
			if err := target.RunHook("env.pre", nil); err != nil {
				return err
			}
			if err := target.WriteEnv(data); err != nil {
				return err
			}
			if err := target.EnsureEnvSupport(); err != nil {
				return err
			}
			if err := target.TrustMise(); err != nil {
				return err
			}
			return target.RunHook("env.post", nil)
		})
	}
	if !opt.NoSkills {
		run("skills", func(target *Context) error {
			if err := target.RunHook("skills.render.pre", nil); err != nil {
				return err
			}
			if _, err := target.RenderSkills(SkillRenderOptions{}); err != nil {
				return err
			}
			return target.RunHook("skills.render.post", nil)
		})
	}
	if !opt.NoMCP {
		if _, err := os.Stat(filepath.Join(targetDir, "harness", ".mcp-servers.yml")); os.IsNotExist(err) {
			report.add("hook", t.collection, "mcp", "skipped", "no MCP registry in target harness", "", "", "")
		} else {
			run("mcp", func(target *Context) error {
				outputs, err := target.RenderMCP()
				if err != nil {
					return err
				}
				if err := target.RunHook("mcp.render.pre", nil); err != nil {
					return err
				}
				if _, err := target.WriteMCP(outputs, false); err != nil {
					return err
				}
				return target.RunHook("mcp.render.post", nil)
			})
		}
	}
	if opt.ReloadStatus {
		c.catchUpReloadStatus(report, t, opt)
	}
}
