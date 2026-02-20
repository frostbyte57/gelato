package server

import (
	"context"
	"fmt"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"gelato/internal/config"
	"gelato/internal/model"
)

func TestDetectLevel(t *testing.T) {
	cases := map[string]model.Level{
		"error occurred":     model.LevelError,
		" slow ERR path ":    model.LevelError,
		"warn: latency":      model.LevelWarn,
		"info boot":          model.LevelInfo,
		"debug trace":        model.LevelDebug,
		"plain message":      model.LevelUnknown,
		"[unknown severity]": model.LevelUnknown,
	}
	for input, expected := range cases {
		if got := detectLevel(input); got != expected {
			t.Fatalf("detectLevel(%q) = %v, want %v", input, got, expected)
		}
	}
}

func TestServerReadLoopDropsWhenQueueFull(t *testing.T) {
	limits := config.DefaultLimits()
	limits.MaxLineBytes = 256
	eventCh := []chan<- model.LogEvent{make(chan model.LogEvent)}
	srv := New(limits, eventCh)

	var dropCount uint64
	srv.SetStatsSink(func(update EngineStatsUpdate) {
		if update.Dropped > 0 {
			atomic.AddUint64(&dropCount, update.Dropped)
		}
	})

	connWriter, connReader := net.Pipe()
	defer connWriter.Close()
	defer connReader.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	state := &listenerState{id: "listener-1", ctx: ctx, cancel: cancel}

	done := make(chan struct{})
	go func() {
		srv.readLoop(state, connReader)
		close(done)
	}()

	go func() {
		for i := 0; i < 5; i++ {
			_, _ = fmt.Fprintf(connWriter, "ERROR burst %d\n", i)
		}
		_ = connWriter.Close()
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("readLoop did not finish in time")
	}

	if got := atomic.LoadUint64(&state.dropped); got < 5 {
		t.Fatalf("expected at least 5 drops recorded, got %d", got)
	}
	if dropCount < 5 {
		t.Fatalf("expected at least 5 drop updates, got %d", dropCount)
	}
}
