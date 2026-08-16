package main

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"koolthing/pkg/dbusapi"
	"koolthing/pkg/driver"
	"koolthing/pkg/tui"
)

// Version is the current release version of koolthing.
const Version = "0.1.0"

// Options allows dependency injection for testing and customization.
type Options struct {
	ClientFactory func() (*dbusapi.DBusClient, error)
	DriverFactory func() driver.HardwareDriver
	TUIStarter    func(client *dbusapi.DBusClient) error
	Out           io.Writer
	ErrOut        io.Writer
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

// NewRootCmd constructs the root Cobra command for koolthing.
func NewRootCmd(opts Options) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:           "koolthing",
		Short:         "KoolThing - ASUS Linux thermal, fan and battery management suite",
		Long:          "KoolThing is a lightweight thermal, fan curve, and battery health charging management CLI and TUI for ASUS VivoBook and ROG Linux laptops.",
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
	rootCmd.AddCommand(newTUICmd(opts))

	return rootCmd
}

// Execute executes the CLI with default production options.
func Execute() {
	cmd := NewRootCmd(Options{})
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
