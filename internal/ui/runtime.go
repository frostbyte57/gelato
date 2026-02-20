package ui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/termenv"

	"gelato/internal/config"
	"gelato/internal/engine"
	"gelato/internal/model"
	"gelato/internal/server"
)

func Run(noColor bool, statePath string) error {
	limits := config.DefaultLimits()
	eng := engine.New(limits)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if !noColor {
		prevBg := termenv.BackgroundColor()
		termenv.SetBackgroundColor(termenv.RGBColor(baseBGHex))
		if prevBg != nil {
			defer termenv.SetBackgroundColor(prevBg)
		}
	}

	srv := server.New(limits, eng.EventShards())
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
				out[id] = engine.ListenerStats{ActiveConns: item.ActiveConns, Dropped: item.Dropped, Rejected: item.Rejected}
			}
			return out
		},
		func() map[string]engine.SourceStats {
			stats := srv.SourceStats()
			out := make(map[string]engine.SourceStats, len(stats))
			for id, item := range stats {
				out[id] = engine.SourceStats{ActiveConns: item.ActiveConns, Dropped: item.Dropped, Errors: item.Errors}
			}
			return out
		},
	)

	go func() {
		_ = eng.Run(ctx)
	}()

	p := tea.NewProgram(New(noColor, statePath, eng, srv, cancel, limits), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
