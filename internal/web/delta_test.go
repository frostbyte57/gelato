package web

import (
	"testing"

	"gelato/internal/engine"
	"gelato/internal/model"
)

func TestBuildDeltaAppendOnlyLogs(t *testing.T) {
	prev := engine.Snapshot{
		Lines:      []string{"a", "b"},
		LineLevels: []model.Level{model.LevelInfo, model.LevelWarn},
		Stats:      engine.StatsSnapshot{},
	}
	curr := engine.Snapshot{
		Lines:      []string{"a", "b", "c"},
		LineLevels: []model.Level{model.LevelInfo, model.LevelWarn, model.LevelError},
		Stats:      engine.StatsSnapshot{},
	}

	got := buildDelta(prev, curr, 1, 2, 2000)
	if got.needsSnapshot {
		t.Fatalf("expected delta, got needsSnapshot")
	}
	if got.frame.Type != "delta" {
		t.Fatalf("expected delta frame, got %q", got.frame.Type)
	}
	if len(got.frame.NewLogs) != 1 || got.frame.NewLogs[0].Line != "c" {
		t.Fatalf("unexpected new logs: %#v", got.frame.NewLogs)
	}
}

func TestBuildDeltaRequestsSnapshotOnNonAppend(t *testing.T) {
	prev := engine.Snapshot{Lines: []string{"a", "b"}}
	curr := engine.Snapshot{Lines: []string{"x", "b"}}

	got := buildDelta(prev, curr, 1, 2, 2000)
	if !got.needsSnapshot {
		t.Fatalf("expected snapshot resync")
	}
}
