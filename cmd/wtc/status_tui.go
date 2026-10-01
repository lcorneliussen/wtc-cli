package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"
	"github.com/lcorneliussen/wtc-cli/internal/wtc"
	"github.com/mattn/go-runewidth"
)

type statusTickMsg time.Time

type statusRefreshStartedMsg struct{ events <-chan tea.Msg }

type statusProgressMsg struct {
	message string
	at      time.Time
	events  <-chan tea.Msg
}

type statusLoadedMsg struct {
	snapshot  wtc.StatusSnapshot
	processes []wtc.StatusProcess
	fetched   wtc.StatusFetchReport
	err       error
	at        time.Time
}

type statusOpenedMsg struct{ err error }

type statusTUIModel struct {
	context      *wtc.Context
	snapshot     wtc.StatusSnapshot
	all          bool
	procs        bool
	reposOnly    bool
	processes    []wtc.StatusProcess
	noFetch      bool
	fetchAge     time.Duration
	noClick      bool
	interval     time.Duration
	background   time.Duration
	nextRefresh  time.Time
	lastRefresh  time.Time
	width        int
	height       int
	scroll       int
	focused      bool
	refreshing   bool
	showHelp     bool
	showArchived bool
	showLog      bool
	progressLog  []string
	progressStep string
	startedAt    time.Time
	errorText    string
}

func statusTUITick() tea.Cmd {
	return tea.Tick(time.Second, func(at time.Time) tea.Msg { return statusTickMsg(at) })
}

func statusTUIRefresh(c *wtc.Context, all, procs, noFetch bool, fetchAge time.Duration) tea.Cmd {
	return statusTUIRefreshWithCollector(c, procs, func(collector *wtc.Context) statusLoadedMsg {
		snapshot, report, err := collector.StatusLiveSnapshotWithFetchAge(all, noFetch, fetchAge)
		return statusLoadedMsg{snapshot: snapshot, fetched: report, err: err, at: time.Now()}
	})
}

func statusTUIRefreshWithCollector(c *wtc.Context, procs bool, collect func(*wtc.Context) statusLoadedMsg) tea.Cmd {
	return func() tea.Msg {
		events := make(chan tea.Msg, 128)
		go func() {
			if procs {
				processes, err := c.StatusProcesses()
				events <- statusLoadedMsg{processes: processes, err: err, at: time.Now()}
				return
			}
			collector := *c
			collector.StatusProgress = func(message string) {
				select {
				case events <- statusProgressMsg{message: message, at: time.Now(), events: events}:
				default: // Keep collecting if the terminal cannot render every step.
				}
			}
			events <- collect(&collector)
		}()
		return statusRefreshStartedMsg{events: events}
	}
}

func statusTUIWait(events <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg { return <-events }
}

func (m *statusTUIModel) startRefresh() {
	m.refreshing = true
	m.startedAt = time.Now()
	m.progressStep = "Starting refresh"
	m.progressLog = []string{"0s  Starting refresh"}
}

func (m *statusTUIModel) focusLogTail() {
	if m.showLog {
		height := m.height
		if height <= 0 {
			height = 24
		}
		m.scroll = max(0, len(m.contentLines())-max(1, height-2))
	}
}

func (m statusTUIModel) Init() tea.Cmd {
	return tea.Batch(statusTUITick(), statusTUIRefresh(m.context, m.all, m.procs, m.noFetch, m.fetchAge))
}

func (m statusTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.FocusMsg:
		m.focused = true
		m.nextRefresh = time.Now().Add(m.interval)
	case tea.BlurMsg:
		m.focused = false
		m.nextRefresh = time.Now().Add(m.background)
	case statusRefreshStartedMsg:
		return m, statusTUIWait(msg.events)
	case statusProgressMsg:
		if m.startedAt.IsZero() {
			m.startedAt = msg.at
		}
		m.progressStep = msg.message
		m.progressLog = append(m.progressLog, fmt.Sprintf("%s  %s", msg.at.Sub(m.startedAt).Truncate(time.Second), msg.message))
		m.focusLogTail()
		return m, statusTUIWait(msg.events)
	case statusLoadedMsg:
		if m.startedAt.IsZero() {
			m.startedAt = msg.at
		}
		m.refreshing = false
		m.progressStep = ""
		outcome := "Refresh finished"
		if msg.err != nil {
			outcome = "Refresh failed: " + msg.err.Error()
		} else if msg.fetched.Failed != 0 {
			plural := "es"
			if msg.fetched.Failed == 1 {
				plural = ""
			}
			outcome = fmt.Sprintf("Refresh finished: %d ref refresh%s failed; showing local refs", msg.fetched.Failed, plural)
		}
		m.progressLog = append(m.progressLog, fmt.Sprintf("%s  %s", msg.at.Sub(m.startedAt).Truncate(time.Second), outcome))
		m.focusLogTail()
		m.lastRefresh = msg.at
		m.nextRefresh = msg.at.Add(m.interval)
		if !m.focused {
			m.nextRefresh = msg.at.Add(m.background)
		}
		if msg.err != nil {
			m.errorText = msg.err.Error()
		} else {
			m.snapshot = msg.snapshot
			m.processes = msg.processes
			m.errorText = ""
			if msg.fetched.Failed != 0 {
				m.errorText = fmt.Sprintf("%d ref refresh(es) failed; showing local refs", msg.fetched.Failed)
			}
		}
	case statusTickMsg:
		if !m.refreshing && !m.nextRefresh.IsZero() && !time.Now().Before(m.nextRefresh) {
			m.startRefresh()
			return m, tea.Batch(statusTUITick(), statusTUIRefresh(m.context, m.all, m.procs, m.noFetch, m.fetchAge))
		}
		return m, statusTUITick()
	case tea.MouseClickMsg:
		header := m.headerLine()
		refreshIndex := strings.Index(header, "refreshing")
		headerWidth := m.width
		if headerWidth <= 0 {
			headerWidth = 80
		}
		if msg.Y == 0 && m.refreshing && refreshIndex >= 0 &&
			msg.X >= runewidth.StringWidth(header[:refreshIndex]) &&
			msg.X < min(headerWidth, runewidth.StringWidth(header)) {
			m.showLog = !m.showLog
			m.scroll = 0
			m.focusLogTail()
			return m, nil
		}
		if !m.noClick && !m.procs {
			if target := m.buildClickTarget(msg.X, msg.Y); target != "" {
				return m, statusOpenURL(target)
			}
		}
	case statusOpenedMsg:
		if msg.err != nil {
			m.errorText = msg.err.Error()
		}
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.showHelp = false
		case "?":
			m.showHelp = !m.showHelp
		case "a":
			m.showArchived = !m.showArchived
		case "r":
			if !m.refreshing {
				m.startRefresh()
				return m, statusTUIRefresh(m.context, m.all, m.procs, m.noFetch, m.fetchAge)
			}
		case "l":
			m.showLog = !m.showLog
			m.scroll = 0
			m.focusLogTail()
		case "up", "k":
			if m.scroll > 0 {
				m.scroll--
			}
		case "down", "j":
			m.scroll++
		case "pgup":
			m.scroll -= m.height / 2
			if m.scroll < 0 {
				m.scroll = 0
			}
		case "pgdown":
			m.scroll += m.height / 2
		}
	}
	return m, nil
}

func statusTUIFit(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if runewidth.StringWidth(value) <= width {
		return value + strings.Repeat(" ", width-runewidth.StringWidth(value))
	}
	if width == 1 {
		return "…"
	}
	var b strings.Builder
	used := 0
	for _, r := range value {
		w := runewidth.RuneWidth(r)
		if used+w >= width {
			break
		}
		b.WriteRune(r)
		used += w
	}
	return b.String() + strings.Repeat(" ", width-1-used) + "…"
}

func statusTUIPR(pr *wtc.StatusPRFacts) string {
	if pr == nil || pr.Number == "" {
		return "·"
	}
	parts := []string{"#" + pr.Number}
	if pr.Draft {
		parts = append(parts, "◇")
	}
	for _, token := range []string{pr.Checks, pr.Merge, pr.Review} {
		switch token {
		case "SUCCESS", "approved":
			parts = append(parts, "✓")
		case "FAILURE", "ERROR":
			parts = append(parts, "✗")
		case "PENDING", "EXPECTED":
			parts = append(parts, "●")
		case "BEHIND":
			parts = append(parts, "↓")
		case "DIRTY":
			parts = append(parts, "⚠")
		case "BLOCKED":
			parts = append(parts, "⊘")
		case "changes":
			parts = append(parts, "!")
		case "waiting":
			parts = append(parts, "…")
		case "commented":
			parts = append(parts, "✎")
		case "noreviewers":
			parts = append(parts, "∅")
		}
	}
	return strings.Join(parts, " ")
}

type statusRepoLayout struct {
	name, branch, pr, tree, ahead, behind, tip, prod int
	showTree, showSync, showBuilds                   bool
}

func statusTUIRepoLayout(snapshot wtc.StatusSnapshot, width int) statusRepoLayout {
	l := statusRepoLayout{name: 16, branch: 30, pr: 15, tree: 4, ahead: 3, behind: 3, tip: 8, prod: 8,
		showTree: true, showSync: true, showBuilds: true}
	for _, row := range snapshot.Repos {
		if row.Tree != "clean" {
			l.tree = max(l.tree, runewidth.StringWidth(row.Tree))
		}
		l.ahead = max(l.ahead, len(fmt.Sprint(row.Ahead)))
		l.behind = max(l.behind, len(fmt.Sprint(row.Behind)))
	}
	if width >= 100 {
		l.name = 20
	}
	if width < 72 {
		l.showBuilds = false
	}
	if width < 55 {
		l.showSync = false
	}
	if width < 46 {
		l.name = 10
	}
	if width < 40 {
		l.name, l.pr = 8, 8
		l.showSync, l.showBuilds = false, false
		l.showTree = width >= 31
	}
	other := func() int {
		n := l.name + 1 + l.pr
		if l.showTree {
			n += 1 + l.tree
		}
		if l.showSync {
			n += 1 + l.ahead + 1 + l.behind
		}
		if l.showBuilds {
			n += 1 + l.tip + 1 + l.prod
		}
		return n
	}
	minBranch := 8
	if width < 31 {
		minBranch = 6
	}
	if other()+minBranch+1 > width {
		l.showBuilds = false
	}
	if other()+minBranch+1 > width {
		l.showSync = false
	}
	l.branch = max(minBranch, min(l.branch, width-other()-1))
	return l
}

func (l statusRepoLayout) prStart() int { return l.name + 1 + l.branch + 1 }
func (l statusRepoLayout) tipStart() int {
	start := l.prStart() + l.pr + 1 + l.tree
	if l.showSync {
		start += 1 + l.ahead + 1 + l.behind
	}
	return start + 1
}

func statusTUIBuildCell(prefix string, build *wtc.StatusBuild) string {
	if build == nil {
		return "·"
	}
	glyph := "·"
	if build.Checks != nil {
		switch *build.Checks {
		case "SUCCESS":
			glyph = "✓"
		case "FAILURE", "ERROR":
			glyph = "✗"
		case "PENDING", "EXPECTED":
			glyph = "●"
		}
	}
	cell := prefix + glyph
	if build.Build != nil && *build.Build != "" {
		cell += "#" + *build.Build
	}
	return cell
}

func statusTUIRepoPRCell(pr *wtc.StatusPRFacts, width int, styled bool) string {
	plain := statusTUIPR(pr)
	if !styled || pr == nil || pr.Number == "" {
		return statusTUIFit(plain, width)
	}
	label := "#" + pr.Number
	if statusTUIURL(pr.URL) != "" {
		label = statusTUILink(label+" ↗", pr.URL)
	} else {
		label = statusTUIStyle(label, statusToneLabel)
	}
	suffix := strings.TrimPrefix(plain, "#"+pr.Number)
	if strings.Contains(suffix, "✗") {
		suffix = statusTUIStyle(suffix, statusToneFailure)
	} else if strings.Contains(suffix, "✓") {
		suffix = statusTUIStyle(suffix, statusToneSuccess)
	} else if strings.Contains(suffix, "●") || strings.Contains(suffix, "↓") {
		suffix = statusTUIStyle(suffix, statusToneWarning)
	}
	return statusTUIFitANSI(label+suffix, width)
}

func statusTUIRepoLines(snapshot wtc.StatusSnapshot, width int, styled bool) []string {
	l := statusTUIRepoLayout(snapshot, width)
	header := statusTUIFit("REPO", l.name) + " " + statusTUIFit("BRANCH", l.branch) + " " + statusTUIFit("PR", l.pr)
	if l.showTree {
		header += " " + statusTUIFit("±", l.tree)
	}
	if l.showSync {
		header += " " + statusTUIFit("↑", l.ahead) + " " + statusTUIFit("↓", l.behind)
	}
	if l.showBuilds {
		header += " " + statusTUIFit("TEST", l.tip) + " " + statusTUIFit("PROD", l.prod)
	}
	if styled {
		header = statusTUIStyle(header, statusToneHeading)
	}
	lines := []string{header}
	for _, row := range snapshot.Repos {
		name := row.Dir
		if snapshot.ShowCollectionColumn {
			name = row.Collection + "/" + row.Dir
		}
		tree := "·"
		if row.Tree != "" && row.Tree != "clean" {
			tree = row.Tree
		}
		nameCell := statusTUIFit(statusTUISafe(name), l.name)
		branchCell := statusTUIFit(statusTUISafe(row.BranchDisplay), l.branch)
		prCell := statusTUIRepoPRCell(row.PR, l.pr, styled)
		if styled {
			nameCell = statusTUIStyle(nameCell, statusToneLabel)
			if row.BranchKind == "detached" {
				branchCell = statusTUIStyle(branchCell, statusToneDim)
			}
		}
		treeCell := statusTUIFit(tree, l.tree)
		if styled {
			if tree == "·" {
				treeCell = statusTUIStyle(treeCell, statusToneDim)
			} else {
				treeCell = statusTUIStyle(treeCell, statusToneWarning)
			}
		}
		line := nameCell + " " + branchCell + " " + prCell
		if l.showTree {
			line += " " + treeCell
		}
		if l.showSync {
			ahead, behind := "·", "·"
			if row.Ahead > 0 {
				ahead = fmt.Sprint(row.Ahead)
			}
			if row.Behind > 0 {
				behind = fmt.Sprint(row.Behind)
			}
			aheadCell, behindCell := statusTUIFit(ahead, l.ahead), statusTUIFit(behind, l.behind)
			if styled {
				if row.Ahead > 0 {
					aheadCell = statusTUIStyle(aheadCell, statusToneSuccess)
				} else {
					aheadCell = statusTUIStyle(aheadCell, statusToneDim)
				}
				if row.Behind > 0 {
					behindCell = statusTUIStyle(behindCell, statusToneWarning)
				} else {
					behindCell = statusTUIStyle(behindCell, statusToneDim)
				}
			}
			line += " " + aheadCell + " " + behindCell
		}
		if l.showBuilds {
			tipCell := statusTUIFit(statusTUIBuildCell("T", row.Tip), l.tip)
			prodCell := statusTUIFit(statusTUIBuildCell("P", row.Prod), l.prod)
			if styled {
				tipCell = statusTUIBuildLink(tipCell, row.Tip)
				prodCell = statusTUIBuildLink(prodCell, row.Prod)
			}
			line += " " + tipCell + " " + prodCell
		}
		lines = append(lines, line)
	}
	if len(snapshot.Repos) == 0 {
		lines = append(lines, "(no worktrees)")
	}
	return lines
}

func (m statusTUIModel) buildClickTarget(x, y int) string {
	if m.showLog || os.Getenv("TERM") == "dumb" {
		return ""
	}
	width, height := m.width, m.height
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}
	if x < 0 || x >= width || y < 0 || y >= max(1, height-2) {
		return ""
	}
	base := 3
	if m.refreshing && m.progressStep != "" {
		base += 2
	}
	if m.showHelp {
		base += 3
	}
	rowIndex := y + m.effectiveScroll() - base
	if rowIndex < 0 {
		return ""
	}
	if rowIndex < len(m.snapshot.Repos) {
		row := m.snapshot.Repos[rowIndex]
		layout := statusTUIRepoLayout(m.snapshot, width)
		prStart := layout.prStart()
		if x >= prStart && x < prStart+layout.pr && row.PR != nil {
			return statusTUIURL(row.PR.URL)
		}
		if !layout.showBuilds {
			return ""
		}
		tipStart := layout.tipStart()
		var build *wtc.StatusBuild
		if x >= tipStart && x < tipStart+layout.tip {
			build = row.Tip
		} else if x >= tipStart+layout.tip+1 && x < tipStart+layout.tip+1+layout.prod {
			build = row.Prod
		}
		if build != nil && build.URL != nil {
			return statusTUIURL(*build.URL)
		}
		return ""
	}
	if m.reposOnly || m.snapshot.ShowCollectionColumn {
		return ""
	}
	visibleIndex := rowIndex - max(1, len(m.snapshot.Repos)) - 3
	if visibleIndex < 0 {
		return ""
	}
	if x >= statusTUIPRLayout(width).number {
		return ""
	}
	visible, _ := statusTUIVisiblePRs(m.snapshot, m.showArchived)
	for _, row := range visible {
		if visibleIndex == 0 && row.URL != nil {
			return statusTUIURL(*row.URL)
		}
		visibleIndex--
	}
	return ""
}

func (m statusTUIModel) effectiveScroll() int {
	height := m.height
	if height <= 0 {
		height = 24
	}
	return min(max(0, m.scroll), max(0, len(m.contentLines())-max(1, height-2)))
}

func statusOpenURL(target string) tea.Cmd {
	return func() tea.Msg {
		tool := "xdg-open"
		if runtime.GOOS == "darwin" {
			tool = "open"
		}
		command := exec.Command(tool, target)
		if err := command.Run(); err != nil {
			return statusOpenedMsg{err: fmt.Errorf("open status URL: %w", err)}
		}
		return statusOpenedMsg{}
	}
}

func statusTUIPRGlyph(value *string) string {
	if value == nil {
		return "·"
	}
	switch *value {
	case "SUCCESS", "approved", "MERGEABLE":
		return "✓"
	case "FAILURE", "ERROR", "changes", "CONFLICTING":
		return "✗"
	case "PENDING", "EXPECTED", "waiting":
		return "●"
	case "BEHIND":
		return "↓"
	case "DIRTY":
		return "⚠"
	case "BLOCKED":
		return "⊘"
	case "commented":
		return "✎"
	case "noreviewers":
		return "∅"
	}
	return "·"
}

func statusTUIMergedPR(row wtc.StatusPRRow) bool {
	return row.Merge != nil && *row.Merge == "MERGED" || row.MergedOn != nil && *row.MergedOn != ""
}

func statusTUIVisiblePRs(snapshot wtc.StatusSnapshot, showArchived bool) ([]wtc.StatusPRRow, int) {
	active := make([]wtc.StatusPRRow, 0, len(snapshot.PRs))
	merged := make([]wtc.StatusPRRow, 0)
	archived := make([]wtc.StatusPRRow, 0)
	for _, row := range snapshot.PRs {
		switch {
		case row.Archived:
			archived = append(archived, row)
		case statusTUIMergedPR(row) && !row.OnBranch:
			merged = append(merged, row)
		default:
			active = append(active, row)
		}
	}
	active = append(active, merged...)
	if showArchived {
		active = append(active, archived...)
	}
	return active, len(archived)
}

type statusPRLayout struct {
	number, repo, state, title int
	signals                    bool
}

func statusTUIPRLayout(width int) statusPRLayout {
	l := statusPRLayout{number: 7, repo: 16, state: 10, signals: true}
	switch {
	case width >= 52:
	case width >= 40:
		l.repo, l.state = 12, 8
	case width >= 34:
		l.repo, l.state, l.signals = 10, 8, false
	default:
		l.number, l.repo, l.state, l.signals = 5, 8, 7, false
	}
	if width >= 100 {
		l.repo = 20
	}
	prefix := l.number + 1 + l.repo + 1 + l.state + 1
	if l.signals {
		prefix += 6 // C M R and the separating space before TITLE.
	}
	l.title = max(1, width-prefix)
	return l
}

func statusTUIPRLines(snapshot wtc.StatusSnapshot, width int, showArchived, styled bool) []string {
	if snapshot.ShowCollectionColumn {
		return nil
	}
	heading := "PRs"
	if styled {
		heading = statusTUIStyle(heading, statusToneHeading)
	}
	l := statusTUIPRLayout(width)
	header := statusTUIFit("PR", l.number) + " " + statusTUIFit("REPO", l.repo) + " " + statusTUIFit("STATE", l.state) + " "
	if l.signals {
		header += "C M R "
	}
	header += statusTUIFit("TITLE", l.title)
	if styled {
		header = statusTUIStyle(header, statusToneDim)
	}
	lines := []string{"", heading, header}
	visible, archived := statusTUIVisiblePRs(snapshot, showArchived)
	for _, row := range visible {
		label := statusTUISafe(row.DisplayTitle)
		if label == "" {
			label = statusTUISafe(row.Title)
		}
		if label == "" {
			label = "PR #" + row.Number
		}
		state := "open"
		merged := statusTUIMergedPR(row)
		if row.OnBranch {
			state = "⚠ catch-up"
		} else if row.Draft {
			state = "◇ draft"
		} else if merged {
			state = "merged"
		}
		number := statusTUIFit("#"+row.Number, l.number)
		repo := statusTUIFit(statusTUISafe(row.Repo), l.repo)
		stateCell := statusTUIFit(state, l.state)
		checks, merge, review := statusTUIPRGlyph(row.Checks), statusTUIPRGlyph(row.Merge), statusTUIPRGlyph(row.Review)
		if merged && !row.OnBranch {
			checks, merge, review = "·", "·", "·"
		}
		title := statusTUIFit(label, l.title)
		if styled {
			if row.URL != nil && statusTUIURL(*row.URL) != "" {
				linkTone := statusToneLink
				if merged && !row.OnBranch {
					linkTone = "2;4;38;5;245"
				}
				number = statusTUIFitANSI(statusTUILinkTone("#"+row.Number+" ↗", *row.URL, linkTone), l.number)
			}
			rowTone := statusToneLabel
			if merged && !row.OnBranch {
				rowTone = "2;38;5;245"
			}
			repo = statusTUIStyle(repo, rowTone)
			title = statusTUIStyle(title, rowTone)
			if row.OnBranch {
				stateCell = statusTUIStyle(stateCell, statusToneWarning)
			} else {
				stateCell = statusTUIStyle(stateCell, statusToneDim)
			}
			checks = statusTUIGlyphStyle(checks)
			merge = statusTUIGlyphStyle(merge)
			review = statusTUIGlyphStyle(review)
		}
		line := number + " " + repo + " " + stateCell + " "
		if l.signals {
			line += checks + " " + merge + " " + review + " "
		}
		lines = append(lines, line+title)
	}
	if len(snapshot.PRs) == 0 {
		lines = append(lines, "(none enlisted)")
	}
	if archived > 0 && !showArchived {
		hint := fmt.Sprintf("%d archived PR(s) hidden; press a", archived)
		if styled {
			hint = statusTUIStyle(fmt.Sprintf("▸ archived (%d) · a to show", archived), statusToneDim)
		}
		lines = append(lines, hint)
	}
	for _, orphan := range snapshot.Orphans {
		warning := fmt.Sprintf("⚠ %s on %s: PR %s; catch-up", orphan.Repo, orphan.Branch, orphan.State)
		if styled {
			warning = statusTUIStyle(warning, statusToneWarning)
		}
		lines = append(lines, warning)
	}
	return lines
}

func statusTUIGlyphStyle(glyph string) string {
	switch glyph {
	case "✓":
		return statusTUIStyle(glyph, statusToneSuccess)
	case "✗":
		return statusTUIStyle(glyph, statusToneFailure)
	case "●", "↓", "⚠":
		return statusTUIStyle(glyph, statusToneWarning)
	default:
		return statusTUIStyle(glyph, statusToneDim)
	}
}

func (m statusTUIModel) headerLine() string {
	name := m.snapshot.Collection
	if m.procs {
		name = "processes"
	}
	if name == "" {
		name = "(all)"
	}
	age := "loading"
	if !m.lastRefresh.IsZero() {
		age = fmt.Sprintf("%ds old", max(0, int(time.Since(m.lastRefresh).Seconds())))
	}
	if m.refreshing {
		age += " · refreshing"
		if m.progressStep != "" {
			age += ": " + m.progressStep
		}
	}
	return fmt.Sprintf("wtc status · %s · %s", name, age)
}

func (m statusTUIModel) contentLines() []string {
	width := m.width
	if width <= 0 {
		width = 80
	}
	lines := []string{statusTUIStyle(m.headerLine(), statusToneHeading), ""}
	if m.refreshing && m.progressStep != "" {
		lines = append(lines, statusTUIStyle(statusTUISafe(m.progressStep)+" (l: refresh log)", statusToneWarning), "")
	}
	if m.showHelp {
		keys := "r refresh   l log   a show/hide archived PRs   ? help   q quit"
		if m.procs {
			keys = "r refresh   l log   ? help   q quit"
		}
		lines = append(lines, keys, "↗ link: modifier-click   PR/T/P: click   ↑/↓ scroll   PgUp/PgDn faster", "")
	}
	if m.showLog {
		lines = append(lines, statusTUIStyle("Refresh log", statusToneHeading), "")
		lines = append(lines, m.progressLog...)
		return lines
	}
	if m.procs {
		lines = append(lines, strings.Split(strings.TrimSuffix(wtc.StatusProcessesText(m.processes), "\n"), "\n")...)
	} else {
		lines = append(lines, statusTUIRepoLines(m.snapshot, width, true)...)
		if !m.reposOnly {
			lines = append(lines, statusTUIPRLines(m.snapshot, width, m.showArchived, true)...)
		}
		if m.snapshot.StaleCount > 0 {
			lines = append(lines, "", fmt.Sprintf("%d worktree(s) behind the development tip", m.snapshot.StaleCount))
		}
	}
	if m.errorText != "" {
		lines = append(lines, "", statusTUIStyle("warning: "+statusTUISafe(m.errorText), statusToneWarning))
	}
	return lines
}

func (m statusTUIModel) View() tea.View {
	width := m.width
	if width <= 0 {
		width = 80
	}
	height := m.height
	if height <= 0 {
		height = 24
	}
	lines := m.contentLines()
	footer := "r refresh · l log · a archived · ↗ link · ? help · q quit"
	if m.procs {
		footer = "r refresh · l log · ? help · q quit"
	}
	if !m.nextRefresh.IsZero() && !m.refreshing {
		footer += fmt.Sprintf(" · next %ds", max(0, int(time.Until(m.nextRefresh).Seconds())))
	}
	bodyHeight := max(1, height-2)
	scroll := m.effectiveScroll()
	end := min(len(lines), scroll+bodyHeight)
	var b strings.Builder
	visibleLink := false
	for _, line := range lines[scroll:end] {
		fitted := statusTUIFitANSI(line, width)
		b.WriteString(fitted)
		b.WriteByte('\n')
		if statusTUIHasVisibleLink(fitted) {
			visibleLink = true
		}
	}
	for i := end - scroll; i < bodyHeight; i++ {
		b.WriteByte('\n')
	}
	b.WriteString(statusTUIFitANSI(statusTUIStyle(footer, statusToneDim), width))
	view := tea.NewView(b.String())
	view.AltScreen = true
	view.ReportFocus = os.Getenv("TERM") != "dumb"
	if os.Getenv("TERM") != "dumb" && (m.refreshing || visibleLink && !m.noClick) {
		view.MouseMode = tea.MouseModeCellMotion
	}
	return view
}

func runStatusTUI(c *wtc.Context, all, procs, reposOnly, noFetch, noClick bool, interval, background, fetchAge time.Duration) error {
	if !term.IsTerminal(os.Stdin.Fd()) || !term.IsTerminal(os.Stdout.Fd()) {
		return fmt.Errorf("status TUI requires a terminal")
	}
	model := statusTUIModel{context: c, all: all, procs: procs, reposOnly: reposOnly, noFetch: noFetch, noClick: noClick, fetchAge: fetchAge, interval: interval,
		background: background, focused: true, refreshing: true}
	model.startRefresh()
	if !all && !procs {
		model.snapshot.Collection = filepath.Base(c.Collection)
	}
	if !all && !procs {
		if cached, err := c.ReadStatusSnapshot(); err == nil {
			model.snapshot = cached
			if age, err := c.CachedStatusAge(); err == nil {
				model.lastRefresh = time.Now().Add(-time.Duration(age) * time.Second)
			}
		}
	}
	_, err := tea.NewProgram(model).Run()
	return err
}
