package ui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

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
