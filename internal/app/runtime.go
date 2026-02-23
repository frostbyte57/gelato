package app

import (
	"context"
	"sync"

	"gelato/internal/config"
	"gelato/internal/engine"
	"gelato/internal/model"
	"gelato/internal/server"
)

type Runtime struct {
	Limits config.Limits
	Engine *engine.Engine
	Server *server.Server

	ctx    context.Context
	cancel context.CancelFunc
	done   chan error

	snapshotMu sync.Mutex
}

func New(limits config.Limits) *Runtime {
	eng := engine.New(limits)
	srv := server.New(limits, eng.EventShards())
	rt := &Runtime{
		Limits: limits,
		Engine: eng,
		Server: srv,
		done:   make(chan error, 1),
	}

	srv.SetStatsSink(func(update server.EngineStatsUpdate) {
		if update.Dropped > 0 {
			eng.DropSink()(update)
		}
	})

	eng.AttachListenerHandlers(
		func(ctx context.Context, listener model.Listener) (string, error) {
			info, err := srv.Start(ctx, listener)
			if err != nil {
				return "", err
			}
			return info.Address, nil
		},
		func(ctx context.Context, id string) error {
			return srv.Stop(id)
		},
		func() map[string]engine.ListenerStats {
			stats := srv.ListenerStats()
			out := make(map[string]engine.ListenerStats, len(stats))
			for id, item := range stats {
				out[id] = engine.ListenerStats{
					ActiveConns: item.ActiveConns,
					Dropped:     item.Dropped,
					Rejected:    item.Rejected,
				}
			}
			return out
		},
		func() map[string]engine.SourceStats {
			stats := srv.SourceStats()
			out := make(map[string]engine.SourceStats, len(stats))
			for id, item := range stats {
				out[id] = engine.SourceStats{
					ActiveConns: item.ActiveConns,
					Dropped:     item.Dropped,
					Errors:      item.Errors,
				}
			}
			return out
		},
	)

	return rt
}

func (r *Runtime) Start(parent context.Context) {
	if parent == nil {
		parent = context.Background()
	}
	r.ctx, r.cancel = context.WithCancel(parent)
	go func() {
		r.done <- r.Engine.Run(r.ctx)
	}()
}

func (r *Runtime) Close() error {
	if r.cancel != nil {
		r.cancel()
	}
	select {
	case err := <-r.done:
		return err
	default:
		return nil
	}
}

func (r *Runtime) Context() context.Context {
	return r.ctx
}

func (r *Runtime) Snapshot(view string) engine.Snapshot {
	r.snapshotMu.Lock()
	defer r.snapshotMu.Unlock()

	if view == "" {
		view = "active"
	}
	r.Engine.SnapshotReqCh() <- engine.SnapshotRequest{View: view}
	return <-r.Engine.SnapshotRespCh()
}
