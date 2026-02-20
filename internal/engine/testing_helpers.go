package engine

import (
	"time"

	"gelato/internal/model"
)

// TestDrainEvents exposes drainEvents for external tests.
func (e *Engine) TestDrainEvents(now time.Time, batchSize int) uint64 {
	return e.drainEvents(now, batchSize)
}

// TestCollectLines exposes the aggregated lines buffer for tests.
func (e *Engine) TestCollectLines() ([]string, []model.Level) {
	lines, levels := e.collectLines()
	return lines, convertLevels(levels)
}

// TestApplyEvent allows tests to inject events into the engine.
func (e *Engine) TestApplyEvent(evt model.LogEvent, now time.Time) {
	e.applyEvent(evt, now)
}

// TestSetFilters sets the filter state for testing.
func (e *Engine) TestSetFilters(filters Filters) {
	e.filters = filters
}

// TestCollectFilteredLines returns filtered lines for verification.
func (e *Engine) TestCollectFilteredLines() ([]string, []model.Level) {
	return e.collectFilteredLines()
}

// TestSetPendingLogs sets pending logs counter for testing.
func (e *Engine) TestSetPendingLogs(value uint64) {
	e.pendingLogs = value
}

// TestFlushPendingLogs flushes pending logs counter for tests.
func (e *Engine) TestFlushPendingLogs(now time.Time) {
	e.flushPendingLogs(now)
}

// TestPendingLogs returns pending log count for verification.
func (e *Engine) TestPendingLogs() uint64 {
	return e.pendingLogs
}

// TestLogsPerSecSnapshot exposes counters for assertions.
func (e *Engine) TestLogsPerSecSnapshot(now time.Time) []uint64 {
	return e.logsPerSec.Snapshot(now)
}
