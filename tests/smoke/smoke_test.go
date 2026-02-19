package smoke

import (
	"context"
	"testing"
	"time"

	"gelato/internal/config"
	"gelato/internal/engine"
	"gelato/internal/model"
	"gelato/internal/server"
	"gelato/internal/testutil"
)

func TestEngineServerSmoke(t *testing.T) {
	limits := config.DefaultLimits()
	limits.QueueSizePerShard = 64
	limits.GlobalBufferLines = 200
	limits.PerSourceBufferLines = 50

	eng := engine.New(limits)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = eng.Run(ctx)
	}()

	srv := server.New(limits, eng.EventShards())
	listener := model.Listener{ID: "smoke-1", BindHost: "127.0.0.1", Port: 0}
	info, err := srv.Start(ctx, listener)
	if err != nil {
		t.Fatalf("start listener: %v", err)
	}

	sender, err := testutil.NewTCPSender(info.Address)
	if err != nil {
		t.Fatalf("dial listener: %v", err)
	}
	if err := sender.SendLine("hello world"); err != nil {
		t.Fatalf("send line: %v", err)
	}
	_ = sender.Close()

	reqCh := eng.SnapshotReqCh()
	respCh := eng.SnapshotRespCh()
	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		reqCh <- engine.SnapshotRequest{View: "dashboard"}
		select {
		case snap := <-respCh:
			if len(snap.Lines) > 0 {
				goto done
			}
		case <-time.After(50 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected lines in snapshot")
		}
	}

done:

	if err := srv.Stop(listener.ID); err != nil {
		t.Fatalf("stop listener: %v", err)
	}

	select {
	case <-ctx.Done():
	case <-time.After(50 * time.Millisecond):
	}
}
