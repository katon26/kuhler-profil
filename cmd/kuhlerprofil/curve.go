package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"kuhlerprofil/pkg/config"
	"kuhlerprofil/pkg/models"
)

func newCurveCmd(opts Options) *cobra.Command {
	curveCmd := &cobra.Command{
		Use:     "curve [list|set <profile>]",
		Aliases: []string{"curves", "profile"},
		Short:   "Query or configure fan curve profiles",
		Long:    "Query available temperature-to-RPM curve profiles, inspect active profile, or set a new curve profile.",
		Example: `  kuhlerprofil curve
  kuhlerprofil curve list
  kuhlerprofil curve set quiet
  kp curve set aggressive`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				var active string
				var hasHW bool

				client, err := opts.getClient()
				if err == nil && client != nil {
					active, _ = client.GetActiveCurveProfile()
					telem, _ := client.GetStatus()
					hasHW = telem.HasHardwareCurve
				}

				// Fallback to local config and direct driver probe if daemon is not running
				if active == "" {
					cfg, _ := config.Load("")
					active = cfg.ActiveCurveProfile
					if active == "" {
						active = "balanced"
					}
					driver := opts.getDriver()
					if driver != nil {
						caps := driver.GetHardwareCurveCaps()
						hasHW = caps.Supported
					}
				}

				hwStr := "Not supported on this model (Using software governor)"
				if hasHW {
					hwStr = "Supported (ASUS ACPI hardware curve registers)"
				}

				profileFormatted := active
				if len(active) > 0 {
					profileFormatted = strings.ToUpper(active[:1]) + active[1:]
				}

				fmt.Fprintf(opts.getOut(), "Active Curve Profile: %s\n", profileFormatted)
				fmt.Fprintf(opts.getOut(), "Hardware ACPI Curve:  %s\n", hwStr)
				return nil
			}

			switch strings.ToLower(strings.TrimSpace(args[0])) {
			case "list", "ls":
				var profiles map[string]models.CurveProfile
				var active string

				client, err := opts.getClient()
				if err == nil && client != nil {
					profiles, _ = client.GetCurveProfiles()
					active, _ = client.GetActiveCurveProfile()
				}

				// Fallback to local config if daemon is not running
				if len(profiles) == 0 {
					cfg, _ := config.Load("")
					profiles = cfg.Curves
					active = cfg.ActiveCurveProfile
					if active == "" {
						active = "balanced"
					}
				}

				fmt.Fprintln(opts.getOut(), "Available Fan Curve Profiles:")
				fmt.Fprintln(opts.getOut(), "──────────────────────────────────────────────────")

				var names []string
				for name := range profiles {
					names = append(names, name)
				}
				sort.Strings(names)

				for _, name := range names {
					p := profiles[name]
					marker := "  "
					if strings.EqualFold(name, active) {
						marker = "* "
					}
					desc := p.Description
					if desc == "" {
						desc = "Pre-configured thermal curve profile"
					}
					fmt.Fprintf(opts.getOut(), "%s%-12s - %s\n", marker, name, desc)
				}
				fmt.Fprintln(opts.getOut(), "──────────────────────────────────────────────────")
				return nil

			case "set":
				if len(args) < 2 {
					return fmt.Errorf("missing profile name (e.g. 'kuhlerprofil curve set quiet')")
				}
				targetProfile := strings.ToLower(strings.TrimSpace(args[1]))
				client, err := opts.getClient()
				if err != nil || client == nil {
					return fmt.Errorf("kuhlerprofild daemon is not running (required for curve profiles): %w\nStart the daemon with: sudo systemctl start kuhlerprofil.service", err)
				}
				if err := client.SetCurveProfile(targetProfile); err != nil {
					return fmt.Errorf("failed to set curve profile to %q: %w", targetProfile, err)
				}
				_ = client.SetAutoMode(true)
				telem, _ := client.GetStatus()
				modeStr := ""
				if telem.ActiveMode != "" {
					modeStr = fmt.Sprintf(" -> %s mode", strings.Title(string(telem.ActiveMode)))
				}
				fmt.Fprintf(opts.getOut(), "✔ Curve profile set to %q (Auto Governor active%s)\n", targetProfile, modeStr)
				return nil

			default:
				targetProfile := strings.ToLower(strings.TrimSpace(args[0]))
				client, err := opts.getClient()
				if err != nil || client == nil {
					return fmt.Errorf("kuhlerprofild daemon is not running (required for curve profiles): %w\nStart the daemon with: sudo systemctl start kuhlerprofil.service", err)
				}
				if err := client.SetCurveProfile(targetProfile); err != nil {
					return fmt.Errorf("failed to set curve profile to %q: %w", targetProfile, err)
				}
				_ = client.SetAutoMode(true)
				telem, _ := client.GetStatus()
				modeStr := ""
				if telem.ActiveMode != "" {
					modeStr = fmt.Sprintf(" -> %s mode", strings.Title(string(telem.ActiveMode)))
				}
				fmt.Fprintf(opts.getOut(), "✔ Curve profile set to %q (Auto Governor active%s)\n", targetProfile, modeStr)
				return nil
			}
		},
	}

	return curveCmd
}

