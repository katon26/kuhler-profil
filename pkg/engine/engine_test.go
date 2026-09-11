package engine_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"kuhlerprofil/pkg/config"
	"kuhlerprofil/pkg/driver"
	"kuhlerprofil/pkg/engine"
	"kuhlerprofil/pkg/models"
)

func setupMockHardware() (driver.FileSystem, driver.HardwareDriver) {
	mockFS := driver.NewMockFS()
	mockFS.WriteFile("/sys/devices/platform/asus-nb-wmi/throttle_thermal_policy", []byte("0\n"))
	mockFS.WriteFile("/sys/class/power_supply/BAT0/charge_control_end_threshold", []byte("80\n"))
	mockFS.WriteFile("/sys/class/power_supply/BAT0/capacity", []byte("85\n"))
	mockFS.WriteFile("/sys/class/power_supply/BAT0/status", []byte("Discharging\n"))
	mockFS.WriteFile("/sys/class/hwmon/hwmon1/temp1_input", []byte("45000\n"))
	mockFS.WriteFile("/sys/class/hwmon/hwmon1/fan1_input", []byte("1800\n"))
	mockFS.WriteFile("/sys/class/hwmon/hwmon1/name", []byte("asus\n"))

	d := driver.NewCustomDriver(mockFS)
	return mockFS, d
}

func TestEngineLifecycle(t *testing.T) {
	mockFS, d := setupMockHardware()
	cfg := models.DefaultConfig()
	cfg.PollIntervalMs = 50

	tmpDir, err := os.MkdirTemp("", "koolthing-engine-lifecycle-*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	cfgPath := filepath.Join(tmpDir, "config.toml")

	eng := engine.NewEngine(d, cfg, cfgPath)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go eng.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	telem := eng.GetTelemetry()
	if telem.ActiveMode != models.ModeStandard {
		t.Errorf("ActiveMode = %v, want %v", telem.ActiveMode, models.ModeStandard)
	}
	if telem.BatteryLimit != 80 {
		t.Errorf("BatteryLimit = %d, want 80", telem.BatteryLimit)
	}
	if telem.CPUTemp != 45.0 {
		t.Errorf("CPUTemp = %v, want 45.0", telem.CPUTemp)
	}
	if telem.Fan1RPM != 1800 {
		t.Errorf("Fan1RPM = %d, want 1800", telem.Fan1RPM)
	}

	// Test SetMode
	if err := eng.SetMode(models.ModeBoost); err != nil {
		t.Fatalf("SetMode error: %v", err)
	}
	if eng.GetTelemetry().ActiveMode != models.ModeBoost {
		t.Errorf("After SetMode, ActiveMode = %v, want %v", eng.GetTelemetry().ActiveMode, models.ModeBoost)
	}
	// Verify sysfs write
	content, _ := mockFS.ReadFile("/sys/devices/platform/asus-nb-wmi/throttle_thermal_policy")
	if string(content) != "1\n" {
		t.Errorf("Sysfs thermal policy = %q, want '1\\n'", string(content))
	}

	// Test SetBatteryLimit
	if err := eng.SetBatteryLimit(60); err != nil {
		t.Fatalf("SetBatteryLimit error: %v", err)
	}
	if eng.GetTelemetry().BatteryLimit != 60 {
		t.Errorf("After SetBatteryLimit, BatteryLimit = %d, want 60", eng.GetTelemetry().BatteryLimit)
	}
	batContent, _ := mockFS.ReadFile("/sys/class/power_supply/BAT0/charge_control_end_threshold")
	if string(batContent) != "60\n" {
		t.Errorf("Sysfs battery limit = %q, want '60\\n'", string(batContent))
	}
}

func TestEngineSetModeAndBatteryValidation(t *testing.T) {
	_, d := setupMockHardware()
	cfg := models.DefaultConfig()
	eng := engine.NewEngine(d, cfg, "")

	// Invalid Mode
	if err := eng.SetMode(models.ThermalMode("turbo_hyper")); err == nil {
		t.Errorf("SetMode with invalid mode expected error, got nil")
	}

	// Invalid Battery Limit
	if err := eng.SetBatteryLimit(75); err == nil {
		t.Errorf("SetBatteryLimit(75) expected error, got nil")
	}
	if err := eng.SetBatteryLimit(0); err == nil {
		t.Errorf("SetBatteryLimit(0) expected error, got nil")
	}

	// Valid Battery Limits
	for _, lim := range []int32{60, 80, 100} {
		if err := eng.SetBatteryLimit(lim); err != nil {
			t.Errorf("SetBatteryLimit(%d) unexpected error: %v", lim, err)
		}
	}

	// Valid Modes
	for _, mode := range []models.ThermalMode{models.ModeSilent, models.ModeStandard, models.ModeBoost} {
		if err := eng.SetMode(mode); err != nil {
			t.Errorf("SetMode(%v) unexpected error: %v", mode, err)
		}
	}
}

func TestEngineTelemetrySubscription(t *testing.T) {
	mockFS, d := setupMockHardware()
	cfg := models.DefaultConfig()
	cfg.PollIntervalMs = 25

	eng := engine.NewEngine(d, cfg, "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	subCh := eng.SubscribeTelemetry()
	defer eng.UnsubscribeTelemetry(subCh)

	go eng.Start(ctx)

	// Wait for first telemetry tick
	select {
	case telem, ok := <-subCh:
		if !ok {
			t.Fatalf("subscription channel closed unexpectedly")
		}
		if telem.CPUTemp != 45.0 {
			t.Errorf("received CPUTemp = %v, want 45.0", telem.CPUTemp)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timeout waiting for telemetry tick")
	}

	// Update mock hardware temp and verify next tick updates
	mockFS.WriteFile("/sys/class/hwmon/hwmon1/temp1_input", []byte("68000\n"))

	select {
	case telem := <-subCh:
		if telem.CPUTemp != 68.0 {
			// Could be next tick
			select {
			case nextTelem := <-subCh:
				if nextTelem.CPUTemp != 68.0 {
					t.Errorf("received CPUTemp = %v, want 68.0", nextTelem.CPUTemp)
				}
			case <-time.After(500 * time.Millisecond):
				t.Fatalf("timeout waiting for updated telemetry tick")
			}
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timeout waiting for telemetry update")
	}
}

func TestEngineAutoGovernorHysteresis(t *testing.T) {
	mockFS, d := setupMockHardware()
	cfg := models.DefaultConfig()
	cfg.PollIntervalMs = 25
	cfg.AutoCurve = true

	eng := engine.NewEngine(d, cfg, "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go eng.Start(ctx)
	time.Sleep(60 * time.Millisecond)

	// Initial temp 45°C -> Standard Mode
	if eng.GetTelemetry().ActiveMode != models.ModeStandard {
		t.Errorf("Initial mode = %v, want %v", eng.GetTelemetry().ActiveMode, models.ModeStandard)
	}

	// Temperature rises to 82°C (above boost threshold >= 75°C/80°C)
	mockFS.WriteFile("/sys/class/hwmon/hwmon1/temp1_input", []byte("82000\n"))
	time.Sleep(80 * time.Millisecond)

	if eng.GetTelemetry().ActiveMode != models.ModeBoost {
		t.Errorf("At 82°C, mode = %v, want %v", eng.GetTelemetry().ActiveMode, models.ModeBoost)
	}

	// Temperature drops slightly to 68°C (hysteresis zone, should remain Boost)
	mockFS.WriteFile("/sys/class/hwmon/hwmon1/temp1_input", []byte("68000\n"))
	time.Sleep(80 * time.Millisecond)

	if eng.GetTelemetry().ActiveMode != models.ModeBoost {
		t.Errorf("At 68°C (hysteresis), mode = %v, want %v", eng.GetTelemetry().ActiveMode, models.ModeBoost)
	}

	// Temperature drops to 48°C (below low threshold <= 55°C) -> should drop to Standard/Silent
	mockFS.WriteFile("/sys/class/hwmon/hwmon1/temp1_input", []byte("48000\n"))
	time.Sleep(80 * time.Millisecond)

	if eng.GetTelemetry().ActiveMode != models.ModeStandard {
		t.Errorf("At 48°C, mode = %v, want %v", eng.GetTelemetry().ActiveMode, models.ModeStandard)
	}

	// Disable auto mode and check manual override stays
	if err := eng.SetAutoMode(false); err != nil {
		t.Fatalf("SetAutoMode(false) error: %v", err)
	}
	if eng.GetTelemetry().AutoMode != false {
		t.Errorf("AutoMode = %v, want false", eng.GetTelemetry().AutoMode)
	}

	if err := eng.SetMode(models.ModeSilent); err != nil {
		t.Fatalf("SetMode error: %v", err)
	}

	// High temp now should NOT change mode because AutoMode is disabled
	mockFS.WriteFile("/sys/class/hwmon/hwmon1/temp1_input", []byte("85000\n"))
	time.Sleep(80 * time.Millisecond)

	if eng.GetTelemetry().ActiveMode != models.ModeSilent {
		t.Errorf("With AutoMode false at 85°C, mode = %v, want %v", eng.GetTelemetry().ActiveMode, models.ModeSilent)
	}
}

func TestEngineConfigPersistence(t *testing.T) {
	_, d := setupMockHardware()
	cfg := models.DefaultConfig()

	tmpDir, err := os.MkdirTemp("", "koolthing-config-persist-*")
	if err != nil {
		t.Fatalf("MkdirTemp error: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	cfgPath := filepath.Join(tmpDir, "config.toml")

	eng := engine.NewEngine(d, cfg, cfgPath)

	if err := eng.SetMode(models.ModeBoost); err != nil {
		t.Fatalf("SetMode error: %v", err)
	}
	if err := eng.SetBatteryLimit(60); err != nil {
		t.Fatalf("SetBatteryLimit error: %v", err)
	}
	if err := eng.SetAutoMode(true); err != nil {
		t.Fatalf("SetAutoMode error: %v", err)
	}

	// Read config file directly
	loadedCfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load config error: %v", err)
	}

	if loadedCfg.DefaultMode != models.ModeBoost {
		t.Errorf("persisted DefaultMode = %v, want %v", loadedCfg.DefaultMode, models.ModeBoost)
	}
	if loadedCfg.DefaultBatteryLimit != 60 {
		t.Errorf("persisted DefaultBatteryLimit = %d, want 60", loadedCfg.DefaultBatteryLimit)
	}
	if loadedCfg.AutoCurve != true {
		t.Errorf("persisted AutoCurve = %v, want true", loadedCfg.AutoCurve)
	}
}

func TestEngineGettersAndNilUnsubscribe(t *testing.T) {
	_, d := setupMockHardware()
	cfg := models.DefaultConfig()

	eng := engine.NewEngine(d, cfg, "")

	if eng.GetDriver() != d {
		t.Errorf("GetDriver did not return expected driver")
	}

	gotCfg := eng.GetConfig()
	if gotCfg.DefaultMode != cfg.DefaultMode {
		t.Errorf("GetConfig defaultMode = %v, want %v", gotCfg.DefaultMode, cfg.DefaultMode)
	}

	// Unsubscribing nil should not panic
	eng.UnsubscribeTelemetry(nil)
}

func TestEngineNewWithZeroConfig(t *testing.T) {
	_, d := setupMockHardware()
	eng := engine.NewEngine(d, models.Config{}, "")

	cfg := eng.GetConfig()
	if cfg.PollIntervalMs != 1500 {
		t.Errorf("PollIntervalMs = %d, want default 1500", cfg.PollIntervalMs)
	}
	if cfg.DefaultMode != models.ModeStandard {
		t.Errorf("DefaultMode = %v, want default %v", cfg.DefaultMode, models.ModeStandard)
	}
	if cfg.DefaultBatteryLimit != 80 {
		t.Errorf("DefaultBatteryLimit = %d, want default 80", cfg.DefaultBatteryLimit)
	}
}

func TestEngineConcurrentAccessRace(t *testing.T) {
	_, d := setupMockHardware()
	cfg := models.DefaultConfig()
	cfg.PollIntervalMs = 10

	eng := engine.NewEngine(d, cfg, "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go eng.Start(ctx)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				_ = eng.GetTelemetry()
				sub := eng.SubscribeTelemetry()
				time.Sleep(5 * time.Millisecond)
				eng.UnsubscribeTelemetry(sub)
				if id%3 == 0 {
					_ = eng.SetMode(models.ModeBoost)
				} else if id%3 == 1 {
					_ = eng.SetMode(models.ModeStandard)
				} else {
					_ = eng.SetBatteryLimit(80)
				}
			}
		}(i)
	}

	wg.Wait()
}
