package ui

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"gelato/internal/config"
	"gelato/internal/engine"
	"gelato/internal/model"
	"gelato/internal/server"
)

type tickMsg time.Time

type snapshotMsg engine.Snapshot

type cmdResultMsg engine.CommandResult

type Model struct {
	noColor      bool
	statePath    string
	engine       *engine.Engine
	server       *server.Server
	cancel       context.CancelFunc
	limits       config.Limits
	snapshot     engine.Snapshot
	selected     int
	lastErr      string
	width        int
	height       int
	addMode      bool
	input        string
	view         string
	follow       bool
	scroll       int
	searchMode   bool
	searchInput  string
	filterText   string
	levelMask    uint32
	sourceFilter string
	sourceIndex  int

	statsCache statsView
}

func New(
	noColor bool,
	statePath string,
	engine *engine.Engine,
	server *server.Server,
	cancel context.CancelFunc,
	limits config.Limits,
) Model {
	return Model{
		noColor:   noColor,
		statePath: statePath,
		engine:    engine,
		server:    server,
		cancel:    cancel,
		limits:    limits,
		view:      "listeners",
		follow:    true,
		levelMask: model.LevelMaskAll,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(requestSnapshot(m.engine), scheduleTick())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = typed.Width
		m.height = typed.Height
		return m, nil
	case tickMsg:
		return m, tea.Batch(requestSnapshot(m.engine), scheduleTick())
	case snapshotMsg:
		m.snapshot = engine.Snapshot(typed)
		if m.selected >= len(m.snapshot.Listeners) {
			m.selected = max(0, len(m.snapshot.Listeners)-1)
		}
		m.statsCache.invalidate()
		return m, nil
	case cmdResultMsg:
		result := engine.CommandResult(typed)
		if result.Err != nil {
			m.lastErr = result.Err.Error()
		} else {
			m.lastErr = ""
		}
		return m, requestSnapshot(m.engine)
	case tea.KeyMsg:
		if m.addMode {
			return handleAddInput(m, typed)
		}
		if m.searchMode {
			return handleSearchInput(m, typed)
		}
		switch typed.String() {
		case "ctrl+c", "q":
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		case "up", "k":
			if m.view == "dashboard" {
				m.follow = false
				m.scroll = min(m.scroll+1, max(0, len(m.snapshot.Lines)-1))
				return m, nil
			}
			if m.view == "sources" {
				if m.sourceIndex > 0 {
					m.sourceIndex--
				}
				return m, nil
			}
			if m.selected > 0 {
				m.selected--
			}
			return m, nil
		case "down", "j":
			if m.view == "dashboard" {
				m.scroll = max(0, m.scroll-1)
				if m.scroll == 0 {
					m.follow = true
				}
				return m, nil
			}
			if m.view == "sources" {
				if m.sourceIndex < len(m.snapshot.Sources)-1 {
					m.sourceIndex++
				}
				return m, nil
			}
			if m.selected < len(m.snapshot.Listeners)-1 {
				m.selected++
			}
			return m, nil
		case "f":
			if m.view == "dashboard" {
				m.follow = !m.follow
				if m.follow {
					m.scroll = 0
				}
			}
			return m, nil
		case "1":
			if m.view == "dashboard" {
				return m, toggleLevel(m, model.LevelMaskInfo)
			}
			return m, nil
		case "2":
			if m.view == "dashboard" {
				return m, toggleLevel(m, model.LevelMaskWarn)
			}
			return m, nil
		case "3":
			if m.view == "dashboard" {
				return m, toggleLevel(m, model.LevelMaskError)
			}
			return m, nil
		case "4":
			if m.view == "dashboard" {
				return m, toggleLevel(m, model.LevelMaskDebug)
			}
			return m, nil
		case "/":
			if m.view == "dashboard" {
				m.searchMode = true
				m.searchInput = m.filterText
			}
			return m, nil
		case "c":
			if m.view == "dashboard" {
				m.filterText = ""
				m.searchInput = ""
				return m, sendFilterUpdate(m)
			}
			return m, nil
		case "tab":
			m.view = nextView(m.view)
			return m, requestSnapshot(m.engine)
		case "enter":
			if m.view == "sources" {
				return m, toggleSourceFocus(m)
			}
			return m, nil
		case "ctrl+a":
			if m.view == "listeners" {
				m.addMode = true
				m.input = ""
				m.lastErr = ""
			}
			return m, nil
		case "ctrl+d":
			if m.view == "listeners" {
				return m, sendRemoveListener(m)
			}
			return m, nil
		case "ctrl+r":
			if m.view == "listeners" {
				return m, sendRetryListener(m)
			}
			return m, nil
		case "m":
			if m.view == "sources" {
				return m, sendToggleMute(m)
			}
			return m, nil
		case "x":
			if m.view == "sources" {
				return m, sendClearSource(m)
			}
			return m, nil

		default:
			return m, nil
		}
	default:
		return m, nil
	}
}

type statsView struct {
	valid   bool
	logs    string
	errors  string
	drops   string
	summary string
	legend  string
}

func (s *statsView) invalidate() {
	s.valid = false
}

func (s *statsView) ensure(snapshot engine.Snapshot) {
	if s.valid {
		return
	}
	logs := sparkline(snapshot.Stats.LogsPerSec)
	errors := sparkline(snapshot.Stats.ErrorsPerMin)
	drops := sparkline(snapshot.Stats.DropsPerMin)
	s.logs = "logs/s  " + logs
	s.errors = "errors  " + errors
	s.drops = "drops   " + drops
	s.summary = fmt.Sprintf("totals: logs=%d errors=%d drops=%d", sum(snapshot.Stats.LogsPerSec), snapshot.Errors, snapshot.Dropped)
	s.legend = "legend: / search  c clear  1 info 2 warn 3 error 4 debug  f follow"
	s.valid = true
}

func (m Model) View() string {
	styles := buildStyles(m.noColor)
	width := normalizeWidth(m.width)

	logo := renderLogo(width, styles)
	chrome := styles.bar.Width(width).Render(renderHeaderMeta(styles))

	sidebarWidth := 26
	inspectorWidth := 30
	if width < 110 {
		inspectorWidth = 26
	}
	if width < 90 {
		sidebarWidth = 22
		inspectorWidth = 22
	}
	mainWidth := max(20, width-sidebarWidth-inspectorWidth-2)

	sidebar := styles.sidebar.Width(sidebarWidth).Render(renderSidebar(m, styles))
	main := styles.panel.Width(mainWidth).Render(renderMain(m))
	inspector := styles.panel.Width(inspectorWidth).Render(renderInspector(m, styles))
	body := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, " ", main, " ", inspector)

	status := styles.status.Width(width).Render(renderStatus(m, styles))

	layout := lipgloss.JoinVertical(lipgloss.Left, logo, chrome, body, status)
	return styles.canvas.Render(layout)
}

type uiStyles struct {
	header    lipgloss.Style
	meta      lipgloss.Style
	bar       lipgloss.Style
	panel     lipgloss.Style
	sidebar   lipgloss.Style
	status    lipgloss.Style
	active    lipgloss.Style
	inactive  lipgloss.Style
	muted     lipgloss.Style
	accent    lipgloss.Style
	warn      lipgloss.Style
	good      lipgloss.Style
	bad       lipgloss.Style
	canvas    lipgloss.Style
	logo      lipgloss.Style
	chipLabel lipgloss.Style
	chipValue lipgloss.Style
}

func buildStyles(noColor bool) uiStyles {
	if noColor {
		border := lipgloss.Border{
			Top: "-", Bottom: "-", Left: "|", Right: "|",
			TopLeft: "+", TopRight: "+", BottomLeft: "+", BottomRight: "+",
		}
		return uiStyles{
			header:    lipgloss.NewStyle().Bold(true),
			meta:      lipgloss.NewStyle(),
			bar:       lipgloss.NewStyle().Padding(0, 1).Border(border),
			panel:     lipgloss.NewStyle().Padding(1, 1).Border(border),
			sidebar:   lipgloss.NewStyle().Padding(1, 1).Border(border),
			status:    lipgloss.NewStyle().Padding(0, 1).Border(border),
			active:    lipgloss.NewStyle().Bold(true),
			inactive:  lipgloss.NewStyle(),
			muted:     lipgloss.NewStyle(),
			accent:    lipgloss.NewStyle(),
			warn:      lipgloss.NewStyle(),
			good:      lipgloss.NewStyle(),
			bad:       lipgloss.NewStyle(),
			canvas:    lipgloss.NewStyle(),
			logo:      lipgloss.NewStyle(),
			chipLabel: lipgloss.NewStyle(),
			chipValue: lipgloss.NewStyle(),
		}
	}

	border := lipgloss.Border{
		Top: "─", Bottom: "─", Left: "│", Right: "│",
		TopLeft: "╭", TopRight: "╮", BottomLeft: "╰", BottomRight: "╯",
	}

	primary := lipgloss.Color("#C084FC")
	secondary := lipgloss.Color("#22D3EE")
	accentAlt := lipgloss.Color("#F472B6")
	muted := lipgloss.Color("#94A3B8")
	bg := lipgloss.Color("#05060A")
	panelBg := lipgloss.Color("#0F172A")
	sidebarBg := lipgloss.Color("#0C1222")
	statusBg := lipgloss.Color("#090F1C")
	good := lipgloss.Color("#10B981")
	warn := lipgloss.Color("#FBBF24")
	bad := lipgloss.Color("#F87171")

	return uiStyles{
		header:    lipgloss.NewStyle().Bold(true).Foreground(secondary),
		meta:      lipgloss.NewStyle().Foreground(muted),
		bar:       lipgloss.NewStyle().Padding(0, 2).Border(border).BorderForeground(primary).Background(lipgloss.Color("#111A2B")),
		panel:     lipgloss.NewStyle().Padding(1, 2).Border(border).BorderForeground(primary).Background(panelBg),
		sidebar:   lipgloss.NewStyle().Padding(1, 2).Border(border).BorderForeground(secondary).Background(sidebarBg),
		status:    lipgloss.NewStyle().Padding(0, 2).Border(border).BorderForeground(secondary).Background(statusBg),
		active:    lipgloss.NewStyle().Bold(true).Foreground(secondary).Background(lipgloss.Color("#1F1B4B")).Padding(0, 1),
		inactive:  lipgloss.NewStyle().Foreground(muted),
		muted:     lipgloss.NewStyle().Foreground(muted),
		accent:    lipgloss.NewStyle().Foreground(accentAlt).Bold(true),
		warn:      lipgloss.NewStyle().Foreground(warn).Bold(true),
		good:      lipgloss.NewStyle().Foreground(good).Bold(true),
		bad:       lipgloss.NewStyle().Foreground(bad).Bold(true),
		canvas:    lipgloss.NewStyle().Foreground(lipgloss.Color("#E2E8F0")).Background(bg).Padding(1, 2),
		logo:      lipgloss.NewStyle().Background(bg).Padding(1, 2),
		chipLabel: lipgloss.NewStyle().Foreground(muted).Bold(true),
		chipValue: lipgloss.NewStyle().Foreground(secondary),
	}
}

func normalizeWidth(width int) int {
	if width < 60 {
		return 80
	}
	return width
}

func renderLogo(width int, styles uiStyles) string {
	logo := []string{
		" ██████╗ ███████╗██╗      █████╗ ████████╗ ██████╗ ",
		"██╔════╝ ██╔════╝██║     ██╔══██╗╚══██╔══╝██╔═══██╗",
		"██║  ███╗█████╗  ██║     ███████║   ██║   ██║   ██║",
		"██║   ██║██╔══╝  ██║     ██╔══██║   ██║   ██║   ██║",
		"╚██████╔╝███████╗███████╗██║  ██║   ██║   ╚██████╔╝",
		" ╚═════╝ ╚══════╝╚══════╝╚═╝  ╚═╝   ╚═╝    ╚═════╝ ",
		"        multi-scoop log gelateria terminal         ",
	}
	colored := make([]string, 0, len(logo))
	for _, line := range logo {
		colored = append(colored, rainbow(line))
	}
	return styles.logo.Width(width).Render(lipgloss.JoinVertical(lipgloss.Left, colored...))
}

func renderHeaderMeta(styles uiStyles) string {
	left := styles.header.Render(rainbow("Gelato"))
	right := styles.meta.Render("multi-listener log console · tcp ingestion")
	return lipgloss.JoinHorizontal(lipgloss.Left, left, "  ", right)
}

func rainbow(text string) string {
	colors := []lipgloss.Color{
		lipgloss.Color("#F472B6"),
		lipgloss.Color("#C084FC"),
		lipgloss.Color("#60A5FA"),
		lipgloss.Color("#34D399"),
		lipgloss.Color("#FBBF24"),
		lipgloss.Color("#F97316"),
	}
	var out strings.Builder
	colorIndex := 0
	for _, r := range text {
		if r == ' ' {
			out.WriteRune(r)
			continue
		}
		style := lipgloss.NewStyle().Foreground(colors[colorIndex%len(colors)]).Bold(true)
		out.WriteString(style.Render(string(r)))
		colorIndex++
	}
	return out.String()
}

func renderSidebar(m Model, styles uiStyles) string {
	lines := []string{
		styles.accent.Render("VIEWS"),
		viewLine(m, styles, "dashboard"),
		viewLine(m, styles, "listeners"),
		viewLine(m, styles, "sources"),
		viewLine(m, styles, "filters"),
		"",
		styles.accent.Render("FILTERS"),
		chipLine(styles, "search", emptyDash(m.filterText)),
		chipLine(styles, "source", emptyDash(m.sourceFilter)),
		chipLine(styles, "levels", levelLabel(m.levelMask)),
		"",
		styles.accent.Render("STATS"),
		styles.good.Render(fmt.Sprintf("logs/s: %d", lastValue(m.snapshot.Stats.LogsPerSec))),
		styles.warn.Render(fmt.Sprintf("errors: %d", lastValue(m.snapshot.Stats.ErrorsPerMin))),
		styles.bad.Render(fmt.Sprintf("drops: %d", lastValue(m.snapshot.Stats.DropsPerMin))),
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func renderMain(m Model) string {
	return renderViewContent(m)
}

func renderInspector(m Model, styles uiStyles) string {
	lines := []string{styles.accent.Render("INSPECT")}
	if m.view == "listeners" {
		listener, ok := selectedListener(m)
		if !ok {
			return lipgloss.JoinVertical(lipgloss.Left, append(lines, "No listener selected")...)
		}
		stats := m.snapshot.ListenerStats[listener.ID]
		lines = append(lines,
			"id: "+listener.ID,
			"status: "+formatStatus(listener),
			fmt.Sprintf("conns: %d", stats.ActiveConns),
			fmt.Sprintf("errors: %d", stats.Errors),
			fmt.Sprintf("drops: %d", stats.Dropped),
			fmt.Sprintf("rejects: %d", stats.Rejected),
			"",
			"errRate: "+formatRate(m.snapshot.ListenerErrorRates[listener.ID]),
			"dropRate: "+formatRate(m.snapshot.ListenerDropRates[listener.ID]),
		)
		return lipgloss.JoinVertical(lipgloss.Left, lines...)
	}
	if m.view == "sources" {
		source, ok := selectedSource(m)
		if !ok {
			return lipgloss.JoinVertical(lipgloss.Left, append(lines, "No source selected")...)
		}
		lines = append(lines,
			"key: "+source.Key,
			"name: "+emptyDash(source.Name),
			fmt.Sprintf("conns: %d", source.ActiveConns),
			fmt.Sprintf("errors: %d", source.Errors),
			fmt.Sprintf("drops: %d", source.Dropped),
			"",
			"errRate: "+formatRate(m.snapshot.SourceErrorRates[source.Key]),
			"dropRate: "+formatRate(m.snapshot.SourceDropRates[source.Key]),
		)
		return lipgloss.JoinVertical(lipgloss.Left, lines...)
	}
	lines = append(lines, "No details")
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func viewLine(m Model, styles uiStyles, view string) string {
	label := strings.ToUpper(view)
	if m.view == view {
		return styles.active.Render("[" + label + "]")
	}
	return styles.inactive.Render(" " + label + " ")
}

func lastValue(values []uint64) uint64 {
	if len(values) == 0 {
		return 0
	}
	return values[len(values)-1]
}

func renderTabs(m Model, styles uiStyles) string {
	views := []string{"dashboard", "listeners", "sources", "filters"}
	labels := make([]string, 0, len(views))
	for _, view := range views {
		label := strings.ToUpper(view)
		if m.view == view {
			labels = append(labels, styles.active.Render("["+label+"]"))
		} else {
			labels = append(labels, styles.inactive.Render(" "+label+" "))
		}
	}
	return strings.Join(labels, " ")
}

func renderViewContent(m Model) string {
	lines := []string{}
	if m.lastErr != "" {
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("error: "+m.lastErr), "")
	}
	if m.searchMode {
		prompt := lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Render("Filter logs: ")
		lines = append(lines, prompt+m.searchInput)
		return lipgloss.JoinVertical(lipgloss.Left, lines...)
	}
	if m.addMode {
		prompt := lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Render("Add listener (host:port): ")
		lines = append(lines, prompt+m.input)
		return lipgloss.JoinVertical(lipgloss.Left, lines...)
	}

	switch m.view {
	case "dashboard":
		lines = append(lines, renderDashboard(m)...)
	case "listeners":
		lines = append(lines, renderListeners(m)...)
	case "sources":
		lines = append(lines, renderSources(m)...)
	case "filters":
		lines = append(lines, renderFilters(m)...)
	default:
		lines = append(lines, "Unknown view.")
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func renderStatus(m Model, styles uiStyles) string {
	key := func(combo, desc string) string {
		return styles.accent.Render(combo) + styles.muted.Render(" "+desc)
	}
	parts := []string{
		key("tab", "views"),
		key("/", "filter"),
		key("c", "clear"),
		key("1-4", "levels"),
		key("ctrl+a", "add"),
		key("ctrl+d", "remove"),
		key("ctrl+r", "retry"),
		key("f", "follow"),
		key("q", "quit"),
	}
	if m.view == "sources" {
		parts = append(parts, key("m", "mute"), key("x", "purge"))
	}
	status := strings.Join(parts, "  ")
	status += styles.muted.Render("  | view: " + strings.ToUpper(m.view))
	if m.sourceFilter != "" {
		status += styles.muted.Render("  | source: " + m.sourceFilter)
	}
	if m.filterText != "" {
		status += styles.muted.Render("  | filter: " + m.filterText)
	}
	if m.view == "dashboard" && !m.follow {
		status += styles.warn.Render("  | paused")
	}
	return status
}

func Run(noColor bool, statePath string) error {
	limits := config.DefaultLimits()
	eng := engine.New(limits)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := server.New(limits, eng.EventShards())
	srv.SetStatsSink(func(update server.EngineStatsUpdate) {
		if update.Dropped > 0 {
			eng.DropSink()(update)
		}
	})
	eng.AttachListenerHandlers(
		func(ctx context.Context, listener model.Listener) (string, error) {
			info, err := srv.Start(ctx, listener)
			if err != nil {
				return "", err
			}
			return info.Address, nil
		},
		func(ctx context.Context, id string) error {
			return srv.Stop(id)
		},
		func() map[string]engine.ListenerStats {
			stats := srv.ListenerStats()
			out := make(map[string]engine.ListenerStats, len(stats))
			for id, item := range stats {
				out[id] = engine.ListenerStats{ActiveConns: item.ActiveConns, Dropped: item.Dropped, Rejected: item.Rejected}
			}
			return out
		},
		func() map[string]engine.SourceStats {
			stats := srv.SourceStats()
			out := make(map[string]engine.SourceStats, len(stats))
			for id, item := range stats {
				out[id] = engine.SourceStats{ActiveConns: item.ActiveConns, Dropped: item.Dropped, Errors: item.Errors}
			}
			return out
		},
	)

	go func() {
		_ = eng.Run(ctx)
	}()

	p := tea.NewProgram(New(noColor, statePath, eng, srv, cancel, limits), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func sendAddListener(m Model, host string, port int) tea.Cmd {
	respCh := make(chan engine.CommandResult, 1)
	m.engine.UICmdCh() <- engine.Command{
		Type:     engine.CommandAddListener,
		BindHost: host,
		Port:     port,
		RespCh:   respCh,
	}
	return waitForCommandResult(respCh)
}

func sendRemoveListener(m Model) tea.Cmd {
	listener, ok := selectedListener(m)
	if !ok {
		return nil
	}
	respCh := make(chan engine.CommandResult, 1)
	m.engine.UICmdCh() <- engine.Command{
		Type:       engine.CommandRemoveListener,
		ListenerID: listener.ID,
		RespCh:     respCh,
	}
	return waitForCommandResult(respCh)
}

func sendRetryListener(m Model) tea.Cmd {
	listener, ok := selectedListener(m)
	if !ok {
		return nil
	}
	respCh := make(chan engine.CommandResult, 1)
	m.engine.UICmdCh() <- engine.Command{
		Type:       engine.CommandRetryListener,
		ListenerID: listener.ID,
		RespCh:     respCh,
	}
	return waitForCommandResult(respCh)
}

func sendFilterUpdate(m Model) tea.Cmd {
	respCh := make(chan engine.CommandResult, 1)
	m.engine.UICmdCh() <- engine.Command{
		Type:    engine.CommandSetFilters,
		Filters: engine.Filters{LevelMask: m.levelMask, SearchText: m.filterText, SourceKey: m.sourceFilter},
		RespCh:  respCh,
	}
	return waitForCommandResult(respCh)
}

func sendToggleMute(m Model) tea.Cmd {
	source, ok := selectedSource(m)
	if !ok {
		return nil
	}
	respCh := make(chan engine.CommandResult, 1)
	m.engine.UICmdCh() <- engine.Command{
		Type:      engine.CommandToggleMuteSource,
		SourceKey: source.Key,
		RespCh:    respCh,
	}
	return waitForCommandResult(respCh)
}

func sendClearSource(m Model) tea.Cmd {
	source, ok := selectedSource(m)
	if !ok {
		return nil
	}
	respCh := make(chan engine.CommandResult, 1)
	m.engine.UICmdCh() <- engine.Command{
		Type:      engine.CommandClearSource,
		SourceKey: source.Key,
		RespCh:    respCh,
	}
	return waitForCommandResult(respCh)
}

func selectedListener(m Model) (model.Listener, bool) {
	if len(m.snapshot.Listeners) == 0 || m.selected < 0 || m.selected >= len(m.snapshot.Listeners) {
		return model.Listener{}, false
	}
	return m.snapshot.Listeners[m.selected], true
}

func selectedSource(m Model) (model.Source, bool) {
	if len(m.snapshot.Sources) == 0 || m.sourceIndex < 0 || m.sourceIndex >= len(m.snapshot.Sources) {
		return model.Source{}, false
	}
	return m.snapshot.Sources[m.sourceIndex], true
}

func requestSnapshot(eng *engine.Engine) tea.Cmd {
	return func() tea.Msg {
		eng.SnapshotReqCh() <- engine.SnapshotRequest{View: "active"}
		return snapshotMsg(<-eng.SnapshotRespCh())
	}
}

func handleAddInput(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		host, port, err := parseHostPort(m.input, m.limits.DefaultBindHost, m.limits.DefaultPort)
		if err != nil {
			m.lastErr = err.Error()
			return m, nil
		}
		m.addMode = false
		m.input = ""
		return m, sendAddListener(m, host, port)
	case "esc":
		m.addMode = false
		m.input = ""
		return m, nil
	case "backspace":
		if len(m.input) > 0 {
			m.input = m.input[:len(m.input)-1]
		}
		return m, nil
	default:
		if msg.Type == tea.KeyRunes {
			m.input += string(msg.Runes)
		}
		return m, nil
	}
}

func handleSearchInput(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.searchMode = false
		m.filterText = strings.TrimSpace(m.searchInput)
		m.searchInput = ""
		m.scroll = 0
		m.follow = true
		return m, sendFilterUpdate(m)
	case "esc":
		m.searchMode = false
		m.searchInput = ""
		return m, nil
	case "backspace":
		if len(m.searchInput) > 0 {
			m.searchInput = m.searchInput[:len(m.searchInput)-1]
		}
		return m, nil
	default:
		if msg.Type == tea.KeyRunes {
			m.searchInput += string(msg.Runes)
		}
		return m, nil
	}
}

func parseHostPort(input, defaultHost string, defaultPort int) (string, int, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return defaultHost, defaultPort, nil
	}
	if strings.Contains(trimmed, ":") {
		host, portStr, err := net.SplitHostPort(trimmed)
		if err != nil {
			return "", 0, err
		}
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return "", 0, err
		}
		return host, port, nil
	}
	if port, err := strconv.Atoi(trimmed); err == nil {
		return defaultHost, port, nil
	}
	return trimmed, defaultPort, nil
}

func waitForCommandResult(ch <-chan engine.CommandResult) tea.Cmd {
	return func() tea.Msg {
		return cmdResultMsg(<-ch)
	}
}

func scheduleTick() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func renderListeners(m Model) []string {
	if len(m.snapshot.Listeners) == 0 {
		return []string{"No listeners running."}
	}

	lines := make([]string, 0, len(m.snapshot.Listeners))
	for i, listener := range m.snapshot.Listeners {
		cursor := " "
		if i == m.selected {
			cursor = ">"
		}
		status := formatStatus(listener)
		stats := m.snapshot.ListenerStats[listener.ID]
		rate := formatRate(m.snapshot.ListenerErrorRates[listener.ID])
		drops := formatRate(m.snapshot.ListenerDropRates[listener.ID])
		hostPort := fmt.Sprintf("%s:%d", listener.BindHost, listener.Port)
		badge := statusBadge(status)
		line := fmt.Sprintf(
			"%s %s %s conns=%4d dropped=%4d rejected=%4d errors=%4d errRate=%6s dropRate=%6s",
			cursor,
			padRight(hostPort, 22),
			padRight(badge, 6),
			stats.ActiveConns,
			stats.Dropped,
			stats.Rejected,
			stats.Errors,
			rate,
			drops,
		)
		if listener.ErrMsg != "" {
			line = fmt.Sprintf("%s (%s)", line, listener.ErrMsg)
		}
		line = styleByErrors(line, stats.Errors)
		line = highlightActive(line, i == m.selected)
		lines = append(lines, line)
	}
	return lines
}

func renderDashboard(m Model) []string {
	header := "Recent logs"
	if m.filterText != "" {
		header += " [filter: " + m.filterText + "]"
	}
	if m.sourceFilter != "" {
		header += " [source: " + m.sourceFilter + "]"
	}
	levelLabel := levelLabel(m.levelMask)
	if levelLabel != "all" {
		header += " [levels: " + levelLabel + "]"
	}
	if !m.follow {
		header += " (paused)"
	}
	lines := []string{header}
	lines = append(lines, renderKpis(m)...)
	statsLines := renderStats(m)
	lines = append(lines, statsLines...)
	logLines := m.snapshot.Lines
	if len(logLines) == 0 {
		if m.filterText != "" {
			return append(lines, "(no matching logs)")
		}
		return append(lines, "(no logs yet)")
	}

	viewport := m.height - 5 - len(statsLines)
	if viewport < 5 {
		viewport = 5
	}

	if m.follow {
		m.scroll = 0
	}

	start := max(0, len(logLines)-viewport-m.scroll)
	end := min(len(logLines), start+viewport)
	levels := m.snapshot.LineLevels
	levelSlice := sliceLevels(levels, start, end)
	lines = append(lines, "TIME     LEVEL   MESSAGE")
	lines = append(lines, "----------------------------------------")
	for i, line := range logLines[start:end] {
		lines = append(lines, formatLogLine(levelSlice, i, line))
	}

	if m.scroll > 0 {
		lines = append(lines, fmt.Sprintf("... %d lines above", m.scroll))
	}
	return lines
}

func renderStats(m Model) []string {
	m.statsCache.ensure(m.snapshot)
	return []string{
		m.statsCache.logs,
		m.statsCache.errors,
		m.statsCache.drops,
		m.statsCache.summary,
		m.statsCache.legend,
	}
}

func renderKpis(m Model) []string {
	logVal := fmt.Sprintf("%d", lastValue(m.snapshot.Stats.LogsPerSec))
	errVal := fmt.Sprintf("%d", lastValue(m.snapshot.Stats.ErrorsPerMin))
	dropVal := fmt.Sprintf("%d", lastValue(m.snapshot.Stats.DropsPerMin))

	logs := chip("LOGS/S", logVal)
	errors := chip("ERRORS", errVal)
	drops := chip("DROPS", dropVal)
	return []string{lipgloss.JoinHorizontal(lipgloss.Left, logs, " ", errors, " ", drops)}
}

func chip(label, value string) string {
	labelStyled := lipgloss.NewStyle().Foreground(lipgloss.Color("#C084FC")).Bold(true).Render(label)
	valueStyled := lipgloss.NewStyle().Foreground(lipgloss.Color("#FDE68A")).Bold(true).Render(value)
	return lipgloss.NewStyle().
		Padding(0, 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#7C3AED")).
		Background(lipgloss.Color("#1B1037")).
		Render(labelStyled + " " + valueStyled)
}

func sparkline(values []uint64) string {
	if len(values) == 0 {
		return "(no data)"
	}
	maxVal := uint64(1)
	for _, val := range values {
		if val > maxVal {
			maxVal = val
		}
	}
	blocks := []rune(" .:-=+*#%@")
	out := make([]rune, len(values))
	for i, val := range values {
		idx := int((float64(val) / float64(maxVal)) * float64(len(blocks)-1))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(blocks) {
			idx = len(blocks) - 1
		}
		out[i] = blocks[idx]
	}
	return string(out)
}

func renderSources(m Model) []string {
	if len(m.snapshot.Sources) == 0 {
		return []string{"No sources detected."}
	}
	lines := []string{"Sources (enter to focus):"}
	limit := 20
	if len(m.snapshot.Sources) < limit {
		limit = len(m.snapshot.Sources)
	}
	for i := 0; i < limit; i++ {
		source := m.snapshot.Sources[i]
		cursor := " "
		if i == m.sourceIndex {
			cursor = ">"
		}
		if m.sourceFilter == source.Key {
			cursor = "*"
		}
		rate := formatRate(m.snapshot.SourceErrorRates[source.Key])
		dropRate := formatRate(m.snapshot.SourceDropRates[source.Key])
		badge := sourceBadge(source.Errors)
		line := fmt.Sprintf(
			"%s %s %s conns=%4d dropped=%4d errors=%4d errRate=%6s dropRate=%6s",
			cursor,
			padRight(source.Key, 22),
			padRight(badge, 6),
			source.ActiveConns,
			source.Dropped,
			source.Errors,
			rate,
			dropRate,
		)
		line = styleByErrors(line, source.Errors)
		line = highlightActive(line, i == m.sourceIndex && m.view == "sources")
		if m.sourceFilter == source.Key {
			line = lipgloss.NewStyle().Underline(true).Render(line)
		}
		lines = append(lines, line)
	}
	if len(m.snapshot.Sources) > limit {
		lines = append(lines, fmt.Sprintf("... %d more", len(m.snapshot.Sources)-limit))
	}
	if m.sourceFilter != "" {
		lines = append(lines, "Focused source active. Press enter to clear.")
	}
	return lines
}

func renderFilters(m Model) []string {
	lines := []string{"Filters:"}
	lines = append(lines, fmt.Sprintf("search: %s", m.filterText))
	lines = append(lines, fmt.Sprintf("source: %s", emptyDash(m.sourceFilter)))
	lines = append(lines, fmt.Sprintf("levels: %s", levelLabel(m.levelMask)))
	lines = append(lines, fmt.Sprintf("shard drops: %s", formatShardDrops(m.snapshot.ShardDrops)))
	lines = append(lines, "Keys:")
	lines = append(lines, "- / to set search")
	lines = append(lines, "- c to clear search")
	lines = append(lines, "- 1 info  2 warn  3 error  4 debug")
	lines = append(lines, "- enter in Sources to focus")
	lines = append(lines, "- m mute/unmute  x clear buffer")
	return lines
}

func emptyDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func chipLine(styles uiStyles, label, value string) string {
	left := styles.chipLabel.Render(label + ":")
	right := styles.chipValue.Render(value)
	return left + " " + right
}

var (
	selectedRowStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#1E1B4B")).
				Foreground(lipgloss.Color("#F8FAFC")).
				Bold(true)
	errorRowStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#F87171"))
)

func highlightActive(line string, active bool) string {
	if !active {
		return line
	}
	return selectedRowStyle.Render(line)
}

func formatRate(values []uint64) string {
	if len(values) == 0 {
		return "-"
	}
	last := values[len(values)-1]
	return fmt.Sprintf("%d/m", last)
}

func styleByErrors(line string, errors uint64) string {
	if errors == 0 {
		return line
	}
	return errorRowStyle.Render(line)
}

func statusBadge(status string) string {
	switch status {
	case "listening":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#10B981")).Render("[ok]")
	case "starting":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#FBBF24")).Render("[boot]")
	case "error":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#F87171")).Render("[err]")
	case "stopped":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#94A3B8")).Render("[stop]")
	default:
		return "[?]"
	}
}

func sourceBadge(errors uint64) string {
	if errors == 0 {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#10B981")).Render("[ok]")
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#F87171")).Render("[err]")
}

func sum(values []uint64) uint64 {
	var total uint64
	for _, val := range values {
		total += val
	}
	return total
}

func formatShardDrops(values []uint64) string {
	if len(values) == 0 {
		return "-"
	}
	parts := make([]string, len(values))
	for i, val := range values {
		parts[i] = fmt.Sprintf("%d", val)
	}
	return strings.Join(parts, ",")
}

func padRight(text string, width int) string {
	if width <= 0 {
		return text
	}
	length := lipgloss.Width(text)
	if length >= width {
		return text
	}
	return text + strings.Repeat(" ", width-length)
}

func sliceLevels(levels []model.Level, start, end int) []model.Level {
	if len(levels) == 0 {
		return nil
	}
	if start < 0 {
		start = 0
	}
	if end > len(levels) {
		end = len(levels)
	}
	if start >= end {
		return nil
	}
	return levels[start:end]
}

func formatLogLine(levels []model.Level, idx int, line string) string {
	prefix := "--:--:--"
	if idx < len(levels) {
		switch levels[idx] {
		case model.LevelError:
			return levelStyle("#F87171").Render(prefix+" [ERROR]") + " " + line
		case model.LevelWarn:
			return levelStyle("#FBBF24").Render(prefix+" [WARN]") + " " + line
		case model.LevelInfo:
			return levelStyle("#60A5FA").Render(prefix+" [INFO]") + " " + line
		case model.LevelDebug:
			return levelStyle("#94A3B8").Render(prefix+" [DEBUG]") + " " + line
		default:
			return prefix + " [----] " + line
		}
	}
	return prefix + " [----] " + line
}

func levelStyle(color string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Bold(true)
}

func formatStatus(listener model.Listener) string {
	switch listener.Status {
	case model.ListenerStarting:
		return "starting"
	case model.ListenerListening:
		return "listening"
	case model.ListenerError:
		return "error"
	case model.ListenerStopped:
		return "stopped"
	default:
		return "unknown"
	}
}

func nextView(current string) string {
	views := []string{"dashboard", "listeners", "sources", "filters"}
	for i, view := range views {
		if view == current {
			return views[(i+1)%len(views)]
		}
	}
	return views[0]
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func filterLines(lines []string, filter string) []string {
	if filter == "" {
		return lines
	}
	filtered := make([]string, 0, len(lines))
	needle := strings.ToLower(filter)
	for _, line := range lines {
		if strings.Contains(strings.ToLower(line), needle) {
			filtered = append(filtered, line)
		}
	}
	return filtered
}

func toggleLevel(m Model, mask uint32) tea.Cmd {
	if m.levelMask == model.LevelMaskAll {
		m.levelMask = 0
	}
	if m.levelMask&mask != 0 {
		m.levelMask &^= mask
	} else {
		m.levelMask |= mask
	}
	if m.levelMask == 0 {
		m.levelMask = model.LevelMaskAll
	}
	return sendFilterUpdate(m)
}

func levelLabel(mask uint32) string {
	if mask == model.LevelMaskAll {
		return "all"
	}
	labels := make([]string, 0, 4)
	if mask&model.LevelMaskInfo != 0 {
		labels = append(labels, "info")
	}
	if mask&model.LevelMaskWarn != 0 {
		labels = append(labels, "warn")
	}
	if mask&model.LevelMaskError != 0 {
		labels = append(labels, "error")
	}
	if mask&model.LevelMaskDebug != 0 {
		labels = append(labels, "debug")
	}
	if len(labels) == 0 {
		return "none"
	}
	return strings.Join(labels, ",")
}

func toggleSourceFocus(m Model) tea.Cmd {
	if len(m.snapshot.Sources) == 0 {
		return nil
	}
	if m.sourceFilter != "" {
		m.sourceFilter = ""
		return sendSourceFilterUpdate(m)
	}
	if m.sourceIndex < 0 || m.sourceIndex >= len(m.snapshot.Sources) {
		return nil
	}
	m.sourceFilter = m.snapshot.Sources[m.sourceIndex].Key
	return sendSourceFilterUpdate(m)
}

func sendSourceFilterUpdate(m Model) tea.Cmd {
	respCh := make(chan engine.CommandResult, 1)
	m.engine.UICmdCh() <- engine.Command{
		Type:    engine.CommandSetFilters,
		Filters: engine.Filters{LevelMask: m.levelMask, SearchText: m.filterText, SourceKey: m.sourceFilter},
		RespCh:  respCh,
	}
	return waitForCommandResult(respCh)
}
