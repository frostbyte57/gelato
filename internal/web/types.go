package web

import (
	"time"

	"gelato/internal/engine"
	"gelato/internal/model"
)

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type CommandResultDTO struct {
	OK       bool            `json:"ok"`
	Listener *model.Listener `json:"listener,omitempty"`
	Error    *APIError       `json:"error,omitempty"`
}

type FiltersDTO struct {
	LevelMask  uint32 `json:"levelMask"`
	SearchText string `json:"searchText"`
	SourceKey  string `json:"sourceKey"`
}

type CreateListenerRequest struct {
	BindHost string `json:"bindHost"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
	Name     string `json:"name,omitempty"`
}

type UpdateSourceRequest struct {
	Name  *string `json:"name,omitempty"`
	Color *string `json:"color,omitempty"`
}

type LogLineDTO struct {
	Line  string      `json:"line"`
	Level model.Level `json:"level"`
}

type ListenerPatchDTO struct {
	Listener model.Listener       `json:"listener"`
	Stats    engine.ListenerStats `json:"stats"`
}

type SourcePatchDTO struct {
	Source model.Source       `json:"source"`
	Stats  engine.SourceStats `json:"stats"`
}

type DeltaStatsDTO struct {
	Stats              engine.StatsSnapshot `json:"stats"`
	Dropped            uint64               `json:"dropped"`
	Errors             uint64               `json:"errors"`
	OverflowSources    uint64               `json:"overflowSources"`
	ShardDrops         []uint64             `json:"shardDrops"`
	ListenerErrorRates map[string][]uint64  `json:"listenerErrorRates"`
	ListenerDropRates  map[string][]uint64  `json:"listenerDropRates"`
	SourceErrorRates   map[string][]uint64  `json:"sourceErrorRates"`
	SourceDropRates    map[string][]uint64  `json:"sourceDropRates"`
}

type DeltaFlags struct {
	FiltersChanged bool `json:"filtersChanged,omitempty"`
	Overflow       bool `json:"overflow,omitempty"`
}

type WSHelloFrame struct {
	Type            string    `json:"type"`
	Version         string    `json:"version"`
	ServerTime      time.Time `json:"serverTime"`
	FlushMS         int       `json:"flushMs"`
	MaxLogBatch     int       `json:"maxLogBatch"`
	FullSnapshotSec int       `json:"fullSnapshotSec"`
}

type WSSnapshotFrame struct {
	Type     string          `json:"type"`
	Seq      uint64          `json:"seq"`
	Reason   string          `json:"reason"`
	Snapshot engine.Snapshot `json:"snapshot"`
}

type WSDeltaFrame struct {
	Type            string             `json:"type"`
	FromSeq         uint64             `json:"fromSeq"`
	ToSeq           uint64             `json:"toSeq"`
	NewLogs         []LogLineDTO       `json:"newLogs,omitempty"`
	ListenerPatches []ListenerPatchDTO `json:"listenerPatches,omitempty"`
	SourcePatches   []SourcePatchDTO   `json:"sourcePatches,omitempty"`
	Stats           DeltaStatsDTO      `json:"stats"`
	Flags           DeltaFlags         `json:"flags,omitempty"`
	TruncateHint    *int               `json:"truncateHint,omitempty"`
}

type WSResyncRequiredFrame struct {
	Type       string `json:"type"`
	FromSeq    uint64 `json:"fromSeq"`
	CurrentSeq uint64 `json:"currentSeq"`
	Reason     string `json:"reason"`
}

type WSErrorFrame struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
}
