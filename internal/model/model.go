package model

import "time"

type Level int

const (
	LevelUnknown Level = iota
	LevelDebug
	LevelInfo
	LevelWarn
	LevelError
)

const (
	LevelMaskDebug uint32 = 1 << iota
	LevelMaskInfo
	LevelMaskWarn
	LevelMaskError
	LevelMaskAll = LevelMaskDebug | LevelMaskInfo | LevelMaskWarn | LevelMaskError
)

type LogEvent struct {
	ID         uint64
	SourceKey  string
	ListenerID string
	ConnID     string
	RemoteAddr string
	ReceivedAt time.Time
	Line       string
	Level      Level
	ParsedTS   *time.Time
	IngestLag  *time.Duration
}

type ListenerStatus int

const (
	ListenerStarting ListenerStatus = iota
	ListenerListening
	ListenerError
	ListenerStopped
)

type Listener struct {
	ID          string
	BindHost    string
	Port        int
	Status      ListenerStatus
	ErrMsg      string
	ActiveConns int
	Dropped     uint64
	Errors      uint64
	StartedAt   time.Time
}

type Source struct {
	Key         string
	Name        string
	Color       string
	Muted       bool
	LastSeen    time.Time
	ActiveConns int
	Dropped     uint64
	Errors      uint64
}
