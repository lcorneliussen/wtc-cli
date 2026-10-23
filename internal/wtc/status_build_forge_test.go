package wtc

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStatusParseGHBuildUsesRollupAndWorkflowRun(t *testing.T) {
	raw := []byte(`{"data":{"repository":{"ref":{"target":{"oid":"0123456789abcdef0123456789abcdef01234567","statusCheckRollup":{"state":"PENDING","contexts":{"nodes":[
		{"__typename":"StatusContext","targetUrl":"https://ci.example.invalid/1"},
		{"__typename":"CheckRun","checkSuite":{"workflowRun":{"runNumber":42,"url":"https://github.com/example/widget/actions/runs/9"}}}]}}}}}}}`)
	build, err := statusParseGHBuild(raw, "example/widget", "main")
	if err != nil || build == nil {
		t.Fatalf("build = %+v, %v", build, err)
	}
	if *build.Checks != "PENDING" || *build.Build != "42" || *build.URL != "https://github.com/example/widget/actions/runs/9" ||
		*build.SHA != "0123456789abcdef0123456789abcdef01234567" || build.Branch != "main" {
		t.Fatalf("unexpected GitHub build: %+v", build)
	}
	none, err := statusParseGHBuild([]byte(`{"data":{"repository":{"ref":{"target":{"oid":"abc1234","statusCheckRollup":null}}}}}`), "example/widget", "main")
	if err != nil || none != nil {
		t.Fatalf("a commit without checks has no build: %+v, %v", none, err)
	}
	missing, err := statusParseGHBuild([]byte(`{"data":{"repository":{"ref":null}}}`), "example/widget", "gone")
	if err != nil || missing != nil {
		t.Fatalf("a missing branch has no build: %+v, %v", missing, err)
	}
}

func TestStatusBBBuildReadsNewestBranchPipeline(t *testing.T) {
	var gotPath, gotUser string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path + "?" + r.URL.RawQuery
		gotUser, _, _ = r.BasicAuth()
		w.Write([]byte(`{"values":[
			{"build_number":12,"state":{"name":"COMPLETED","result":{"name":"FAILED"}},"target":{"ref_name":"main","selector":{"type":"custom"},"commit":{"hash":"aaaaaaa"}}},
			{"build_number":11,"state":{"name":"COMPLETED","result":{"name":"SUCCESSFUL"}},"target":{"ref_name":"main","selector":{"type":"branches"},"commit":{"hash":"bbbbbbb"}}}]}`))
	}))
	defer server.Close()
	old := statusBBAPI
	statusBBAPI = server.URL
	defer func() { statusBBAPI = old }()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("BB_API_USER", "")
	t.Setenv("BB_USERNAME", "builder")
	t.Setenv("BB_API_TOKEN", "token")
	build, err := statusForgeBuild("bitbucket.org", "team/widget", "main")
	if err != nil || build == nil {
		t.Fatalf("build = %+v, %v", build, err)
	}
	if gotUser != "builder" || !strings.Contains(gotPath, "/repositories/team/widget/pipelines/") || !strings.Contains(gotPath, "target.ref_name=main") {
		t.Fatalf("unexpected request %q as %q", gotPath, gotUser)
	}
	if *build.Build != "11" || *build.Checks != "SUCCESS" || *build.SHA != "bbbbbbb" ||
		*build.URL != "https://bitbucket.org/team/widget/pipelines/results/11" {
		t.Fatalf("custom pipelines must not stand in for the branch build: %+v", build)
	}
}

func TestBitbucketCredentialsFallBackToBBLogin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, name := range []string{"BB_API_USER", "BB_USERNAME", "ATLASSIAN_ACCOUNT_EMAIL"} {
		t.Setenv(name, "")
	}
	config := filepath.Join(home, ".config", "bb", "config.json")
	if err := os.MkdirAll(filepath.Dir(config), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, []byte(`{"username":"someone@example.invalid","apiToken":"secret","authMethod":"api-token"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if user, pass := bitbucketCredentials(); user != "someone@example.invalid" || pass != "secret" {
		t.Fatalf("bb login not used: %q %q", user, pass)
	}
	t.Setenv("BB_USERNAME", "env-user")
	t.Setenv("BB_API_TOKEN", "env-token")
	if user, pass := bitbucketCredentials(); user != "env-user" || pass != "env-token" {
		t.Fatalf("environment should win over the bb login: %q %q", user, pass)
	}
}

func TestStatusBuildHookNormalizesCommitAndMarksBuildWithoutNumber(t *testing.T) {
	c := newWorkspaceFixture(t)
	built := fixtureGit(t, "-C", c.Harness, "rev-parse", "HEAD")
	fixtureGit(t, "-C", c.Harness, "-c", "user.name=t", "-c", "user.email=t@example.invalid", "commit", "-q", "--allow-empty", "-m", "ahead")
	statusWriteBuildHook(t, c, "printf '{\"checks\":\"SUCCESS\",\"sha\":\""+strings.ToUpper(built)+"\"}\\n'\n")
	snapshot, err := c.StatusForgePreview()
	if err != nil {
		t.Fatal(err)
	}
	row := statusHarnessRow(t, snapshot)
	build := statusSoleBuild(row)
	if build == nil || build.Error != "" || build.SHA == nil || *build.SHA != built {
		t.Fatalf("an uppercase commit should be accepted and normalized: %+v", build)
	}
	if label := statusBuildLabel(build); label != "✓ (HEAD not in build)" {
		t.Fatalf("a build without a number still says HEAD is outside it: %q", label)
	}
}

func TestStatusBBBuildReportsMissingCredentials(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, name := range []string{"BB_API_USER", "BB_USERNAME", "ATLASSIAN_ACCOUNT_EMAIL"} {
		t.Setenv(name, "")
	}
	if _, err := statusForgeBuild("bitbucket.org", "team/widget", "main"); err == nil || !strings.Contains(err.Error(), "credentials") {
		t.Fatalf("missing credentials should be an error the cell can show: %v", err)
	}
}

func TestStatusBuildTiers(t *testing.T) {
	for _, tc := range []struct {
		repo Repo
		want string
	}{
		{Repo{DefaultRef: "origin/main"}, "prod:main"},
		{Repo{DefaultRef: "origin/master"}, "prod:master"},
		{Repo{DefaultRef: "origin/dev"}, "tip:dev prod:master"},
		{Repo{DefaultRef: "origin/dev", ProductionRef: "origin/master"}, "tip:dev prod:master"},
		{Repo{DefaultRef: "origin/main", ReleaseRef: "origin/release"}, "tip:main prod:release"},
		{Repo{DefaultRef: "origin/main", ProductionRef: "origin/main"}, "prod:main"},
	} {
		got := []string{}
		for _, tier := range statusBuildTiers(tc.repo) {
			got = append(got, tier.Tier+":"+tier.Branch)
		}
		if strings.Join(got, " ") != tc.want {
			t.Fatalf("%+v: tiers %v, want %s", tc.repo, got, tc.want)
		}
	}
}

func statusWriteBuildHook(t *testing.T, c *Context, script string) {
	t.Helper()
	hook := filepath.Join(c.Harness, "hooks", "wtc", "status.build.sh")
	if err := os.MkdirAll(filepath.Dir(hook), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hook, []byte("#!/bin/sh\ncat >/dev/null\n"+script), 0755); err != nil {
		t.Fatal(err)
	}
}

func statusSoleBuild(row StatusRepo) *StatusBuild {
	if row.Tip != nil {
		return row.Tip
	}
	return row.Prod
}

func statusHarnessRow(t *testing.T, snapshot StatusSnapshot) StatusRepo {
	t.Helper()
	for _, row := range snapshot.Repos {
		if row.Dir == "harness" {
			return row
		}
	}
	t.Fatal("harness row missing")
	return StatusRepo{}
}

func TestStatusBuildAttributesRowsByCommit(t *testing.T) {
	c := newWorkspaceFixture(t)
	built := fixtureGit(t, "-C", c.Harness, "rev-parse", "HEAD")
	statusWriteBuildHook(t, c, "printf '{\"checks\":\"SUCCESS\",\"build\":\"5\",\"sha\":\""+built+"\"}\\n'\n")
	snapshot, err := c.StatusForgePreview()
	if err != nil {
		t.Fatal(err)
	}
	if row := statusHarnessRow(t, snapshot); statusSoleBuild(row) == nil || statusSoleBuild(row).HeadInBuild == nil || !*statusSoleBuild(row).HeadInBuild {
		t.Fatalf("a row at the build commit is in the build: %+v", statusSoleBuild(row))
	}
	fixtureGit(t, "-C", c.Harness, "-c", "user.name=t", "-c", "user.email=t@example.invalid", "commit", "-q", "--allow-empty", "-m", "ahead")
	snapshot, err = c.StatusForgePreview()
	if err != nil {
		t.Fatal(err)
	}
	if row := statusHarnessRow(t, snapshot); statusSoleBuild(row) == nil || statusSoleBuild(row).HeadInBuild == nil || *statusSoleBuild(row).HeadInBuild {
		t.Fatalf("a row ahead of the build commit is not in it: %+v", statusSoleBuild(row))
	}
	if label := statusBuildLabel(statusSoleBuild(statusHarnessRow(t, snapshot))); !strings.Contains(label, "HEAD not in build") {
		t.Fatalf("Markdown should say the row is outside the build: %q", label)
	}
}

func TestStatusBuildHookCanDeferToDefaultProvider(t *testing.T) {
	c := newWorkspaceFixture(t)
	// The fixture's remotes are local paths, so the default provider has no
	// forge to ask and reports nothing, without an error.
	statusWriteBuildHook(t, c, "printf '{\"use_default\":true}\\n'\n")
	snapshot, err := c.StatusForgePreview()
	if err != nil {
		t.Fatal(err)
	}
	if row := statusHarnessRow(t, snapshot); row.Tip != nil || row.Prod != nil {
		t.Fatalf("deferring to the default provider without a forge yields no build: %+v", row)
	}
}

func TestStatusFollowPRThroughTiers(t *testing.T) {
	c := newWorkspaceFixture(t)
	git := func(args ...string) string {
		return fixtureGit(t, append([]string{"-C", c.Harness, "-c", "user.name=t", "-c", "user.email=t@example.invalid"}, args...)...)
	}
	before := git("rev-parse", "HEAD")
	git("commit", "-q", "--allow-empty", "-m", "merge")
	merge := git("rev-parse", "HEAD")
	ok, pending := "SUCCESS", "PENDING"
	n12, n13 := "12", "13"
	old := &StatusBuild{Branch: "main", Checks: &ok, Build: &n12, SHA: &before}
	running := &StatusBuild{Branch: "main", Checks: &pending, Build: &n13, SHA: &merge}
	passed := &StatusBuild{Branch: "main", Checks: &ok, Build: &n13, SHA: &merge}

	tiers := []statusBuildTier{{"tip", "main"}, {"prod", "prod"}}
	follow := func(pr *StatusPRRow, tip, prod *StatusBuild) {
		statusFollowPR(pr, c.Harness, tiers, map[string]*StatusBuild{"tip": tip, "prod": prod})
	}
	pr := StatusPRRow{State: "MERGED", MergeCommit: merge}
	follow(&pr, old, nil)
	if pr.Follow == nil || pr.Follow.Stage != "tip" || pr.Follow.Checks != "WAITING" || *pr.FollowTipBuild != "waiting #12" {
		t.Fatalf("a build older than the merge leaves the PR waiting: %+v", pr.Follow)
	}
	pr = StatusPRRow{State: "MERGED", MergeCommit: merge}
	follow(&pr, passed, running)
	if pr.Follow == nil || pr.Follow.Stage != "prod" || pr.Follow.Checks != "PENDING" {
		t.Fatalf("a passed test build moves the follow to production: %+v", pr.Follow)
	}
	pr = StatusPRRow{State: "MERGED", MergeCommit: merge}
	follow(&pr, passed, passed)
	if pr.Follow == nil || pr.Follow.Stage != "done" {
		t.Fatalf("both tiers passed with the merge in them: %+v", pr.Follow)
	}
	pr = StatusPRRow{State: "MERGED"}
	follow(&pr, passed, nil)
	if pr.Follow != nil {
		t.Fatalf("a merge recorded without its commit is shown as plainly merged: %+v", pr.Follow)
	}
	pr = StatusPRRow{State: "MERGED", MergeCommit: merge}
	unattributed := &StatusBuild{Branch: "main", Checks: &ok, Build: &n13}
	follow(&pr, unattributed, nil)
	if pr.Follow == nil || pr.Follow.Checks != "UNKNOWN" {
		t.Fatalf("a build without a commit cannot be attributed: %+v", pr.Follow)
	}
	pr = StatusPRRow{State: "MERGED", MergeCommit: merge, Base: "prod"}
	follow(&pr, old, running)
	if pr.Follow == nil || pr.Follow.Stage != "prod" || pr.FollowTipBuild != nil {
		t.Fatalf("a hotfix merged into the production branch is followed from prod only: %+v", pr.Follow)
	}
	pr = StatusPRRow{State: "MERGED", MergeCommit: merge, Base: "feature/parent"}
	follow(&pr, passed, passed)
	if pr.Follow != nil || pr.FollowTipBuild != nil {
		t.Fatalf("a PR merged into a branch no tier builds is not followed: %+v", pr.Follow)
	}
	pr = StatusPRRow{State: "OPEN", MergeCommit: merge}
	follow(&pr, passed, nil)
	if pr.Follow != nil {
		t.Fatalf("open PRs are not followed: %+v", pr.Follow)
	}
}

func TestMergedPRRecordsMergeCommit(t *testing.T) {
	c := fixture(t)
	if _, err := c.EnlistPR(PRRecord{Repo: "widget", Number: "7", Branch: "topic"}); err != nil {
		t.Fatal(err)
	}
	records, err := c.ListPRs()
	if err != nil {
		t.Fatal(err)
	}
	sha := "0123456789abcdef0123456789abcdef01234567"
	detail := statusPRDetail{Number: "7", State: "MERGED", MergedOn: "2026-09-30T12:00:00Z", Checks: "SUCCESS", MergeCommit: sha, Base: "main"}
	if count, err := c.recordMergedPRs(records, []statusPRDetail{detail}); err != nil || count != 1 {
		t.Fatalf("record: %d %v", count, err)
	}
	data, _ := os.ReadFile(c.PRFile())
	if !strings.Contains(string(data), "# merged-pr widget 7 2026-09-30T12:00:00Z SUCCESS\n# merged-commit widget 7 "+sha+" main\n") {
		t.Fatalf("merge commit row missing: %s", data)
	}
	records, err = c.ListPRs()
	if err != nil || len(records) != 1 || records[0].MergeCommit != sha || records[0].MergeBase != "main" {
		t.Fatalf("merge commit not read back: %+v %v", records, err)
	}
}

func TestStatusGHBuildPassesNumericNamesAsStrings(t *testing.T) {
	bin := t.TempDir()
	argsFile := filepath.Join(bin, "args")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + argsFile + "\nprintf '{\"data\":{\"repository\":{\"ref\":null}}}'\n"
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if _, err := statusGHBuild("2048/4096", "main"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	args := string(raw)
	for _, want := range []string{"-f\nowner=2048\n", "-f\nname=4096\n", "-f\nref=refs/heads/main\n"} {
		if !strings.Contains(args, want) {
			t.Fatalf("gh variables must be raw strings (-f), missing %q in:\n%s", want, args)
		}
	}
}
