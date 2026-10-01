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

func statusTUIBuildColumns(snapshot wtc.StatusSnapshot) bool {
	for _, row := range snapshot.Repos {
		if row.Tip != nil || row.Prod != nil {
			return true
		}
	}
	return false
}

func statusTUIBranchWidth(width int, builds bool) int {
	if builds {
		return max(14, width-64)
	}
	return max(14, width-48)
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

func statusTUIRepoPRCell(pr *wtc.StatusPRFacts, styled bool) string {
	plain := statusTUIPR(pr)
	if !styled || pr == nil || pr.Number == "" {
		return statusTUIFit(plain, 15)
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
	return statusTUIFitANSI(label+suffix, 15)
}

func statusTUIRepoLines(snapshot wtc.StatusSnapshot, width int, styled bool) []string {
	if width < 40 {
		width = 40
	}
	nameWidth := 16
	builds := statusTUIBuildColumns(snapshot)
	branchWidth := statusTUIBranchWidth(width, builds)
	header := statusTUIFit("REPO", nameWidth) + " " + statusTUIFit("BRANCH", branchWidth) + " " + statusTUIFit("PR", 15) + " "
	if builds {
		header += statusTUIFit("LOCAL", 12) + " " + statusTUIFit("TIP", 8) + " " + statusTUIFit("PROD", 8)
	} else {
		header += "LOCAL"
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
		local := "·"
		if row.Tree != "" && row.Tree != "clean" {
			local = row.Tree
		}
		if row.Ahead > 0 {
			local += fmt.Sprintf(" ↑%d", row.Ahead)
		}
		if row.Behind > 0 {
			local += fmt.Sprintf(" ↓%d", row.Behind)
		}
		nameCell := statusTUIFit(statusTUISafe(name), nameWidth)
		branchCell := statusTUIFit(statusTUISafe(row.BranchDisplay), branchWidth)
		prCell := statusTUIRepoPRCell(row.PR, styled)
		if styled {
			nameCell = statusTUIStyle(nameCell, statusToneLabel)
			if row.BranchKind == "detached" {
				branchCell = statusTUIStyle(branchCell, statusToneDim)
			}
		}
		line := nameCell + " " + branchCell + " " + prCell + " "
		if builds {
			localCell := statusTUIFit(local, 12)
			tipCell := statusTUIFit(statusTUIBuildCell("T", row.Tip), 8)
			prodCell := statusTUIFit(statusTUIBuildCell("P", row.Prod), 8)
			if styled {
				localCell = statusTUILocalCell(localCell, row)
				tipCell = statusTUIBuildLink(tipCell, row.Tip)
				prodCell = statusTUIBuildLink(prodCell, row.Prod)
			}
			line += localCell + " " + tipCell + " " + prodCell
		} else {
			if styled {
				local = statusTUILocalCell(local, row)
			}
			line += local
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
		prStart := 18 + statusTUIBranchWidth(max(40, m.width), statusTUIBuildColumns(m.snapshot))
		if x >= prStart && x < prStart+15 && row.PR != nil {
			return statusTUIURL(row.PR.URL)
		}
		if !statusTUIBuildColumns(m.snapshot) {
			return ""
		}
		tipStart := 47 + statusTUIBranchWidth(max(40, m.width), true)
		var build *wtc.StatusBuild
		if x >= tipStart && x < tipStart+8 {
			build = row.Tip
		} else if x >= tipStart+9 && x < tipStart+17 {
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
	visibleIndex := rowIndex - max(1, len(m.snapshot.Repos)) - 2
	if visibleIndex < 0 {
		return ""
	}
	for _, row := range m.snapshot.PRs {
		if row.Archived && !m.showArchived {
			continue
		}
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

func statusTUIPRLines(snapshot wtc.StatusSnapshot, showArchived, styled bool) []string {
	if snapshot.ShowCollectionColumn {
		return nil
	}
	heading := "PRs"
	if styled {
		heading = statusTUIStyle(heading, statusToneHeading)
	}
	lines := []string{"", heading}
	archived := 0
	for _, row := range snapshot.PRs {
		if row.Archived && !showArchived {
			archived++
			continue
		}
		label := statusTUISafe(row.Title)
		if label == "" {
			label = "PR #" + row.Number
		}
		state := ""
		if row.OnBranch {
			state = " ⚠ merged; catch-up"
		} else if row.Draft {
			state = " ◇ draft"
		} else if row.Merge != nil && *row.Merge == "MERGED" {
			state = " merged"
		}
		if styled {
			number := "#" + row.Number
			if row.URL != nil && statusTUIURL(*row.URL) != "" {
				number = statusTUILink(number+" ↗", *row.URL)
			}
			stateCell := statusTUIStyle(state, statusToneDim)
			if row.OnBranch {
				stateCell = statusTUIStyle(state, statusToneWarning)
			}
			if row.Merge != nil && *row.Merge == "MERGED" {
				label = statusTUIStyle(label, statusToneDim)
			}
			lines = append(lines, statusTUIStyle(statusTUISafe(row.Repo), statusToneLabel)+" "+number+stateCell+"  "+label)
		} else {
			lines = append(lines, fmt.Sprintf("%s #%s%s  %s", row.Repo, row.Number, state, label))
		}
	}
	if len(snapshot.PRs) == 0 {
		lines = append(lines, "(none enlisted)")
	}
	if archived > 0 {
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
			lines = append(lines, statusTUIPRLines(m.snapshot, m.showArchived, true)...)
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
