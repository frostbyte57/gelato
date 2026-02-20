package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"gelato/internal/model"
)

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
