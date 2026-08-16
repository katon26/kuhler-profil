package main

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"koolthing/pkg/models"
)

func newBatteryCmd(opts Options) *cobra.Command {
	batteryCmd := &cobra.Command{
		Use:     "battery [60|80|100]",
		Aliases: []string{"bat", "charge"},
		Short:   "Get or set battery health charging limit (60, 80, 100)",
		Long:    "Query or configure the ASUS Battery Care health threshold (60%, 80%, or 100%) to prolong battery lifespan.",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				telem, err := fetchTelemetry(opts)
				if err != nil {
					return err
				}
				fmt.Fprintf(opts.getOut(), "Battery charge limit: %d%% (Current level: %d%%, AC: %v)\n",
					telem.BatteryLimit, telem.BatteryPercent, telem.OnAC)
				return nil
			}

			return applyBatteryLimit(opts, args[0])
		},
	}

	limitSubCmd := &cobra.Command{
		Use:   "limit <60|80|100>",
		Short: "Set battery health charging threshold",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return applyBatteryLimit(opts, args[0])
		},
	}

	setSubCmd := &cobra.Command{
		Use:   "set <60|80|100>",
		Short: "Set battery health charging threshold",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return applyBatteryLimit(opts, args[0])
		},
	}

	batteryCmd.AddCommand(limitSubCmd)
	batteryCmd.AddCommand(setSubCmd)

	return batteryCmd
}

func applyBatteryLimit(opts Options, limitStr string) error {
	val, err := strconv.Atoi(limitStr)
	if err != nil {
		return fmt.Errorf("invalid battery limit %q (must be a number: 60, 80, or 100)", limitStr)
	}

	intLimit := int32(val)
	if err := models.ValidateBatteryLimit(intLimit); err != nil {
		return err
	}

	// Try D-Bus first
	client, clientErr := opts.getClient()
	if clientErr == nil && client != nil {
		if setErr := client.SetBatteryLimit(intLimit); setErr == nil {
			fmt.Fprintf(opts.getOut(), "Battery charge limit set to: %d%%\n", intLimit)
			return nil
		}
	}

	// Fallback to direct driver write
	drv := opts.getDriver()
	if drvErr := drv.SetBatteryLimit(intLimit); drvErr == nil {
		fmt.Fprintf(opts.getOut(), "Battery charge limit set to: %d%% (direct driver)\n", intLimit)
		return nil
	} else {
		return fmt.Errorf("failed to set battery limit: koolthingd daemon is not running and direct driver write failed: %w", drvErr)
	}
}
