package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"gelato/internal/model"
)

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
	gap := lipgloss.NewStyle().Background(lipgloss.Color(baseBGHex)).Render(" ")
	return []string{lipgloss.JoinHorizontal(lipgloss.Left, logs, gap, errors, gap, drops)}
}

func chip(label, value string) string {
	bg := lipgloss.Color(baseBGHex)
	labelStyled := lipgloss.NewStyle().Foreground(lipgloss.Color("#C084FC")).Background(bg).Bold(true).Render(label)
	valueStyled := lipgloss.NewStyle().Foreground(lipgloss.Color("#FDE68A")).Background(bg).Bold(true).Render(value)
	spacer := lipgloss.NewStyle().Background(bg).Render(" ")
	return lipgloss.NewStyle().
		Padding(0, 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#7C3AED")).
		Background(bg).
		Render(labelStyled + spacer + valueStyled)
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
				Foreground(lipgloss.Color("#F8FAFC")).
				Bold(true).
				Underline(true)
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
