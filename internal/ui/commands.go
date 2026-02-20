package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"gelato/internal/engine"
	"gelato/internal/model"
)

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

func sendSourceFilterUpdate(m Model) tea.Cmd {
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
