// Package cli holds the cobra commands for the locci binary.
package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/MikeTeddyOmondi/locci-docker/internal/docker"
)

// Execute runs the root command.
func Execute() {
	root := &cobra.Command{
		Use:          "locci",
		Short:        "Locci container manager: API server, web UI and CLI in one binary",
		SilenceUsage: true,
	}

	var output string
	root.PersistentFlags().StringVarP(&output, "output", "o", "table", "output format: table or json")
	root.PersistentFlags().StringVarP(&envFlag, "env", "E", "", "environment (Docker host) to use; default: LOCCI_ENV, then `locci env use`, then the server default")
	root.PersistentPreRun = func(cmd *cobra.Command, args []string) { outputJSON = output == "json" }

	root.AddGroup(
		&cobra.Group{ID: "server", Title: "Server:"},
		&cobra.Group{ID: "client", Title: "Client:"},
	)

	for _, c := range []*cobra.Command{serveCmd(), userCmd()} {
		c.GroupID = "server"
		root.AddCommand(c)
	}

	client := []*cobra.Command{
		loginCmd(), logoutCmd(), whoamiCmd(), envCmd(),
		psCmd(), runCmd(), rmCmd(), logsCmd(), inspectCmd(), statsCmd(), execCmd(),
		imageCmd(), networkCmd(), volumeCmd(), infoCmd(), auditCmd(),
	}
	for _, action := range docker.Actions {
		client = append(client, actionCmd(action))
	}
	for _, c := range client {
		c.GroupID = "client"
		root.AddCommand(c)
	}

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
