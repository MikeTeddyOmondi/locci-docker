// Package cli holds the cobra commands for the locci binary.
package cli

import (
	"os"

	"github.com/spf13/cobra"
)

// Execute runs the root command.
func Execute() {
	root := &cobra.Command{
		Use:           "locci",
		Short:         "Locci container manager: API server, web UI and CLI in one binary",
		SilenceUsage:  true,
		SilenceErrors: false,
	}

	root.AddCommand(serveCmd())

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
