package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"kuhlerprofil/pkg/config"
	"kuhlerprofil/pkg/models"
)

func cooldownModeDetail(mode models.CooldownMode) string {
	switch mode {
	case models.CooldownKick:
		return `kick (Thermal Policy "Kick" - Instant Reset)`
	case models.CooldownDecay:
		return `decay (Smart Passive Governor - Cooldown Decay)`
	case models.CooldownOff:
		return `off (Factory firmware default - No intervention)`
	default:
		return string(mode)
	}
}

func newCooldownCmd(opts Options) *cobra.Command {
	var statusFlag bool

	cooldownCmd := &cobra.Command{
		Use:     "cooldown [kick|decay|off]",
		Aliases: []string{"cd", "zero-rpm", "zerorpm"},
		Short:   "Get or set Zero-RPM cooldown recovery mode",
		Long: `Configure or inspect the Zero-RPM cooldown behavior when the CPU returns to idle temperatures.

Available Modes:
  kick   - Thermal Policy "Kick" (Instant Reset)
           Immediately re-asserts sysfs thermal mode once CPU <= 47°C with spinning fan,
           prompting the EC to release the fan curve hold in 5–10s. (10s debounce)
  decay  - Smart Passive Governor (Cooldown Decay)
           Monitors the cooldown trend and smoothly releases the fan latch once the CPU
           remains continuously cool (<= 47°C) for 15 seconds.
  off    - Factory firmware default (No intervention)
           Disables cooldown assistance, allowing stock BIOS hysteresis timers (1–3 min)
           to run unassisted without any software intervention.`,
		Example: `  # Query active cooldown mode
  kuhlerprofil cooldown
  kp cooldown --status

  # Switch cooldown recovery modes
  kp cooldown kick     # Thermal Policy "Kick" (Instant Reset)
  kp cooldown decay    # Smart Passive Governor (Cooldown Decay)
  kp cooldown off      # Factory firmware default (No intervention)`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 || statusFlag {
				// Query active cooldown mode
				var activeMode models.CooldownMode

				// Try D-Bus first
				client, err := opts.getClient()
				if err == nil && client != nil {
					if modeStr, err := client.GetCooldownMode(); err == nil && modeStr != "" {
						if parsed, err := models.ParseCooldownMode(modeStr); err == nil {
							activeMode = parsed
						}
					}
				}

				// Fallback to local config file
				if activeMode == "" {
					cfg, _ := config.Load("")
					activeMode = cfg.CooldownMode
					if activeMode == "" {
						activeMode = models.CooldownKick
					}
				}

				fmt.Fprintf(opts.getOut(), "Active cooldown mode: %s\n", cooldownModeDetail(activeMode))
				return nil
			}

			mode, err := models.ParseCooldownMode(args[0])
			if err != nil {
				return fmt.Errorf("invalid cooldown mode %q (valid: kick, decay, off)", args[0])
			}

			// Try D-Bus client first
			client, err := opts.getClient()
			if err == nil && client != nil {
				if setErr := client.SetCooldownMode(string(mode)); setErr == nil {
					fmt.Fprintf(opts.getOut(), "Cooldown mode set to: %s\n", cooldownModeDetail(mode))
					return nil
				}
			}

			// Fallback to persisting directly to local config
			cfg, err := config.Load("")
			if err != nil {
				cfg = models.DefaultConfig()
			}
			cfg.CooldownMode = mode
			if saveErr := config.Save("", cfg); saveErr != nil {
				return fmt.Errorf("failed to save cooldown mode to configuration: %w", saveErr)
			}

			fmt.Fprintf(opts.getOut(), "Cooldown mode set to: %s (persisted to config; restart daemon to apply)\n", cooldownModeDetail(mode))
			return nil
		},
	}

	cooldownCmd.Flags().BoolVarP(&statusFlag, "status", "s", false, "Display the currently active cooldown mode")

	return cooldownCmd
}
