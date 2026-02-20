package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) View() string {
	styles := buildStyles(m.noColor)
	width := normalizeWidth(m.width)
	canvasFrameWidth := styles.canvas.GetHorizontalFrameSize()
	contentWidth := width - canvasFrameWidth
	if contentWidth < 0 {
		contentWidth = 0
	}
	contentHeight := m.height - styles.canvas.GetVerticalFrameSize()
	if contentHeight < 0 {
		contentHeight = 0
	}

	innerWidth := func(outer int, style lipgloss.Style) int {
		return max(0, outer-style.GetHorizontalFrameSize())
	}

	logoWidth := innerWidth(contentWidth, styles.logo)
	logo := renderLogo(logoWidth, styles)
	barWidth := innerWidth(contentWidth, styles.bar)
	chromeContent := renderHeaderMeta(styles)
	if barWidth > 0 {
		chromeContent = lipgloss.NewStyle().MaxWidth(barWidth).Render(chromeContent)
	}
	chrome := styles.bar.Width(barWidth).Render(chromeContent)

	gapWidth := 1
	gap := styles.gap.Render(strings.Repeat(" ", gapWidth))
	available := contentWidth - (gapWidth * 2)
	if available < 0 {
		available = 0
	}

	sidebarOuter := 26
	inspectorOuter := 30
	if contentWidth < 110 {
		inspectorOuter = 26
	}
	if contentWidth < 90 {
		sidebarOuter = 22
		inspectorOuter = 22
	}

	sidebarMinOuter := styles.sidebar.GetHorizontalFrameSize() + 12
	inspectorMinOuter := styles.panel.GetHorizontalFrameSize() + 12
	mainMinOuter := styles.panel.GetHorizontalFrameSize() + 16

	useVertical := available < sidebarMinOuter+mainMinOuter+inspectorMinOuter
	var body string
	if useVertical {
		sidebar := styles.sidebar.Width(innerWidth(contentWidth, styles.sidebar)).Render(renderSidebar(m, styles))
		main := styles.panel.Width(innerWidth(contentWidth, styles.panel)).Render(renderMain(m))
		inspector := styles.panel.Width(innerWidth(contentWidth, styles.panel)).Render(renderInspector(m, styles))
		body = lipgloss.JoinVertical(lipgloss.Left, sidebar, main, inspector)
	} else {
		maxSidebarOuter := available - mainMinOuter - inspectorMinOuter
		if sidebarOuter > maxSidebarOuter {
			sidebarOuter = maxSidebarOuter
		}
		if sidebarOuter < sidebarMinOuter {
			sidebarOuter = sidebarMinOuter
		}
		maxInspectorOuter := available - mainMinOuter - sidebarOuter
		if inspectorOuter > maxInspectorOuter {
			inspectorOuter = maxInspectorOuter
		}
		if inspectorOuter < inspectorMinOuter {
			inspectorOuter = inspectorMinOuter
		}
		mainOuter := available - sidebarOuter - inspectorOuter

		sidebar := styles.sidebar.Width(innerWidth(sidebarOuter, styles.sidebar)).Render(renderSidebar(m, styles))
		main := styles.panel.Width(innerWidth(mainOuter, styles.panel)).Render(renderMain(m))
		inspector := styles.panel.Width(innerWidth(inspectorOuter, styles.panel)).Render(renderInspector(m, styles))
		body = lipgloss.JoinHorizontal(lipgloss.Top, sidebar, gap, main, gap, inspector)
	}
	body = lipgloss.NewStyle().Width(contentWidth).Render(body)

	statusWidth := innerWidth(contentWidth, styles.status)
	statusContent := renderStatus(m, styles)
	if statusWidth > 0 {
		statusContent = lipgloss.NewStyle().MaxWidth(statusWidth).Render(statusContent)
	}
	status := styles.status.Width(statusWidth).Render(statusContent)

	layout := lipgloss.JoinVertical(lipgloss.Left, logo, chrome, body, status)
	canvas := styles.canvas.Width(contentWidth)
	if m.height > 0 {
		canvas = canvas.Height(contentHeight)
	}
	return canvas.Render(layout)
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
	return strings.Join(labels, styles.gap.Render(" "))
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
	status := strings.Join(parts, styles.gap.Render("  "))
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
