package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/godbus/dbus/v5"

	"koolthing/pkg/config"
	"koolthing/pkg/dbusapi"
	"koolthing/pkg/driver"
	"koolthing/pkg/engine"
)

// Version is the current semantic release version of koolthingd.
const Version = "0.1.0"

// DaemonOptions holds configuration and runtime options for koolthingd.
type DaemonOptions struct {
	ConfigPath string
	DryRun     bool
	Verbose    bool
	Driver     driver.HardwareDriver
	DBusConn   *dbus.Conn
	Output     io.Writer
	HupChan    chan os.Signal
}

// Daemon coordinates hardware drivers, background telemetry engine,
// D-Bus IPC service, and OS signal management.
type Daemon struct {
	mu           sync.RWMutex
	opts         DaemonOptions
	drv          driver.HardwareDriver
	eng          *engine.Engine
	server       *dbusapi.DBusServer
	dbusConn     *dbus.Conn
	ownsDBusConn bool
	logger       *log.Logger
}

// NewDaemon constructs a Daemon instance with provided options.
func NewDaemon(opts DaemonOptions) *Daemon {
	out := opts.Output
	if out == nil {
		out = os.Stdout
	}
	logger := log.New(out, "[koolthingd] ", log.LstdFlags)

	drv := opts.Driver
	if drv == nil {
		drv = driver.NewDriver()
	}

	return &Daemon{
		opts:   opts,
		drv:    drv,
		logger: logger,
	}
}

// Engine returns the active telemetry Engine.
func (d *Daemon) Engine() *engine.Engine {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.eng
}

// Driver returns the underlying HardwareDriver.
func (d *Daemon) Driver() driver.HardwareDriver {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.drv
}

// ProbeAndLogCapabilities inspects system hardware sysfs and logs detected capabilities.
func (d *Daemon) ProbeAndLogCapabilities() driver.DriverCaps {
	caps := d.drv.ProbeCapabilities()
	d.logger.Printf("Hardware capability probe:")
	d.logger.Printf("  • Thermal mode control: %v (policy: %v, boost: %v, profile: %v, path: %s)",
		caps.HasThermalPolicy || caps.HasFanBoostMode || caps.HasPlatformProfile,
		caps.HasThermalPolicy, caps.HasFanBoostMode, caps.HasPlatformProfile, caps.ThermalPath)
	d.logger.Printf("  • Battery charge limiter: %v (path: %s)", caps.HasBatteryLimit, caps.BatteryPath)
	d.logger.Printf("  • Telemetry sensors: CPU temp=%v, Fan1=%v, Fan2=%v (hwmon: %s)",
		caps.HasCPUTemp, caps.HasFan1RPM, caps.HasFan2RPM, caps.HwmonPath)
	return caps
}

// Run executes the daemon lifecycle: probes hardware, loads configuration,
// starts telemetry engine, exports D-Bus service, and manages signals until context cancellation.
func (d *Daemon) Run(ctx context.Context) error {
	d.logger.Printf("Starting KoolThing Daemon v%s (PID: %d, UID: %d)", Version, os.Getpid(), os.Geteuid())

	if os.Geteuid() != 0 && !d.opts.DryRun {
		d.logger.Printf("[WARN] Running without root privileges (UID: %d). Modifying sysfs and system D-Bus may require CAP_SYS_ADMIN or root.", os.Geteuid())
	}

	// 1. Probe Hardware
	caps := d.ProbeAndLogCapabilities()
	_ = caps

	// 2. Discover & Load Configuration
	cfgPath := d.opts.ConfigPath
	if cfgPath == "" {
		cfgPath = config.DiscoverConfigPath()
	}
	d.opts.ConfigPath = cfgPath

	cfg, err := config.Load(cfgPath)
	if err != nil {
		d.logger.Printf("[WARN] Error loading configuration from %q, using defaults: %v", cfgPath, err)
		cfg = config.Default()
	} else {
		d.logger.Printf("Loaded configuration from %q (DefaultMode: %s, BatteryLimit: %d%%, PollInterval: %dms, AutoCurve: %v)",
			cfgPath, cfg.DefaultMode, cfg.DefaultBatteryLimit, cfg.PollIntervalMs, cfg.AutoCurve)
	}

	// 3. Read initial telemetry snapshot
	if telem, err := d.drv.ReadTelemetry(); err == nil {
		d.logger.Printf("Initial telemetry: Temp=%.1f°C | Fan1=%d RPM | Fan2=%d RPM | Battery=%d%% (Limit: %d%%, AC: %v) | Mode=%s",
			telem.CPUTemp, telem.Fan1RPM, telem.Fan2RPM, telem.BatteryPercent, telem.BatteryLimit, telem.OnAC, telem.ActiveMode)
	}

	// 4. If DryRun, exit cleanly after probing
	if d.opts.DryRun {
		d.logger.Printf("Dry-run complete: hardware probe and configuration validated successfully.")
		return nil
	}

	// 5. Initialize & Start Engine
	eng := engine.NewEngine(d.drv, cfg, cfgPath)
	engCtx, engCancel := context.WithCancel(ctx)
	defer engCancel()

	d.mu.Lock()
	d.eng = eng
	d.mu.Unlock()

	go eng.Start(engCtx)
	d.logger.Printf("Telemetry and governor engine started (interval: %dms)", cfg.PollIntervalMs)

	// 6. Connect & Export D-Bus Service
	var conn *dbus.Conn
	if d.opts.DBusConn != nil {
		conn = d.opts.DBusConn
	} else {
		sysConn, err := dbus.SystemBus()
		if err != nil {
			d.logger.Printf("[WARN] Could not connect to System D-Bus: %v. D-Bus IPC service is disabled for this session.", err)
		} else {
			conn = sysConn
			d.ownsDBusConn = true
		}
	}

	if conn != nil {
		server := dbusapi.NewServer(eng)
		if err := server.Export(conn, eng); err != nil {
			d.logger.Printf("[WARN] Failed to export D-Bus interface %q: %v", dbusapi.Interface, err)
		} else {
			server.StartSignalBroadcaster(engCtx, conn)
			d.logger.Printf("D-Bus IPC service successfully exported on %q at object path %q", dbusapi.ServiceName, dbusapi.Path)
		}
		d.mu.Lock()
		d.dbusConn = conn
		d.server = server
		d.mu.Unlock()
	}

	// 7. Setup SIGHUP listener for dynamic configuration reload
	hupChan := d.opts.HupChan
	if hupChan == nil {
		hupChan = make(chan os.Signal, 1)
		signal.Notify(hupChan, syscall.SIGHUP)
		defer signal.Stop(hupChan)
	}

	d.logger.Printf("koolthingd daemon is active and listening for requests (press Ctrl+C or send SIGTERM to stop)...")

	// 8. Main Daemon Event Loop
	for {
		select {
		case <-ctx.Done():
			d.logger.Printf("Shutdown signal received, terminating daemon services...")
			d.mu.Lock()
			server := d.server
			dbusConn := d.dbusConn
			ownsConn := d.ownsDBusConn
			d.server = nil
			d.dbusConn = nil
			d.mu.Unlock()

			if server != nil {
				server.Stop()
			}
			if dbusConn != nil && ownsConn {
				_ = dbusConn.Close()
			}
			d.logger.Printf("koolthingd daemon stopped cleanly.")
			return nil

		case <-hupChan:
			d.logger.Printf("Received SIGHUP signal: reloading configuration from disk...")
			if err := d.ReloadConfig(); err != nil {
				d.logger.Printf("[ERROR] Configuration reload failed: %v", err)
			}
		}
	}
}

// ReloadConfig reads configuration from disk and applies updated parameters to the engine.
func (d *Daemon) ReloadConfig() error {
	d.mu.RLock()
	eng := d.eng
	d.mu.RUnlock()

	if eng == nil {
		return fmt.Errorf("engine not initialized")
	}

	cfgPath := d.opts.ConfigPath
	if cfgPath == "" {
		cfgPath = config.DiscoverConfigPath()
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("failed to reload config from %q: %w", cfgPath, err)
	}

	d.logger.Printf("Applying reloaded config from %q: Mode=%s, BatteryLimit=%d%%, AutoCurve=%v",
		cfgPath, cfg.DefaultMode, cfg.DefaultBatteryLimit, cfg.AutoCurve)

	if err := d.eng.SetMode(cfg.DefaultMode); err != nil {
		d.logger.Printf("[WARN] Failed to apply reloaded thermal mode: %v", err)
	}
	if err := d.eng.SetBatteryLimit(cfg.DefaultBatteryLimit); err != nil {
		d.logger.Printf("[WARN] Failed to apply reloaded battery limit: %v", err)
	}
	if err := d.eng.SetAutoMode(cfg.AutoCurve); err != nil {
		d.logger.Printf("[WARN] Failed to apply reloaded auto mode: %v", err)
	}

	return nil
}

// ParseFlags parses command line arguments and returns configured DaemonOptions.
// If help or version flags were requested, shouldExit returns true.
func ParseFlags(args []string, out io.Writer) (*DaemonOptions, bool, error) {
	flags := flag.NewFlagSet("koolthingd", flag.ContinueOnError)
	flags.SetOutput(out)

	configPath := flags.String("config", "", "Path to configuration file (/etc/koolthing/config.toml)")
	flags.StringVar(configPath, "c", "", "Path to configuration file (shorthand)")

	dryRun := flags.Bool("dry-run", false, "Probe hardware and print capabilities then exit")
	verbose := flags.Bool("verbose", false, "Enable verbose debug logging")
	flags.BoolVar(verbose, "v", false, "Enable verbose debug logging (shorthand)")

	versionFlag := flags.Bool("version", false, "Print koolthingd version")

	flags.Usage = func() {
		fmt.Fprintf(out, "KoolThing Daemon (koolthingd) v%s\n", Version)
		fmt.Fprintf(out, "ASUS VivoBook Thermal, Fan and Battery Management Background Service\n\n")
		fmt.Fprintf(out, "Usage: koolthingd [options]\n\n")
		fmt.Fprintf(out, "Options:\n")
		flags.PrintDefaults()
	}

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil, true, nil
		}
		return nil, true, err
	}

	if *versionFlag {
		fmt.Fprintf(out, "koolthingd v%s\n", Version)
		return nil, true, nil
	}

	opts := &DaemonOptions{
		ConfigPath: *configPath,
		DryRun:     *dryRun,
		Verbose:    *verbose,
		Output:     out,
	}

	return opts, false, nil
}

func main() {
	opts, shouldExit, err := ParseFlags(os.Args[1:], os.Stdout)
	if err != nil {
		os.Exit(2)
	}
	if shouldExit {
		os.Exit(0)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	daemon := NewDaemon(*opts)
	if err := daemon.Run(ctx); err != nil {
		log.Fatalf("[FATAL] koolthingd runtime error: %v", err)
	}
}
