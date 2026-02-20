package unit

import (
	"strconv"
	"testing"
	"time"

	"gelato/internal/config"
	"gelato/internal/engine"
	"gelato/internal/model"
)

func newTestLimits() config.Limits {
	limits := config.DefaultLimits()
	limits.AsyncIngest = false
	limits.ShardCount = 2
	limits.QueueSizePerShard = 8
	limits.GlobalBufferLines = 64
	limits.PerSourceBufferLines = 64
	return limits
}

func TestEngineDrainEventsProcessesAllShards(t *testing.T) {
	eng := engine.New(newTestLimits())
	now := time.Now()

	for i, ch := range eng.EventShards() {
		ch <- model.LogEvent{SourceKey: "source-" + strconv.Itoa(i), Line: "line", Level: model.LevelInfo}
	}

	processed := eng.TestDrainEvents(now, 1)
	if processed != uint64(len(eng.EventShards())) {
		t.Fatalf("expected %d processed events, got %d", len(eng.EventShards()), processed)
	}
	lines, _ := eng.TestCollectLines()
	if len(lines) != len(eng.EventShards()) {
		t.Fatalf("expected %d lines in global buffer, got %d", len(eng.EventShards()), len(lines))
	}
}

func TestEngineCollectFilteredLines(t *testing.T) {
	eng := engine.New(newTestLimits())
	now := time.Now()
	eng.TestApplyEvent(model.LogEvent{SourceKey: "svc", Line: "INFO start up", Level: model.LevelInfo}, now)
	eng.TestApplyEvent(model.LogEvent{SourceKey: "svc", Line: "ERROR failed to bind", Level: model.LevelError}, now)

	eng.TestSetFilters(engine.Filters{LevelMask: model.LevelMaskError, SearchText: "failed"})
	lines, levels := eng.TestCollectFilteredLines()
	if len(lines) != 1 {
		t.Fatalf("expected 1 filtered line, got %d", len(lines))
	}
	if levels[0] != model.LevelError {
		t.Fatalf("expected error level, got %v", levels[0])
	}
	if lines[0] != "ERROR failed to bind" {
		t.Fatalf("unexpected line: %s", lines[0])
	}
}

func TestEngineFlushPendingLogs(t *testing.T) {
	eng := engine.New(newTestLimits())
	eng.TestSetPendingLogs(7)
	now := time.Now()
	eng.TestFlushPendingLogs(now)

	values := eng.TestLogsPerSecSnapshot(now)
	var total uint64
	for _, val := range values {
		total += val
	}
	if total != 7 {
		t.Fatalf("expected logs/sec to record 7, got %d", total)
	}
	if eng.TestPendingLogs() != 0 {
		t.Fatalf("expected pending logs cleared, got %d", eng.TestPendingLogs())
	}
}
