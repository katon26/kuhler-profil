package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newAutoCmd(opts Options) *cobra.Command {
	autoCmd := &cobra.Command{
		Use:     "auto [on|off]",
		Aliases: []string{"governor", "curve"},
		Short:   "Enable or disable automatic thermal fan curve governor",
		Long:    "Query or configure the automated dynamic fan curve governor in koolthingd.",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				telem, err := fetchTelemetry(opts)
				if err != nil {
					return err
				}
				stateStr := "DISABLED"
				if telem.AutoMode {
					stateStr = "ENABLED"
				}
				fmt.Fprintf(opts.getOut(), "Auto-curve governor: %s\n", stateStr)
				return nil
			}

			var enabled bool
			switch strings.ToLower(strings.TrimSpace(args[0])) {
			case "on", "enable", "enabled", "true", "1":
				enabled = true
			case "off", "disable", "disabled", "false", "0":
				enabled = false
			default:
				return fmt.Errorf("invalid auto setting %q (expected 'on' or 'off')", args[0])
			}

			client, clientErr := opts.getClient()
			if clientErr != nil || client == nil {
				return fmt.Errorf("failed to set auto governor: koolthingd daemon is not running (required for auto-governor): %w", clientErr)
			}

			if err := client.SetAutoMode(enabled); err != nil {
				return fmt.Errorf("failed to set auto governor over D-Bus: %w", err)
			}

			if enabled {
				fmt.Fprintln(opts.getOut(), "Auto-curve governor enabled")
			} else {
				fmt.Fprintln(opts.getOut(), "Auto-curve governor disabled")
			}
			return nil
		},
	}

	return autoCmd
}
