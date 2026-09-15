package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"kuhlerprofil/pkg/dbusapi"
	"kuhlerprofil/pkg/driver"
	"kuhlerprofil/pkg/tui"
)

// Version is the current release version of kuhlerprofil.
const Version = "v0.1.0"

// Options allows dependency injection for testing and customization.
type Options struct {
	ClientFactory func() (*dbusapi.DBusClient, error)
	DriverFactory func() driver.HardwareDriver
	TUIStarter    func(client *dbusapi.DBusClient) error
	Out           io.Writer
	ErrOut        io.Writer
	BinaryName    string
}

func (o Options) getClient() (*dbusapi.DBusClient, error) {
	if o.ClientFactory != nil {
		return o.ClientFactory()
	}
	return dbusapi.NewClient()
}

func (o Options) getDriver() driver.HardwareDriver {
	if o.DriverFactory != nil {
		return o.DriverFactory()
	}
	return driver.NewDriver()
}

func (o Options) getOut() io.Writer {
	if o.Out != nil {
		return o.Out
	}
	return os.Stdout
}

func (o Options) getErrOut() io.Writer {
	if o.ErrOut != nil {
		return o.ErrOut
	}
	return os.Stderr
}

func (o Options) startTUI(client *dbusapi.DBusClient) error {
	if o.TUIStarter != nil {
		return o.TUIStarter(client)
	}
	prog := tui.NewTUI(client)
	_, err := prog.Run()
	return err
}

// NewRootCmd constructs the root Cobra command for kuhlerprofil.
func NewRootCmd(opts Options) *cobra.Command {
	binName := opts.BinaryName
	if binName == "" {
		binName = os.Args[0]
	}
	binName = filepath.Base(binName)
	if binName == "" || binName == "." || strings.HasSuffix(binName, ".test") {
		binName = "kuhlerprofil"
	}

	rootCmd := &cobra.Command{
		Use:   binName,
		Short: "KühlerProfil - ASUS Linux thermal, fan and battery management suite",
		Long:  "KühlerProfil is a lightweight thermal, fan curve, and battery health charging management CLI and TUI for ASUS VivoBook and ROG Linux laptops.",
		Example: fmt.Sprintf(`  %[1]s status
  %[1]s mode boost
  %[1]s battery 80
  %[1]s auto on
  %[1]s cooldown kick
  %[1]s tui`, binName),
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// When run without subcommands, default to launching the interactive TUI dashboard.
			client, _ := opts.getClient()
			return opts.startTUI(client)
		},
	}

	rootCmd.SetOut(opts.getOut())
	rootCmd.SetErr(opts.getErrOut())

	// Add subcommands
	rootCmd.AddCommand(newStatusCmd(opts))
	rootCmd.AddCommand(newModeCmd(opts))
	rootCmd.AddCommand(newBatteryCmd(opts))
	rootCmd.AddCommand(newAutoCmd(opts))
	rootCmd.AddCommand(newCurveCmd(opts))
	rootCmd.AddCommand(newCooldownCmd(opts))
	rootCmd.AddCommand(newTUICmd(opts))
	rootCmd.AddCommand(newSetCmd(opts))

	return rootCmd
}

func newSetCmd(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "set <setting> [value]",
		Short: "Convenience shortcut to set mode, curve profile, or battery limit",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("missing argument for set. Examples:\n  kp mode boost\n  kp curve set aggressive\n  kp battery 80\n  kp cooldown kick")
			}
			target := strings.ToLower(strings.TrimSpace(args[0]))
			if len(args) >= 2 {
				val := strings.ToLower(strings.TrimSpace(args[1]))
				switch target {
				case "curve", "profile":
					curveCmd := newCurveCmd(opts)
					return curveCmd.RunE(cmd, []string{"set", val})
				case "mode":
					modeCmd := newModeCmd(opts)
					return modeCmd.RunE(cmd, []string{val})
				case "battery":
					batCmd := newBatteryCmd(opts)
					return batCmd.RunE(cmd, []string{val})
				case "cooldown", "cd":
					cooldownCmd := newCooldownCmd(opts)
					return cooldownCmd.RunE(cmd, []string{val})
				case "auto":
					autoCmd := newAutoCmd(opts)
					return autoCmd.RunE(cmd, []string{val})
				}
			}

			// Single argument smart routing
			switch target {
			case "quiet", "balanced", "aggressive":
				curveCmd := newCurveCmd(opts)
				return curveCmd.RunE(cmd, []string{"set", target})
			case "silent", "standard", "boost":
				modeCmd := newModeCmd(opts)
				return modeCmd.RunE(cmd, []string{target})
			case "60", "80", "100":
				batCmd := newBatteryCmd(opts)
				return batCmd.RunE(cmd, []string{target})
			case "kick", "decay":
				cooldownCmd := newCooldownCmd(opts)
				return cooldownCmd.RunE(cmd, []string{target})
			default:
				return fmt.Errorf("unknown target %q. Did you mean:\n  kp curve set %s\n  kp mode %s\n  kp cooldown %s", target, target, target, target)
			}
		},
	}
}

// Execute executes the CLI with default production options.
func Execute() {
	cmd := NewRootCmd(Options{})
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
