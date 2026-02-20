package engine

import (
	"context"
	"errors"
	"net"
	"strconv"
	"time"

	"gelato/internal/model"
)

func (e *Engine) handleCommand(cmd Command) error {
	switch cmd.Type {
	case CommandAddListener:
		return e.addListener(cmd)
	case CommandRemoveListener:
		return e.removeListener(cmd)
	case CommandRetryListener:
		return e.retryListener(cmd)
	case CommandSetFilters:
		e.filters = cmd.Filters
		return nil
	case CommandRenameSource:
		return e.renameSource(cmd)
	case CommandRecolorSource:
		return e.recolorSource(cmd)
	case CommandToggleMuteSource:
		return e.toggleMuteSource(cmd)
	case CommandClearSource:
		return e.clearSource(cmd)
	default:
		return errors.New("unknown command")
	}
}

func (e *Engine) addListener(cmd Command) error {
	if e.startListenerFn == nil {
		return errors.New("listener handler not attached")
	}
	id := cmd.ListenerID
	if id == "" {
		id = fmtListenerID(cmd.BindHost, cmd.Port)
	}
	listener := model.Listener{
		ID:        id,
		BindHost:  cmd.BindHost,
		Port:      cmd.Port,
		Status:    model.ListenerStarting,
		StartedAt: time.Now(),
	}
	e.listeners[id] = listener

	addr, err := e.startListenerFn(context.Background(), listener)
	if err != nil {
		listener.Status = model.ListenerError
		listener.ErrMsg = err.Error()
		e.listeners[id] = listener
		sendResult(cmd, listener, err)
		return err
	}

	host, port := splitAddress(addr)
	listener.BindHost = host
	listener.Port = port
	listener.Status = model.ListenerListening
	listener.ErrMsg = ""
	e.listeners[id] = listener
	sendResult(cmd, listener, nil)
	return nil
}

func (e *Engine) removeListener(cmd Command) error {
	listener, ok := e.listeners[cmd.ListenerID]
	if !ok {
		return errors.New("listener not found")
	}
	if e.stopListenerFn == nil {
		return errors.New("listener handler not attached")
	}

	err := e.stopListenerFn(context.Background(), cmd.ListenerID)
	if err != nil {
		listener.Status = model.ListenerError
		listener.ErrMsg = err.Error()
		e.listeners[cmd.ListenerID] = listener
		sendResult(cmd, listener, err)
		return err
	}
	listener.Status = model.ListenerStopped
	listener.ErrMsg = ""
	e.listeners[cmd.ListenerID] = listener
	sendResult(cmd, listener, nil)
	return nil
}

func (e *Engine) retryListener(cmd Command) error {
	listener, ok := e.listeners[cmd.ListenerID]
	if !ok {
		return errors.New("listener not found")
	}
	if e.startListenerFn == nil {
		return errors.New("listener handler not attached")
	}

	listener.Status = model.ListenerStarting
	listener.ErrMsg = ""
	e.listeners[cmd.ListenerID] = listener

	addr, err := e.startListenerFn(context.Background(), listener)
	if err != nil {
		listener.Status = model.ListenerError
		listener.ErrMsg = err.Error()
		e.listeners[cmd.ListenerID] = listener
		sendResult(cmd, listener, err)
		return err
	}

	host, port := splitAddress(addr)
	listener.BindHost = host
	listener.Port = port
	listener.Status = model.ListenerListening
	listener.ErrMsg = ""
	e.listeners[cmd.ListenerID] = listener
	sendResult(cmd, listener, nil)
	return nil
}

func (e *Engine) renameSource(cmd Command) error {
	if cmd.SourceKey == "" {
		return errors.New("source key required")
	}
	source := e.sources[cmd.SourceKey]
	if source == nil {
		return errors.New("source not found")
	}
	source.Name = cmd.Value
	return nil
}

func (e *Engine) recolorSource(cmd Command) error {
	if cmd.SourceKey == "" {
		return errors.New("source key required")
	}
	source := e.sources[cmd.SourceKey]
	if source == nil {
		return errors.New("source not found")
	}
	source.Color = cmd.Value
	return nil
}

func (e *Engine) toggleMuteSource(cmd Command) error {
	if cmd.SourceKey == "" {
		return errors.New("source key required")
	}
	source := e.sources[cmd.SourceKey]
	if source == nil {
		return errors.New("source not found")
	}
	source.Muted = !source.Muted
	return nil
}

func (e *Engine) clearSource(cmd Command) error {
	if cmd.SourceKey == "" {
		return errors.New("source key required")
	}
	buf := e.perSource[cmd.SourceKey]
	if buf != nil {
		buf.Clear()
	}
	return nil
}

func splitAddress(addr string) (string, int) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return addr, 0
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return host, 0
	}
	return host, port
}

func fmtListenerID(host string, port int) string {
	if port == 0 {
		return host
	}
	return host + ":" + strconv.Itoa(port)
}

func sendResult(cmd Command, listener model.Listener, err error) {
	if cmd.RespCh == nil {
		return
	}
	cmd.RespCh <- CommandResult{Listener: listener, Err: err}
}
