package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"gelato/internal/app"
	"gelato/internal/config"
	"gelato/internal/web"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "gelato",
		Short: "Gelato is a local web log aggregator",
		RunE: func(cmd *cobra.Command, args []string) error {
			webHost, _ := cmd.Flags().GetString("web-host")
			webPort, _ := cmd.Flags().GetInt("web-port")
			assetsDir, _ := cmd.Flags().GetString("assets-dir")
			wsFlushMS, _ := cmd.Flags().GetInt("ws-flush-ms")
			wsMaxLogBatch, _ := cmd.Flags().GetInt("ws-max-log-batch")
			wsSnapshotSecs, _ := cmd.Flags().GetInt("ws-full-snapshot-sec")
			uiJournalCap, _ := cmd.Flags().GetInt("ui-journal-cap")

			limits := config.DefaultLimits()
			if uiJournalCap > 0 {
				// Placeholder flag preserved for future engine journal optimization.
				_ = uiJournalCap
			}

			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			rt := app.New(limits)
			rt.Start(ctx)

			webServer := web.New(rt, web.Options{
				Host:           webHost,
				Port:           webPort,
				AssetsDir:      assetsDir,
				WSFlushMS:      wsFlushMS,
				WSMaxLogBatch:  wsMaxLogBatch,
				WSSnapshotSecs: wsSnapshotSecs,
			})

			fmt.Fprintf(os.Stdout, "gelato web UI: %s\n", webServer.Addr())
			fmt.Fprintln(os.Stdout, "websocket: /api/v1/ws")
			fmt.Fprintln(os.Stdout, "api snapshot: /api/v1/snapshot")

			err := webServer.Start(ctx)
			_ = rt.Close()
			if err != nil {
				return err
			}
			return nil
		},
	}

	rootCmd.Version = fmt.Sprintf("%s (commit %s, built %s)", version, commit, date)
	rootCmd.SetVersionTemplate("{{.Version}}\n")

	rootCmd.Flags().String("web-host", "127.0.0.1", "Host interface for the local web server")
	rootCmd.Flags().Int("web-port", 8080, "Port for the local web server")
	rootCmd.Flags().String("assets-dir", "", "Optional path to built frontend assets directory (defaults to web/dist if present)")
	rootCmd.Flags().Int("ws-flush-ms", 100, "WebSocket broadcast flush interval in milliseconds")
	rootCmd.Flags().Int("ws-max-log-batch", 2000, "Maximum new log lines sent in a single delta frame")
	rootCmd.Flags().Int("ws-full-snapshot-sec", 5, "Periodic full snapshot interval in seconds")
	rootCmd.Flags().Int("ui-journal-cap", 65536, "Reserved for future engine journal capacity tuning")

	if err := rootCmd.Execute(); err != nil {
		log.Println(err)
		os.Exit(1)
	}
}
