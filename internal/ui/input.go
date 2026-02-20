package ui

import (
	"net"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

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
