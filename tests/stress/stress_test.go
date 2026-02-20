package stress

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"gelato/internal/config"
	"gelato/internal/engine"
	"gelato/internal/model"
	"gelato/internal/server"
	"gelato/internal/testutil"
)

func TestStressHighVolumeIngestion(t *testing.T) {
	limits := config.DefaultLimits()
	limits.QueueSizePerShard = 256
	limits.PerSourceBufferLines = 8192
	limits.GlobalBufferLines = 32768

	eng := engine.New(limits)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = eng.Run(ctx)
	}()

	srv := server.New(limits, eng.EventShards())
	dropSink := eng.DropSink()
	srv.SetStatsSink(func(update server.EngineStatsUpdate) {
		if update.Dropped > 0 {
			dropSink(update)
		}
	})

	listener := model.Listener{ID: "stress-1", BindHost: "127.0.0.1", Port: 0}
	info, err := srv.Start(ctx, listener)
	if err != nil {
		t.Fatalf("start listener: %v", err)
	}
	defer func() {
		_ = srv.Stop(listener.ID)
	}()

	senders := 4
	linesPerSender := 200
	var wg sync.WaitGroup
	for i := 0; i < senders; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			sender, err := testutil.NewTCPSender(info.Address)
			if err != nil {
				t.Errorf("dial sender %d: %v", id, err)
				return
			}
			defer sender.Close()
			for j := 0; j < linesPerSender; j++ {
				line := fmt.Sprintf("INFO sender=%d idx=%d payload", id, j)
				if err := sender.SendLine(line); err != nil {
					t.Errorf("send line: %v", err)
					return
				}
			}
		}(i)
	}
	wg.Wait()

	expectedLines := senders * linesPerSender
	reqCh := eng.SnapshotReqCh()
	respCh := eng.SnapshotRespCh()
	deadline := time.Now().Add(2 * time.Second)
	var snap engine.Snapshot
	for {
		reqCh <- engine.SnapshotRequest{View: "dashboard"}
		select {
		case snap = <-respCh:
			if len(snap.Lines) >= expectedLines {
				goto done
			}
		case <-time.After(50 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d lines (have %d)", expectedLines, len(snap.Lines))
		}
	}

done:
	if snap.Dropped != 0 {
		t.Fatalf("expected 0 dropped events, got %d", snap.Dropped)
	}
}
