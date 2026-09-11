package main

import (
	"github.com/spf13/cobra"
)

func newTUICmd(opts Options) *cobra.Command {
	tuiCmd := &cobra.Command{
		Use:     "tui",
		Aliases: []string{"dashboard", "ui", "top"},
		Short:   "Launch interactive terminal dashboard",
		Long:    "Launch the full-screen interactive Bubbletea TUI dashboard with live telemetry meters, thermal gauges, battery monitoring, and instant keybindings.",
		Example: `  kuhlerprofil tui
  kp tui`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _ := opts.getClient()
			return opts.startTUI(client)
		},
	}

	return tuiCmd
}
