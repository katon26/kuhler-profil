package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/godbus/dbus/v5"

	"kuhlerprofil/pkg/dbusapi"
	"kuhlerprofil/pkg/driver"
	"kuhlerprofil/pkg/models"
)

// mockCaller implements dbusapi.DBusCaller for testing CLI subcommands.
type mockCaller struct {
	statusMap   interface{}
	lastMode    string
	lastLimit   int32
	lastAuto    bool
	lastCurve    string
	lastCooldown string
	returnError  bool
}

func (m *mockCaller) Call(method string, flags dbus.Flags, args ...interface{}) *dbus.Call {
	call := &dbus.Call{
		Method: method,
		Args:   args,
		Done:   make(chan *dbus.Call, 1),
	}

	if m.returnError {
		call.Err = errors.New("connection to kuhlerprofild failed")
		call.Done <- call
		return call
	}

	switch method {
	case dbusapi.Interface + ".GetStatus":
		call.Body = []interface{}{m.statusMap}
	case dbusapi.Interface + ".SetThermalMode":
		if len(args) > 0 {
			m.lastMode = args[0].(string)
		}
	case dbusapi.Interface + ".SetBatteryLimit":
		if len(args) > 0 {
			m.lastLimit = args[0].(int32)
		}
	case dbusapi.Interface + ".SetAutoMode":
		if len(args) > 0 {
			m.lastAuto = args[0].(bool)
		}
	case dbusapi.Interface + ".GetCooldownMode":
		mode := m.lastCooldown
		if mode == "" {
			mode = "kick"
		}
		call.Body = []interface{}{mode}
	case dbusapi.Interface + ".SetCooldownMode":
		if len(args) > 0 {
			m.lastCooldown = args[0].(string)
		}
	case dbusapi.Interface + ".GetCurveProfiles":
		profiles := map[string]map[string]dbus.Variant{
			"quiet": {
				"name":        dbus.MakeVariant("quiet"),
				"description": dbus.MakeVariant("Quiet acoustic profile"),
			},
			"balanced": {
				"name":        dbus.MakeVariant("balanced"),
				"description": dbus.MakeVariant("Balanced cooling profile"),
			},
			"aggressive": {
				"name":        dbus.MakeVariant("aggressive"),
				"description": dbus.MakeVariant("High-performance cooling profile"),
			},
		}
		call.Body = []interface{}{profiles}
	case dbusapi.Interface + ".GetActiveCurveProfile":
		active := m.lastCurve
		if active == "" {
			active = "balanced"
		}
		call.Body = []interface{}{active}
	case dbusapi.Interface + ".SetCurveProfile":
		if len(args) > 0 {
			m.lastCurve = args[0].(string)
		}
	default:
		call.Err = errors.New("unknown D-Bus method: " + method)
	}

	call.Done <- call
	return call
}

func (m *mockCaller) CallWithContext(ctx context.Context, method string, flags dbus.Flags, args ...interface{}) *dbus.Call {
	return m.Call(method, flags, args...)
}

type failingDriver struct {
	driver.HardwareDriver
}

func (f *failingDriver) ReadTelemetry() (models.Telemetry, error) {
	return models.Telemetry{}, errors.New("permission denied: /sys/class/hwmon")
}

func (f *failingDriver) SetThermalMode(mode models.ThermalMode) error {
	return errors.New("permission denied: /sys/devices/platform/asus-nb-wmi/throttle_thermal_policy")
}

func (f *failingDriver) SetBatteryLimit(limit int32) error {
	return errors.New("permission denied: /sys/class/power_supply/BAT0/charge_control_end_threshold")
}

func setupTestEnvironment(caller *mockCaller, mockFS *driver.MockFS) (Options, *bytes.Buffer, *bytes.Buffer) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}

	if mockFS == nil {
		mockFS = driver.NewMockFS()
		mockFS.WriteFile("/sys/devices/platform/asus-nb-wmi/throttle_thermal_policy", []byte("0\n"))
		mockFS.WriteFile("/sys/class/power_supply/BAT0/charge_control_end_threshold", []byte("80\n"))
		mockFS.WriteFile("/sys/class/power_supply/BAT0/capacity", []byte("82\n"))
		mockFS.WriteFile("/sys/class/power_supply/BAT0/status", []byte("Charging\n"))
		mockFS.WriteFile("/sys/class/hwmon/hwmon1/temp1_input", []byte("48000\n"))
		mockFS.WriteFile("/sys/class/hwmon/hwmon1/fan1_input", []byte("2400\n"))
		mockFS.WriteFile("/sys/class/hwmon/hwmon1/fan2_input", []byte("2200\n"))
	}

	opts := Options{
		ClientFactory: func() (*dbusapi.DBusClient, error) {
			if caller == nil {
				return nil, errors.New("no mock caller configured")
			}
			return dbusapi.NewCustomClient(caller), nil
		},
		DriverFactory: func() driver.HardwareDriver {
			return driver.NewCustomDriver(mockFS)
		},
		TUIStarter: func(client *dbusapi.DBusClient) error {
			out.WriteString("[TUI DASHBOARD STARTED]\n")
			return nil
		},
		Out:    out,
		ErrOut: errOut,
	}

	return opts, out, errOut
}

func defaultMockTelemetryMap() map[string]interface{} {
	return map[string]interface{}{
		"cpu_temp":           52.5,
		"fan1_rpm":           int32(2600),
		"fan2_rpm":           int32(2400),
		"battery_percent":    int32(80),
		"battery_limit":      int32(80),
		"on_ac":              true,
		"active_mode":        "standard",
		"auto_mode":          false,
		"active_curve":       "balanced",
		"has_hardware_curve": false,
		"cooldown_mode":      "kick",
	}
}

func TestRootCommand_DefaultLaunchesTUI(t *testing.T) {
	caller := &mockCaller{statusMap: defaultMockTelemetryMap()}
	opts, out, _ := setupTestEnvironment(caller, nil)

	cmd := NewRootCmd(opts)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error executing root command: %v", err)
	}

	if !strings.Contains(out.String(), "[TUI DASHBOARD STARTED]") {
		t.Errorf("expected TUI to launch on root command, got output: %s", out.String())
	}
}

func TestTUICommand(t *testing.T) {
	caller := &mockCaller{statusMap: defaultMockTelemetryMap()}
	opts, out, _ := setupTestEnvironment(caller, nil)

	cmd := NewRootCmd(opts)
	cmd.SetArgs([]string{"tui"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error executing tui command: %v", err)
	}

	if !strings.Contains(out.String(), "[TUI DASHBOARD STARTED]") {
		t.Errorf("expected TUI to launch on 'tui' subcommand, got output: %s", out.String())
	}
}

func TestHelpFlag(t *testing.T) {
	caller := &mockCaller{statusMap: defaultMockTelemetryMap()}
	opts, out, _ := setupTestEnvironment(caller, nil)

	cmd := NewRootCmd(opts)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error executing --help: %v", err)
	}

	helpOutput := out.String()
	if !strings.Contains(helpOutput, "KühlerProfil is a lightweight thermal") {
		t.Errorf("expected help output to mention KühlerProfil description, got:\n%s", helpOutput)
	}

	subcommands := []string{"status", "mode", "battery", "auto", "tui"}
	for _, sub := range subcommands {
		if !strings.Contains(helpOutput, sub) {
			t.Errorf("expected help output to mention subcommand %q, got:\n%s", sub, helpOutput)
		}
	}
}

func TestVersionFlag(t *testing.T) {
	caller := &mockCaller{statusMap: defaultMockTelemetryMap()}
	opts, out, _ := setupTestEnvironment(caller, nil)

	cmd := NewRootCmd(opts)
	cmd.SetArgs([]string{"--version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error executing --version: %v", err)
	}

	if !strings.Contains(out.String(), Version) {
		t.Errorf("expected output to contain version %s, got: %s", Version, out.String())
	}
	if Version != "v0.1.0" {
		t.Errorf("Version = %q, want 'v0.1.0'", Version)
	}
}

func TestRootCommand_BinaryNameAliases(t *testing.T) {
	testCases := []struct {
		name        string
		binName     string
		expectedUse string
	}{
		{"Default", "", "kuhlerprofil"},
		{"KuhlerProfil", "kuhlerprofil", "kuhlerprofil"},
		{"KP", "kp", "kp"},
		{"Kuhler", "kuhler", "kuhler"},
		{"FullPathKP", "/usr/local/bin/kp", "kp"},
		{"FullPathKuhler", "/usr/bin/kuhler", "kuhler"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts, out, _ := setupTestEnvironment(nil, nil)
			opts.BinaryName = tc.binName
			cmd := NewRootCmd(opts)
			if cmd.Use != tc.expectedUse {
				t.Errorf("cmd.Use = %q, want %q", cmd.Use, tc.expectedUse)
			}
			cmd.SetArgs([]string{"--help"})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("unexpected error executing --help: %v", err)
			}
			helpText := out.String()
			if !strings.Contains(helpText, "KühlerProfil is a lightweight thermal") {
				t.Errorf("expected help text to contain KühlerProfil description, got: %s", helpText)
			}
			if !strings.Contains(helpText, tc.expectedUse+" status") {
				t.Errorf("expected help text examples to use %q, got: %s", tc.expectedUse, helpText)
			}
		})
	}
}

func TestRootCommand_OSArgsDetection(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"/usr/bin/kp"}
	opts, _, _ := setupTestEnvironment(nil, nil)
	cmd := NewRootCmd(opts)
	if cmd.Use != "kp" {
		t.Errorf("cmd.Use from os.Args = %q, want 'kp'", cmd.Use)
	}

	os.Args = []string{"/usr/local/bin/kuhler"}
	cmd2 := NewRootCmd(opts)
	if cmd2.Use != "kuhler" {
		t.Errorf("cmd2.Use from os.Args = %q, want 'kuhler'", cmd2.Use)
	}

	os.Args = []string{"/usr/local/bin/kuhlerprofil"}
	cmd3 := NewRootCmd(opts)
	if cmd3.Use != "kuhlerprofil" {
		t.Errorf("cmd3.Use from os.Args = %q, want 'kuhlerprofil'", cmd3.Use)
	}
}

func TestStatusCmd_HumanReadable(t *testing.T) {
	caller := &mockCaller{statusMap: defaultMockTelemetryMap()}
	opts, out, _ := setupTestEnvironment(caller, nil)

	cmd := NewRootCmd(opts)
	cmd.SetArgs([]string{"status"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("status command failed: %v", err)
	}

	output := out.String()
	expectedSubstrings := []string{
		"52.5°C",
		"2600 RPM",
		"2400 RPM",
		"80%",
		"Standard",
		"Cooldown Mode:     Kick (Instant Reset)",
		"Auto Governor:     Disabled",
	}

	for _, sub := range expectedSubstrings {
		if !strings.Contains(output, sub) {
			t.Errorf("status output missing %q, got:\n%s", sub, output)
		}
	}

	// Verify misleading text is NOT present when Auto Governor is disabled
	if strings.Contains(output, "Disabled (Profile:") {
		t.Errorf("status output contains misleading text 'Disabled (Profile: ...)', got:\n%s", output)
	}
}

func TestStatusCmd_HumanReadable_AutoEnabled(t *testing.T) {
	statusMap := defaultMockTelemetryMap()
	statusMap["auto_mode"] = true
	statusMap["active_curve"] = "quiet"
	caller := &mockCaller{statusMap: statusMap}
	opts, out, _ := setupTestEnvironment(caller, nil)

	cmd := NewRootCmd(opts)
	cmd.SetArgs([]string{"status"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("status command failed: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Auto Governor:     Enabled (Profile: Quiet)") {
		t.Errorf("expected Auto Governor Enabled (Profile: Quiet), got:\n%s", output)
	}
}

func TestStatusCmd_JSONOutput(t *testing.T) {
	caller := &mockCaller{statusMap: defaultMockTelemetryMap()}
	opts, out, _ := setupTestEnvironment(caller, nil)

	cmd := NewRootCmd(opts)
	cmd.SetArgs([]string{"status", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("status --json command failed: %v", err)
	}

	var telem models.Telemetry
	if err := json.Unmarshal(out.Bytes(), &telem); err != nil {
		t.Fatalf("failed to parse json output: %v\nOutput was:\n%s", err, out.String())
	}

	if telem.CPUTemp != 52.5 {
		t.Errorf("telem.CPUTemp = %v, want 52.5", telem.CPUTemp)
	}
	if telem.Fan1RPM != 2600 || telem.Fan2RPM != 2400 {
		t.Errorf("fan RPMs = %d, %d; want 2600, 2400", telem.Fan1RPM, telem.Fan2RPM)
	}
	if telem.ActiveMode != models.ModeStandard {
		t.Errorf("telem.ActiveMode = %v, want standard", telem.ActiveMode)
	}
	if telem.BatteryLimit != 80 || telem.BatteryPercent != 80 {
		t.Errorf("battery = %d, %d; want 80, 80", telem.BatteryPercent, telem.BatteryLimit)
	}
	if !telem.OnAC {
		t.Errorf("telem.OnAC = false, want true")
	}
}

func TestStatusCmd_JSONShorthandFlag(t *testing.T) {
	caller := &mockCaller{statusMap: defaultMockTelemetryMap()}
	opts, out, _ := setupTestEnvironment(caller, nil)

	cmd := NewRootCmd(opts)
	cmd.SetArgs([]string{"status", "-j"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("status -j command failed: %v", err)
	}

	var telem models.Telemetry
	if err := json.Unmarshal(out.Bytes(), &telem); err != nil {
		t.Fatalf("failed to parse json output: %v", err)
	}
	if telem.CPUTemp != 52.5 {
		t.Errorf("telem.CPUTemp = %v, want 52.5", telem.CPUTemp)
	}
}

func TestStatusCmd_FallbackToDirectDriver(t *testing.T) {
	// D-Bus returns error
	caller := &mockCaller{returnError: true}
	opts, out, _ := setupTestEnvironment(caller, nil)

	cmd := NewRootCmd(opts)
	cmd.SetArgs([]string{"status"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected fallback to direct driver, got error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "48.0°C") || !strings.Contains(output, "2400 RPM") {
		t.Errorf("expected status output from direct driver fallback, got:\n%s", output)
	}
}

func TestStatusCmd_FailureWhenBothDBusAndDriverFail(t *testing.T) {
	caller := &mockCaller{returnError: true}
	opts, _, _ := setupTestEnvironment(caller, nil)
	// Inject failing driver
	opts.DriverFactory = func() driver.HardwareDriver {
		return &failingDriver{}
	}

	cmd := NewRootCmd(opts)
	cmd.SetArgs([]string{"status"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error when both D-Bus and driver fail, got nil")
	}
	if !strings.Contains(err.Error(), "kuhlerprofild daemon is not running") {
		t.Errorf("expected error to mention daemon not running, got: %v", err)
	}
}

func TestModeCmd_QueryCurrentMode(t *testing.T) {
	caller := &mockCaller{statusMap: defaultMockTelemetryMap()}
	opts, out, _ := setupTestEnvironment(caller, nil)

	cmd := NewRootCmd(opts)
	cmd.SetArgs([]string{"mode"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("mode query failed: %v", err)
	}

	if !strings.Contains(out.String(), "standard") {
		t.Errorf("expected output to contain current mode 'standard', got: %s", out.String())
	}
}

func TestModeCmd_SetValidModes(t *testing.T) {
	tests := []struct {
		arg          string
		expectedMode string
	}{
		{"silent", "silent"},
		{"standard", "standard"},
		{"boost", "boost"},
		{"0", "standard"},
		{"1", "boost"},
		{"2", "silent"},
		{"performance", "boost"},
		{"quiet", "silent"},
	}

	for _, tt := range tests {
		t.Run("Mode_"+tt.arg, func(t *testing.T) {
			caller := &mockCaller{statusMap: defaultMockTelemetryMap()}
			opts, out, _ := setupTestEnvironment(caller, nil)

			cmd := NewRootCmd(opts)
			cmd.SetArgs([]string{"mode", tt.arg})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("failed to set mode %q: %v", tt.arg, err)
			}

			if caller.lastMode != tt.expectedMode {
				t.Errorf("caller.lastMode = %q, want %q", caller.lastMode, tt.expectedMode)
			}
			if !strings.Contains(out.String(), tt.expectedMode) {
				t.Errorf("output missing confirmation for mode %q: %s", tt.expectedMode, out.String())
			}
		})
	}
}

func TestModeCmd_InvalidMode(t *testing.T) {
	caller := &mockCaller{statusMap: defaultMockTelemetryMap()}
	opts, _, _ := setupTestEnvironment(caller, nil)

	cmd := NewRootCmd(opts)
	cmd.SetArgs([]string{"mode", "turbo_nitro"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error for invalid mode, got nil")
	}
	if !strings.Contains(err.Error(), "invalid thermal mode") {
		t.Errorf("expected error message to mention 'invalid thermal mode', got: %v", err)
	}
}

func TestModeCmd_FallbackToDirectDriver(t *testing.T) {
	caller := &mockCaller{returnError: true}
	mockFS := driver.NewMockFS()
	mockFS.WriteFile("/sys/devices/platform/asus-nb-wmi/throttle_thermal_policy", []byte("0\n"))
	opts, out, _ := setupTestEnvironment(caller, mockFS)

	cmd := NewRootCmd(opts)
	cmd.SetArgs([]string{"mode", "boost"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("mode set with driver fallback failed: %v", err)
	}

	val, err := mockFS.ReadFile("/sys/devices/platform/asus-nb-wmi/throttle_thermal_policy")
	if err != nil || strings.TrimSpace(string(val)) != "1" {
		t.Errorf("sysfs throttle_thermal_policy = %q (err: %v), want '1'", string(val), err)
	}

	if !strings.Contains(out.String(), "boost") {
		t.Errorf("output missing boost confirmation: %s", out.String())
	}
}

func TestModeCmd_FailureWhenBothDBusAndDriverFail(t *testing.T) {
	caller := &mockCaller{returnError: true}
	opts, _, _ := setupTestEnvironment(caller, nil)
	opts.DriverFactory = func() driver.HardwareDriver {
		return &failingDriver{}
	}

	cmd := NewRootCmd(opts)
	cmd.SetArgs([]string{"mode", "boost"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error when both D-Bus and driver fail, got nil")
	}
	if !strings.Contains(err.Error(), "failed to set thermal mode") {
		t.Errorf("expected error message to mention failure, got: %v", err)
	}
}

func TestBatteryCmd_QueryCurrentLimit(t *testing.T) {
	caller := &mockCaller{statusMap: defaultMockTelemetryMap()}
	opts, out, _ := setupTestEnvironment(caller, nil)

	cmd := NewRootCmd(opts)
	cmd.SetArgs([]string{"battery"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("battery query failed: %v", err)
	}

	if !strings.Contains(out.String(), "80%") {
		t.Errorf("expected output to mention 80%% limit, got: %s", out.String())
	}
}

func TestBatteryCmd_SetValidLimits(t *testing.T) {
	limits := []struct {
		arg           string
		subcommand    []string
		expectedLimit int32
	}{
		{"60", []string{"battery", "60"}, 60},
		{"80", []string{"battery", "80"}, 80},
		{"100", []string{"battery", "100"}, 100},
		{"limit 60", []string{"battery", "limit", "60"}, 60},
		{"set 80", []string{"battery", "set", "80"}, 80},
	}

	for _, tt := range limits {
		t.Run("Limit_"+tt.arg, func(t *testing.T) {
			caller := &mockCaller{statusMap: defaultMockTelemetryMap()}
			opts, out, _ := setupTestEnvironment(caller, nil)

			cmd := NewRootCmd(opts)
			cmd.SetArgs(tt.subcommand)

			if err := cmd.Execute(); err != nil {
				t.Fatalf("failed to set battery limit %v: %v", tt.subcommand, err)
			}

			if caller.lastLimit != tt.expectedLimit {
				t.Errorf("caller.lastLimit = %d, want %d", caller.lastLimit, tt.expectedLimit)
			}
			if !strings.Contains(out.String(), string(rune(tt.expectedLimit))) && !strings.Contains(out.String(), "%") {
				t.Errorf("output missing confirmation: %s", out.String())
			}
		})
	}
}

func TestBatteryCmd_InvalidLimit(t *testing.T) {
	tests := [][]string{
		{"battery", "75"},
		{"battery", "90"},
		{"battery", "abc"},
		{"battery", "-10"},
		{"battery", "limit", "50"},
		{"battery", "set", "xyz"},
	}

	for _, args := range tests {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			caller := &mockCaller{statusMap: defaultMockTelemetryMap()}
			opts, _, _ := setupTestEnvironment(caller, nil)

			cmd := NewRootCmd(opts)
			cmd.SetArgs(args)

			err := cmd.Execute()
			if err == nil {
				t.Fatalf("expected error for invalid limit args %v, got nil", args)
			}
		})
	}
}

func TestBatteryCmd_FallbackToDirectDriver(t *testing.T) {
	caller := &mockCaller{returnError: true}
	mockFS := driver.NewMockFS()
	mockFS.WriteFile("/sys/class/power_supply/BAT0/charge_control_end_threshold", []byte("80\n"))
	opts, out, _ := setupTestEnvironment(caller, mockFS)

	cmd := NewRootCmd(opts)
	cmd.SetArgs([]string{"battery", "60"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("battery set with driver fallback failed: %v", err)
	}

	val, err := mockFS.ReadFile("/sys/class/power_supply/BAT0/charge_control_end_threshold")
	if err != nil || strings.TrimSpace(string(val)) != "60" {
		t.Errorf("sysfs battery threshold = %q (err: %v), want '60'", string(val), err)
	}

	if !strings.Contains(out.String(), "60%") {
		t.Errorf("output missing 60%% confirmation: %s", out.String())
	}
}

func TestBatteryCmd_FailureWhenBothDBusAndDriverFail(t *testing.T) {
	caller := &mockCaller{returnError: true}
	opts, _, _ := setupTestEnvironment(caller, nil)
	opts.DriverFactory = func() driver.HardwareDriver {
		return &failingDriver{}
	}

	cmd := NewRootCmd(opts)
	cmd.SetArgs([]string{"battery", "60"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error when both D-Bus and driver fail, got nil")
	}
	if !strings.Contains(err.Error(), "failed to set battery limit") {
		t.Errorf("expected error message to mention failure, got: %v", err)
	}
}

func TestAutoCmd_QueryGovernor(t *testing.T) {
	caller := &mockCaller{statusMap: defaultMockTelemetryMap()}
	opts, out, _ := setupTestEnvironment(caller, nil)

	cmd := NewRootCmd(opts)
	cmd.SetArgs([]string{"auto"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("auto query failed: %v", err)
	}

	if !strings.Contains(out.String(), "DISABLED") {
		t.Errorf("expected auto query to show DISABLED, got: %s", out.String())
	}
}

func TestAutoCmd_SetModes(t *testing.T) {
	tests := []struct {
		arg         string
		expectedVal bool
	}{
		{"on", true},
		{"off", false},
		{"enable", true},
		{"disable", false},
		{"true", true},
		{"false", false},
		{"1", true},
		{"0", false},
	}

	for _, tt := range tests {
		t.Run("Auto_"+tt.arg, func(t *testing.T) {
			caller := &mockCaller{statusMap: defaultMockTelemetryMap()}
			opts, out, _ := setupTestEnvironment(caller, nil)

			cmd := NewRootCmd(opts)
			cmd.SetArgs([]string{"auto", tt.arg})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("failed to set auto %q: %v", tt.arg, err)
			}

			if caller.lastAuto != tt.expectedVal {
				t.Errorf("caller.lastAuto = %v, want %v", caller.lastAuto, tt.expectedVal)
			}

			if tt.expectedVal && !strings.Contains(strings.ToLower(out.String()), "enabled") {
				t.Errorf("output missing enabled confirmation: %s", out.String())
			} else if !tt.expectedVal && !strings.Contains(strings.ToLower(out.String()), "disabled") {
				t.Errorf("output missing disabled confirmation: %s", out.String())
			}
		})
	}
}

func TestAutoCmd_InvalidArg(t *testing.T) {
	caller := &mockCaller{statusMap: defaultMockTelemetryMap()}
	opts, _, _ := setupTestEnvironment(caller, nil)

	cmd := NewRootCmd(opts)
	cmd.SetArgs([]string{"auto", "sometimes"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error for invalid auto arg, got nil")
	}
	if !strings.Contains(err.Error(), "invalid auto setting") {
		t.Errorf("expected error to mention 'invalid auto setting', got: %v", err)
	}
}

func TestAutoCmd_RequiresDaemonWhenDBusFails(t *testing.T) {
	caller := &mockCaller{returnError: true}
	opts, _, _ := setupTestEnvironment(caller, nil)

	cmd := NewRootCmd(opts)
	cmd.SetArgs([]string{"auto", "on"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error when setting auto with D-Bus failure, got nil")
	}
	if !strings.Contains(err.Error(), "kuhlerprofild") {
		t.Errorf("expected error to mention kuhlerprofild daemon, got: %v", err)
	}
}

func TestCurveCmd_Get(t *testing.T) {
	caller := &mockCaller{statusMap: defaultMockTelemetryMap(), lastCurve: "balanced"}
	opts, out, _ := setupTestEnvironment(caller, nil)

	cmd := NewRootCmd(opts)
	cmd.SetArgs([]string{"curve"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error executing 'curve': %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Active Curve Profile: Balanced") {
		t.Errorf("expected output to contain active curve profile 'Balanced', got: %s", output)
	}
	if !strings.Contains(output, "Hardware ACPI Curve") {
		t.Errorf("expected output to contain hardware ACPI curve info, got: %s", output)
	}
}

func TestCurveCmd_List(t *testing.T) {
	caller := &mockCaller{statusMap: defaultMockTelemetryMap(), lastCurve: "quiet"}
	opts, out, _ := setupTestEnvironment(caller, nil)

	cmd := NewRootCmd(opts)
	cmd.SetArgs([]string{"curve", "list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error executing 'curve list': %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Available Fan Curve Profiles:") {
		t.Errorf("expected header 'Available Fan Curve Profiles:', got: %s", output)
	}
	if !strings.Contains(output, "quiet") || !strings.Contains(output, "balanced") || !strings.Contains(output, "aggressive") {
		t.Errorf("expected list to contain profiles quiet, balanced, aggressive, got: %s", output)
	}
	// 'quiet' should have the active marker '* '
	if !strings.Contains(output, "* quiet") {
		t.Errorf("expected active marker '* quiet', got: %s", output)
	}
}

func TestCurveCmd_Set(t *testing.T) {
	caller := &mockCaller{statusMap: defaultMockTelemetryMap()}
	opts, out, _ := setupTestEnvironment(caller, nil)

	cmd := NewRootCmd(opts)
	cmd.SetArgs([]string{"curve", "set", "aggressive"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error executing 'curve set aggressive': %v", err)
	}

	if caller.lastCurve != "aggressive" {
		t.Errorf("expected caller.lastCurve to be 'aggressive', got %q", caller.lastCurve)
	}
	if !strings.Contains(out.String(), `Curve profile set to "aggressive"`) {
		t.Errorf("expected success message, got: %s", out.String())
	}
}

func TestCurveCmd_DirectProfileArg(t *testing.T) {
	caller := &mockCaller{statusMap: defaultMockTelemetryMap()}
	opts, out, _ := setupTestEnvironment(caller, nil)

	cmd := NewRootCmd(opts)
	cmd.SetArgs([]string{"curve", "quiet"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error executing 'curve quiet': %v", err)
	}

	if caller.lastCurve != "quiet" {
		t.Errorf("expected caller.lastCurve to be 'quiet', got %q", caller.lastCurve)
	}
	if !strings.Contains(out.String(), `Curve profile set to "quiet"`) {
		t.Errorf("expected success message, got: %s", out.String())
	}
}

func TestCurveCmd_DaemonFailure(t *testing.T) {
	caller := &mockCaller{returnError: true}
	opts, _, _ := setupTestEnvironment(caller, nil)

	cmd := NewRootCmd(opts)
	cmd.SetArgs([]string{"curve", "set", "quiet"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error when setting curve with daemon not running, got nil")
	}
	if !strings.Contains(err.Error(), "kuhlerprofild") {
		t.Errorf("expected daemon error to mention 'kuhlerprofild', got: %v", err)
	}
}

func TestSetCmd_SmartRouting(t *testing.T) {
	caller := &mockCaller{statusMap: defaultMockTelemetryMap()}
	opts, _, _ := setupTestEnvironment(caller, nil)

	cmd := NewRootCmd(opts)
	cmd.SetArgs([]string{"set", "aggressive"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error executing 'set aggressive': %v", err)
	}
	if caller.lastCurve != "aggressive" {
		t.Errorf("expected caller.lastCurve to be 'aggressive', got %q", caller.lastCurve)
	}

	cmd = NewRootCmd(opts)
	cmd.SetArgs([]string{"set", "boost"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error executing 'set boost': %v", err)
	}
	if caller.lastMode != "boost" {
		t.Errorf("expected caller.lastMode to be 'boost', got %q", caller.lastMode)
	}

	cmd = NewRootCmd(opts)
	cmd.SetArgs([]string{"set", "80"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error executing 'set 80': %v", err)
	}
	if caller.lastLimit != 80 {
		t.Errorf("expected caller.lastLimit to be 80, got %d", caller.lastLimit)
	}
}

func TestAutoCmd_WithProfileFlag(t *testing.T) {
	caller := &mockCaller{statusMap: defaultMockTelemetryMap()}
	opts, out, _ := setupTestEnvironment(caller, nil)

	cmd := NewRootCmd(opts)
	cmd.SetArgs([]string{"auto", "on", "--profile", "quiet"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error executing 'auto on --profile quiet': %v", err)
	}

	if !caller.lastAuto {
		t.Errorf("expected auto mode to be set to true")
	}
	if caller.lastCurve != "quiet" {
		t.Errorf("expected curve profile to be set to 'quiet', got %q", caller.lastCurve)
	}
	if !strings.Contains(out.String(), "quiet") {
		t.Errorf("expected output to mention profile 'quiet', got: %s", out.String())
	}
}

func TestCooldownCmd_GetStatus(t *testing.T) {
	caller := &mockCaller{statusMap: defaultMockTelemetryMap(), lastCooldown: "kick"}
	opts, out, _ := setupTestEnvironment(caller, nil)

	cmd := NewRootCmd(opts)
	cmd.SetArgs([]string{"cooldown"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cooldown command failed: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "kick") {
		t.Errorf("expected cooldown status to contain 'kick', got: %s", output)
	}
}

func TestCooldownCmd_SetModes(t *testing.T) {
	modes := []struct {
		input string
		want  string
	}{
		{"kick", "kick"},
		{"decay", "decay"},
		{"off", "off"},
	}

	for _, m := range modes {
		caller := &mockCaller{statusMap: defaultMockTelemetryMap()}
		opts, out, _ := setupTestEnvironment(caller, nil)

		cmd := NewRootCmd(opts)
		cmd.SetArgs([]string{"cooldown", m.input})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("cooldown %s failed: %v", m.input, err)
		}

		if caller.lastCooldown != m.want {
			t.Errorf("expected caller.lastCooldown = %q, got %q", m.want, caller.lastCooldown)
		}

		if !strings.Contains(out.String(), m.want) {
			t.Errorf("expected output to contain %q, got: %s", m.want, out.String())
		}
	}
}

func TestCooldownCmd_InvalidMode(t *testing.T) {
	caller := &mockCaller{statusMap: defaultMockTelemetryMap()}
	opts, _, _ := setupTestEnvironment(caller, nil)

	cmd := NewRootCmd(opts)
	cmd.SetArgs([]string{"cooldown", "hyper_super_mode"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error for invalid cooldown mode, got nil")
	}
	if !strings.Contains(err.Error(), "invalid cooldown mode") {
		t.Errorf("expected error message to mention invalid cooldown mode, got: %v", err)
	}
}

func TestSetCmd_CooldownRouting(t *testing.T) {
	caller := &mockCaller{statusMap: defaultMockTelemetryMap()}
	opts, _, _ := setupTestEnvironment(caller, nil)

	// Test kp set cooldown decay
	cmd := NewRootCmd(opts)
	cmd.SetArgs([]string{"set", "cooldown", "decay"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("set cooldown decay failed: %v", err)
	}
	if caller.lastCooldown != "decay" {
		t.Errorf("expected lastCooldown to be 'decay', got %q", caller.lastCooldown)
	}

	// Test single arg shortcut: kp set kick
	cmd = NewRootCmd(opts)
	cmd.SetArgs([]string{"set", "kick"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("set kick failed: %v", err)
	}
	if caller.lastCooldown != "kick" {
		t.Errorf("expected lastCooldown to be 'kick', got %q", caller.lastCooldown)
	}
}


