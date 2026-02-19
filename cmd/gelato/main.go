package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"gelato/internal/ui"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "gelato",
		Short: "Gelato is a TUI log aggregator",
		RunE: func(cmd *cobra.Command, args []string) error {
			noColor, _ := cmd.Flags().GetBool("no-color")
			statePath, _ := cmd.Flags().GetString("state")
			return ui.Run(noColor, statePath)
		},
	}

	rootCmd.Version = fmt.Sprintf("%s (commit %s, built %s)", version, commit, date)
	rootCmd.SetVersionTemplate("{{.Version}}\n")

	rootCmd.Flags().Bool("no-color", false, "Disable color output")
	rootCmd.Flags().String("state", "", "Path to state file")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
