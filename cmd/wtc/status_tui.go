package main

import (
	"fmt"
	"net/url"
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
	errorText    string
}

func statusTUITick() tea.Cmd {
	return tea.Tick(time.Second, func(at time.Time) tea.Msg { return statusTickMsg(at) })
}

func statusTUIRefresh(c *wtc.Context, all, procs, noFetch bool) tea.Cmd {
	return func() tea.Msg {
		if procs {
			processes, err := c.StatusProcesses()
			return statusLoadedMsg{processes: processes, err: err, at: time.Now()}
		}
		snapshot, report, err := c.StatusLiveSnapshot(all, noFetch)
		return statusLoadedMsg{snapshot: snapshot, fetched: report, err: err, at: time.Now()}
	}
}

func (m statusTUIModel) Init() tea.Cmd {
	return tea.Batch(statusTUITick(), statusTUIRefresh(m.context, m.all, m.procs, m.noFetch))
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
	case statusLoadedMsg:
		m.refreshing = false
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
			m.refreshing = true
			return m, tea.Batch(statusTUITick(), statusTUIRefresh(m.context, m.all, m.procs, m.noFetch))
		}
		return m, statusTUITick()
	case tea.MouseClickMsg:
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
				m.refreshing = true
				return m, statusTUIRefresh(m.context, m.all, m.procs, m.noFetch)
			}
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

func statusTUIRepoLines(snapshot wtc.StatusSnapshot, width int) []string {
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
		line := statusTUIFit(name, nameWidth) + " " + statusTUIFit(row.BranchDisplay, branchWidth) + " " + statusTUIFit(statusTUIPR(row.PR), 15) + " "
		if builds {
			line += statusTUIFit(local, 12) + " " + statusTUIFit(statusTUIBuildCell("T", row.Tip), 8) + " " + statusTUIFit(statusTUIBuildCell("P", row.Prod), 8)
		} else {
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
	if !statusTUIBuildColumns(m.snapshot) {
		return ""
	}
	base := 3
	if m.showHelp {
		base += 3
	}
	rowIndex := y + m.effectiveScroll() - base
	if rowIndex < 0 || rowIndex >= len(m.snapshot.Repos) {
		return ""
	}
	tipStart := 47 + statusTUIBranchWidth(max(40, m.width), true)
	var build *wtc.StatusBuild
	if x >= tipStart && x < tipStart+8 {
		build = m.snapshot.Repos[rowIndex].Tip
	} else if x >= tipStart+9 && x < tipStart+17 {
		build = m.snapshot.Repos[rowIndex].Prod
	}
	if build == nil || build.URL == nil {
		return ""
	}
	parsed, err := url.Parse(*build.URL)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
		return ""
	}
	return *build.URL
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
			return statusOpenedMsg{err: fmt.Errorf("open build URL: %w", err)}
		}
		return statusOpenedMsg{}
	}
}

func statusTUIPRLines(snapshot wtc.StatusSnapshot, showArchived bool) []string {
	if snapshot.ShowCollectionColumn {
		return nil
	}
	lines := []string{"", "PRs"}
	archived := 0
	for _, row := range snapshot.PRs {
		if row.Archived && !showArchived {
			archived++
			continue
		}
		label := row.Title
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
		lines = append(lines, fmt.Sprintf("%s #%s%s  %s", row.Repo, row.Number, state, label))
	}
	if len(snapshot.PRs) == 0 {
		lines = append(lines, "(none enlisted)")
	}
	if archived > 0 {
		lines = append(lines, fmt.Sprintf("%d archived PR(s) hidden; press a", archived))
	}
	for _, orphan := range snapshot.Orphans {
		lines = append(lines, fmt.Sprintf("⚠ %s on %s: PR %s; catch-up", orphan.Repo, orphan.Branch, orphan.State))
	}
	return lines
}

func (m statusTUIModel) contentLines() []string {
	width := m.width
	if width <= 0 {
		width = 80
	}
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
	}
	lines := []string{fmt.Sprintf("wtc status · %s · %s", name, age), ""}
	if m.showHelp {
		keys := "r refresh   a show/hide archived PRs   ? help   q quit"
		if m.procs {
			keys = "r refresh   ? help   q quit"
		}
		lines = append(lines, keys, "↑/↓ scroll   PgUp/PgDn scroll faster", "")
	}
	if m.procs {
		lines = append(lines, strings.Split(strings.TrimSuffix(wtc.StatusProcessesText(m.processes), "\n"), "\n")...)
	} else {
		lines = append(lines, statusTUIRepoLines(m.snapshot, width)...)
		if !m.reposOnly {
			lines = append(lines, statusTUIPRLines(m.snapshot, m.showArchived)...)
		}
		if m.snapshot.StaleCount > 0 {
			lines = append(lines, "", fmt.Sprintf("%d worktree(s) behind the development tip", m.snapshot.StaleCount))
		}
	}
	if m.errorText != "" {
		lines = append(lines, "", "warning: "+m.errorText)
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
	footer := "r refresh · a archived · ? help · q quit"
	if m.procs {
		footer = "r refresh · ? help · q quit"
	}
	if !m.nextRefresh.IsZero() && !m.refreshing {
		footer += fmt.Sprintf(" · next %ds", max(0, int(time.Until(m.nextRefresh).Seconds())))
	}
	bodyHeight := max(1, height-2)
	scroll := m.effectiveScroll()
	end := min(len(lines), scroll+bodyHeight)
	var b strings.Builder
	for _, line := range lines[scroll:end] {
		b.WriteString(statusTUIFit(line, width))
		b.WriteByte('\n')
	}
	for i := end - scroll; i < bodyHeight; i++ {
		b.WriteByte('\n')
	}
	b.WriteString(statusTUIFit(footer, width))
	view := tea.NewView(b.String())
	view.AltScreen = true
	view.ReportFocus = true
	if !m.noClick && statusTUIBuildColumns(m.snapshot) {
		view.MouseMode = tea.MouseModeCellMotion
	}
	return view
}

func runStatusTUI(c *wtc.Context, all, procs, reposOnly, noFetch, noClick bool, interval, background time.Duration) error {
	if !term.IsTerminal(os.Stdin.Fd()) || !term.IsTerminal(os.Stdout.Fd()) {
		return fmt.Errorf("status TUI requires a terminal")
	}
	model := statusTUIModel{context: c, all: all, procs: procs, reposOnly: reposOnly, noFetch: noFetch, noClick: noClick, interval: interval,
		background: background, focused: true, refreshing: true}
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
