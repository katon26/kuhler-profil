package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"kuhlerprofil/pkg/config"
	"kuhlerprofil/pkg/driver"
	"kuhlerprofil/pkg/engine"
	"kuhlerprofil/pkg/models"
)

func setupMockDriver() (driver.HardwareDriver, driver.FileSystem) {
	mockFS := driver.NewMockFS()
	mockFS.WriteFile("/sys/devices/platform/asus-nb-wmi/throttle_thermal_policy", []byte("0\n"))
	mockFS.WriteFile("/sys/class/power_supply/BAT0/charge_control_end_threshold", []byte("80\n"))
	mockFS.WriteFile("/sys/class/power_supply/BAT0/capacity", []byte("85\n"))
	mockFS.WriteFile("/sys/class/power_supply/BAT0/status", []byte("Discharging\n"))
	mockFS.WriteFile("/sys/class/hwmon/hwmon0/temp1_input", []byte("48000\n"))
	mockFS.WriteFile("/sys/class/hwmon/hwmon0/fan1_input", []byte("2100\n"))
	mockFS.WriteFile("/sys/class/hwmon/hwmon0/fan2_input", []byte("1950\n"))

	drv := driver.NewCustomDriver(mockFS)
	return drv, mockFS
}

func TestParseFlags(t *testing.T) {
	t.Run("DefaultFlags", func(t *testing.T) {
		var buf bytes.Buffer
		opts, shouldExit, err := ParseFlags([]string{}, &buf)
		if err != nil {
			t.Fatalf("ParseFlags unexpected error: %v", err)
		}
		if shouldExit {
			t.Fatalf("ParseFlags should not exit on default flags")
		}
		if opts.ConfigPath != "" {
			t.Errorf("ConfigPath = %q, want empty", opts.ConfigPath)
		}
		if opts.DryRun {
			t.Errorf("DryRun = true, want false")
		}
		if opts.Verbose {
			t.Errorf("Verbose = true, want false")
		}
	})

	t.Run("CustomFlags", func(t *testing.T) {
		var buf bytes.Buffer
		opts, shouldExit, err := ParseFlags([]string{"--config", "/etc/custom.toml", "--dry-run", "-v"}, &buf)
		if err != nil {
			t.Fatalf("ParseFlags unexpected error: %v", err)
		}
		if shouldExit {
			t.Fatalf("ParseFlags should not exit on custom flags")
		}
		if opts.ConfigPath != "/etc/custom.toml" {
			t.Errorf("ConfigPath = %q, want /etc/custom.toml", opts.ConfigPath)
		}
		if !opts.DryRun {
			t.Errorf("DryRun = false, want true")
		}
		if !opts.Verbose {
			t.Errorf("Verbose = false, want true")
		}
	})

	t.Run("ShorthandConfigFlag", func(t *testing.T) {
		var buf bytes.Buffer
		opts, shouldExit, err := ParseFlags([]string{"-c", "/tmp/kuhler.toml"}, &buf)
		if err != nil {
			t.Fatalf("ParseFlags error: %v", err)
		}
		if shouldExit || opts.ConfigPath != "/tmp/kuhler.toml" {
			t.Errorf("ConfigPath = %q, want /tmp/kuhler.toml", opts.ConfigPath)
		}
	})

	t.Run("VersionFlag", func(t *testing.T) {
		var buf bytes.Buffer
		opts, shouldExit, err := ParseFlags([]string{"--version"}, &buf)
		if err != nil {
			t.Fatalf("ParseFlags error: %v", err)
		}
		if !shouldExit {
			t.Fatalf("ParseFlags should exit on --version")
		}
		if opts != nil {
			t.Errorf("expected nil opts on version exit")
		}
		if !strings.Contains(buf.String(), Version) {
			t.Errorf("version output %q does not contain %s", buf.String(), Version)
		}
	})

	t.Run("HelpFlag", func(t *testing.T) {
		var buf bytes.Buffer
		opts, shouldExit, err := ParseFlags([]string{"--help"}, &buf)
		if err != nil {
			t.Fatalf("ParseFlags error on help: %v", err)
		}
		if !shouldExit {
			t.Fatalf("ParseFlags should exit on --help")
		}
		if opts != nil {
			t.Errorf("expected nil opts on help exit")
		}
		if !strings.Contains(buf.String(), "Usage: kuhlerprofild") {
			t.Errorf("help output %q does not contain usage", buf.String())
		}
		if !strings.Contains(buf.String(), "KühlerProfil Daemon (kuhlerprofild) v0.1.0") {
			t.Errorf("help output %q does not contain banner", buf.String())
		}
	})

	t.Run("InvalidFlag", func(t *testing.T) {
		var buf bytes.Buffer
		_, _, err := ParseFlags([]string{"--nonexistent-flag"}, &buf)
		if err == nil {
			t.Fatalf("expected error on invalid flag")
		}
	})
}

func TestDaemonDryRun(t *testing.T) {
	drv, _ := setupMockDriver()
	var buf bytes.Buffer

	opts := DaemonOptions{
		DryRun:  true,
		Verbose: true,
		Driver:  drv,
		Output:  &buf,
	}

	daemon := NewDaemon(opts)
	if daemon.Driver() == nil {
		t.Fatalf("daemon.Driver() is nil")
	}

	ctx := context.Background()
	err := daemon.Run(ctx)
	if err != nil {
		t.Fatalf("daemon.Run in DryRun failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Hardware capability probe:") {
		t.Errorf("output does not contain capability probe: %s", out)
	}
	if !strings.Contains(out, "Dry-run complete") {
		t.Errorf("output does not contain dry-run completion log: %s", out)
	}
	if !strings.Contains(out, "Initial telemetry:") {
		t.Errorf("output does not contain initial telemetry log: %s", out)
	}
}

func TestDaemonLifecycleGracefulShutdown(t *testing.T) {
	drv, _ := setupMockDriver()
	var buf bytes.Buffer

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.toml")
	cfg := models.Config{
		DefaultMode:         models.ModeStandard,
		DefaultBatteryLimit: 80,
		PollIntervalMs:      20,
		AutoCurve:           false,
	}
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatalf("config.Save failed: %v", err)
	}

	opts := DaemonOptions{
		ConfigPath: cfgPath,
		DryRun:     false,
		Verbose:    true,
		Driver:     drv,
		Output:     &buf,
	}

	daemon := NewDaemon(opts)
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- daemon.Run(ctx)
	}()

	// Allow daemon to spin up
	time.Sleep(50 * time.Millisecond)

	eng := daemon.Engine()
	if eng == nil {
		t.Fatalf("daemon.Engine() is nil after startup")
	}

	telem := eng.GetTelemetry()
	if telem.ActiveMode != models.ModeStandard {
		t.Errorf("ActiveMode = %v, want %v", telem.ActiveMode, models.ModeStandard)
	}

	// Trigger shutdown
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("daemon.Run returned error on graceful shutdown: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for daemon to shutdown")
	}

	out := buf.String()
	if !strings.Contains(out, "Shutdown signal received") {
		t.Errorf("missing shutdown log in output: %s", out)
	}
	if !strings.Contains(out, "stopped cleanly") {
		t.Errorf("missing stopped cleanly log in output: %s", out)
	}
}

func TestDaemonConfigReloadSIGHUP(t *testing.T) {
	drv, _ := setupMockDriver()
	var buf bytes.Buffer

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.toml")
	cfg := models.Config{
		DefaultMode:         models.ModeStandard,
		DefaultBatteryLimit: 80,
		PollIntervalMs:      20,
		AutoCurve:           false,
	}
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatalf("config.Save failed: %v", err)
	}

	hupChan := make(chan os.Signal, 2)
	opts := DaemonOptions{
		ConfigPath: cfgPath,
		DryRun:     false,
		Driver:     drv,
		Output:     &buf,
		HupChan:    hupChan,
	}

	daemon := NewDaemon(opts)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- daemon.Run(ctx)
	}()

	time.Sleep(50 * time.Millisecond)

	eng := daemon.Engine()
	if eng == nil {
		t.Fatalf("engine is nil")
	}
	if eng.GetTelemetry().ActiveMode != models.ModeStandard {
		t.Errorf("initial mode = %v, want standard", eng.GetTelemetry().ActiveMode)
	}

	// Update configuration on disk: change mode to Boost and battery limit to 60%
	newCfg := models.Config{
		DefaultMode:         models.ModeBoost,
		DefaultBatteryLimit: 60,
		PollIntervalMs:      20,
		AutoCurve:           false,
	}
	if err := config.Save(cfgPath, newCfg); err != nil {
		t.Fatalf("saving updated config failed: %v", err)
	}

	// Send SIGHUP signal
	hupChan <- syscall.SIGHUP

	// Wait for reload
	time.Sleep(100 * time.Millisecond)

	telem := eng.GetTelemetry()
	if telem.ActiveMode != models.ModeBoost {
		t.Errorf("after SIGHUP, mode = %v, want boost", telem.ActiveMode)
	}
	if telem.BatteryLimit != 60 {
		t.Errorf("after SIGHUP, battery limit = %d, want 60", telem.BatteryLimit)
	}
	if telem.AutoMode {
		t.Errorf("after SIGHUP, auto mode = true, want false")
	}

	// Update configuration again with AutoCurve: true
	autoCfg := models.Config{
		DefaultMode:         models.ModeStandard,
		DefaultBatteryLimit: 80,
		PollIntervalMs:      20,
		AutoCurve:           true,
	}
	if err := config.Save(cfgPath, autoCfg); err != nil {
		t.Fatalf("saving updated auto config failed: %v", err)
	}

	// Send SIGHUP signal
	hupChan <- syscall.SIGHUP
	time.Sleep(100 * time.Millisecond)

	telemAuto := eng.GetTelemetry()
	if !telemAuto.AutoMode {
		t.Errorf("after second SIGHUP, auto mode = false, want true")
	}
	if telemAuto.BatteryLimit != 80 {
		t.Errorf("after second SIGHUP, battery limit = %d, want 80", telemAuto.BatteryLimit)
	}

	// Shutdown cleanly
	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("daemon.Run exited with error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for daemon exit")
	}
}

func TestDaemonReloadConfigErrors(t *testing.T) {
	daemon := NewDaemon(DaemonOptions{})
	err := daemon.ReloadConfig()
	if err == nil {
		t.Errorf("expected error when engine is uninitialized")
	}

	drv, _ := setupMockDriver()
	opts := DaemonOptions{
		ConfigPath: "/nonexistent/path/that/cannot/exist/config.toml",
		Driver:     drv,
	}
	daemon2 := NewDaemon(opts)
	daemon2.eng = modelsDefaultEngine(drv)
	// When config file doesn't exist, config.Load returns defaults without error
	if err := daemon2.ReloadConfig(); err != nil {
		t.Errorf("unexpected error on default fallback: %v", err)
	}
}

func modelsDefaultEngine(drv driver.HardwareDriver) *engine.Engine {
	return engine.NewEngine(drv, models.DefaultConfig(), "")
}

func TestProbeAndLogCapabilitiesVariations(t *testing.T) {
	// 1. Full ASUS VivoBook Sysfs
	drv1, _ := setupMockDriver()
	var buf1 bytes.Buffer
	d1 := NewDaemon(DaemonOptions{Driver: drv1, Output: &buf1})
	caps1 := d1.ProbeAndLogCapabilities()
	if !caps1.HasThermalPolicy || !caps1.HasBatteryLimit || !caps1.HasCPUTemp || !caps1.HasFan1RPM || !caps1.HasFan2RPM {
		t.Errorf("caps1 mismatch: %+v", caps1)
	}

	// 2. Empty mock filesystem
	mockFS2 := driver.NewMockFS()
	drv2 := driver.NewCustomDriver(mockFS2)
	var buf2 bytes.Buffer
	d2 := NewDaemon(DaemonOptions{Driver: drv2, Output: &buf2})
	caps2 := d2.ProbeAndLogCapabilities()
	if caps2.HasThermalPolicy || caps2.HasBatteryLimit || caps2.HasCPUTemp {
		t.Errorf("caps2 expected all false, got %+v", caps2)
	}
}

func TestSystemdServiceFile(t *testing.T) {
	servicePath := filepath.Join("..", "..", "systemd", "kuhlerprofil.service")
	data, err := os.ReadFile(servicePath)
	if err != nil {
		t.Fatalf("failed to read systemd/kuhlerprofil.service: %v", err)
	}

	content := string(data)
	requiredStrings := []string{
		"[Unit]",
		"Description=KühlerProfil ASUS VivoBook Thermal, Fan & Battery Management Daemon",
		"After=dbus.service systemd-suspend.service",
		"Wants=dbus.service",
		"[Service]",
		"Type=simple",
		"ExecStart=/usr/local/bin/kuhlerprofild",
		"Restart=always",
		"RestartSec=3s",
		"KillMode=process",
		"CapabilityBoundingSet=CAP_SYS_ADMIN",
		"ProtectSystem=full",
		"ProtectHome=read-only",
		"[Install]",
		"WantedBy=multi-user.target",
	}

	for _, req := range requiredStrings {
		if !strings.Contains(content, req) {
			t.Errorf("systemd service file missing required entry %q", req)
		}
	}
}
