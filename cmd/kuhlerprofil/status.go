package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"kuhlerprofil/pkg/models"
)

func newStatusCmd(opts Options) *cobra.Command {
	var jsonOutput bool

	statusCmd := &cobra.Command{
		Use:     "status",
		Aliases: []string{"stat", "info"},
		Short:   "Show current thermal, fan, and battery status",
		Long:    "Query real-time hardware telemetry and daemon state including CPU temperature, fan RPMs, battery charge & limit, and active thermal profile.",
		Example: `  kuhlerprofil status
  kuhlerprofil status --json
  kp status -j`,
		RunE: func(cmd *cobra.Command, args []string) error {
			telem, err := fetchTelemetry(opts)
			if err != nil {
				return err
			}

			if jsonOutput {
				data, err := json.MarshalIndent(telem, "", "  ")
				if err != nil {
					return fmt.Errorf("failed to encode JSON telemetry: %w", err)
				}
				fmt.Fprintln(opts.getOut(), string(data))
				return nil
			}

			formatStatusHuman(opts, telem)
			return nil
		},
	}

	statusCmd.Flags().BoolVarP(&jsonOutput, "json", "j", false, "Output status telemetry in JSON format")

	return statusCmd
}

func fetchTelemetry(opts Options) (models.Telemetry, error) {
	var dbusErr error
	client, err := opts.getClient()
	if err == nil && client != nil {
		telem, err := client.GetStatus()
		if err == nil {
			return telem, nil
		}
		dbusErr = err
	} else {
		dbusErr = err
	}

	// Fallback to direct driver reading
	drv := opts.getDriver()
	telem, drvErr := drv.ReadTelemetry()
	if drvErr == nil {
		return telem, nil
	}

	return models.Telemetry{}, fmt.Errorf("failed to get status: kuhlerprofild daemon is not running and direct driver read failed: %w (D-Bus: %v)", drvErr, dbusErr)
}

func formatStatusHuman(opts Options, t models.Telemetry) {
	out := opts.getOut()

	modeStr := string(t.ActiveMode)
	if modeStr == "" {
		modeStr = "standard"
	}
	modeFormatted := strings.ToUpper(modeStr[:1]) + modeStr[1:]

	autoStr := "Disabled"
	if t.AutoMode {
		autoStr = "Enabled"
	}

	powerStr := "Battery (Discharging)"
	if t.OnAC {
		powerStr = "AC Power (Plugged in)"
	}

	fan2Str := fmt.Sprintf("%d RPM", t.Fan2RPM)
	if t.Fan2RPM <= 0 {
		fan2Str = "0 RPM (Stopped / Idle)"
	}

	fmt.Fprintln(out, "KühlerProfil System Status")
	fmt.Fprintln(out, "──────────────────────────────────────────────────")
	fmt.Fprintf(out, "  • Thermal Profile:   %s\n", modeFormatted)
	fmt.Fprintf(out, "  • Auto Governor:     %s\n", autoStr)
	fmt.Fprintf(out, "  • CPU Temperature:   %.1f°C\n", t.CPUTemp)
	fmt.Fprintf(out, "  • Fan 1 (CPU):       %d RPM\n", t.Fan1RPM)
	fmt.Fprintf(out, "  • Fan 2 (GPU):       %s\n", fan2Str)
	fmt.Fprintf(out, "  • Battery Level:     %d%%\n", t.BatteryPercent)
	fmt.Fprintf(out, "  • Charge Limit:      %d%%\n", t.BatteryLimit)
	fmt.Fprintf(out, "  • Power Source:      %s\n", powerStr)
	fmt.Fprintln(out, "──────────────────────────────────────────────────")
}
