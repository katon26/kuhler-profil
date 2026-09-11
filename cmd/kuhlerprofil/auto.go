package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newAutoCmd(opts Options) *cobra.Command {
	var profileFlag string

	autoCmd := &cobra.Command{
		Use:     "auto [on|off]",
		Aliases: []string{"governor"},
		Short:   "Enable or disable automatic thermal fan curve governor",
		Long:    "Query or configure the automated dynamic fan curve governor in kuhlerprofild.",
		Example: `  kuhlerprofil auto
  kuhlerprofil auto on
  kuhlerprofil auto on --profile quiet
  kp auto off`,
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
				if telem.ActiveCurveProfile != "" {
					fmt.Fprintf(opts.getOut(), "Active Curve Profile: %s\n", telem.ActiveCurveProfile)
				}
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
				return fmt.Errorf("failed to set auto governor: kuhlerprofild daemon is not running (required for auto-governor): %w", clientErr)
			}

			if profileFlag != "" {
				if err := client.SetCurveProfile(profileFlag); err != nil {
					return fmt.Errorf("failed to set curve profile %q: %w", profileFlag, err)
				}
			}

			if err := client.SetAutoMode(enabled); err != nil {
				return fmt.Errorf("failed to set auto governor over D-Bus: %w", err)
			}

			if enabled {
				if profileFlag != "" {
					fmt.Fprintf(opts.getOut(), "Auto-curve governor enabled with profile %q\n", profileFlag)
				} else {
					fmt.Fprintln(opts.getOut(), "Auto-curve governor enabled")
				}
			} else {
				fmt.Fprintln(opts.getOut(), "Auto-curve governor disabled")
			}
			return nil
		},
	}

	autoCmd.Flags().StringVarP(&profileFlag, "profile", "p", "", "Set curve profile (quiet, balanced, aggressive) when enabling auto governor")

	return autoCmd
}
