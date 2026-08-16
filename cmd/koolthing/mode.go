package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"koolthing/pkg/models"
)

func newModeCmd(opts Options) *cobra.Command {
	modeCmd := &cobra.Command{
		Use:     "mode [silent|standard|boost]",
		Aliases: []string{"profile", "thermal"},
		Short:   "Get or set ASUS thermal profile",
		Long:    "Query or switch the active ASUS platform thermal profile mode (silent, standard, or boost).",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				telem, err := fetchTelemetry(opts)
				if err != nil {
					return err
				}
				fmt.Fprintf(opts.getOut(), "Current thermal mode: %s\n", telem.ActiveMode)
				return nil
			}

			mode, err := models.ParseThermalMode(args[0])
			if err != nil {
				return fmt.Errorf("invalid thermal mode %q (valid: silent, standard, boost)", args[0])
			}

			// Try D-Bus first
			client, clientErr := opts.getClient()
			if clientErr == nil && client != nil {
				if setErr := client.SetThermalMode(string(mode)); setErr == nil {
					fmt.Fprintf(opts.getOut(), "Thermal mode set to: %s\n", mode)
					return nil
				}
			}

			// Fallback to direct driver write
			drv := opts.getDriver()
			if drvErr := drv.SetThermalMode(mode); drvErr == nil {
				fmt.Fprintf(opts.getOut(), "Thermal mode set to: %s (direct driver)\n", mode)
				return nil
			} else {
				return fmt.Errorf("failed to set thermal mode: koolthingd daemon is not running and direct driver write failed: %w", drvErr)
			}
		},
	}

	return modeCmd
}
