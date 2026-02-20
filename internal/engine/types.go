package engine

import (
	"gelato/internal/model"
)

type CommandType int

const (
	CommandAddListener CommandType = iota
	CommandRemoveListener
	CommandRetryListener
	CommandSetFilters
	CommandRenameSource
	CommandRecolorSource
	CommandToggleMuteSource
	CommandClearSource
)

type Command struct {
	Type       CommandType
	ListenerID string
	BindHost   string
	Port       int
	Filters    Filters
	SourceKey  string
	Value      string
	RespCh     chan CommandResult
}

type CommandResult struct {
	Listener model.Listener
	Err      error
}

type Filters struct {
	LevelMask  uint32
	SearchText string
	SourceKey  string
}

type SnapshotRequest struct {
	View string
}

type Snapshot struct {
	Listeners          []model.Listener
	ListenerStats      map[string]ListenerStats
	Sources            []model.Source
	SourceStats        map[string]SourceStats
	Lines              []string
	LineLevels         []model.Level
	Dropped            uint64
	Errors             uint64
	OverflowSources    uint64
	ShardDrops         []uint64
	SourceErrorRates   map[string][]uint64
	SourceDropRates    map[string][]uint64
	ListenerErrorRates map[string][]uint64
	ListenerDropRates  map[string][]uint64
	Stats              StatsSnapshot
}

type SourceStats struct {
	ActiveConns int
	Dropped     uint64
	Errors      uint64
}

type ListenerStats struct {
	ActiveConns int
	Dropped     uint64
	Rejected    uint64
	Errors      uint64
}

type StatsSnapshot struct {
	LogsPerSec   []uint64
	ErrorsPerMin []uint64
	DropsPerMin  []uint64
}
