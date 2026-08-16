package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/godbus/dbus/v5"

	"koolthing/pkg/dbusapi"
	"koolthing/pkg/driver"
	"koolthing/pkg/models"
)

// mockCaller implements dbusapi.DBusCaller for testing CLI subcommands.
type mockCaller struct {
	statusMap   interface{}
	lastMode    string
	lastLimit   int32
	lastAuto    bool
	returnError bool
}

func (m *mockCaller) Call(method string, flags dbus.Flags, args ...interface{}) *dbus.Call {
	call := &dbus.Call{
		Method: method,
		Args:   args,
		Done:   make(chan *dbus.Call, 1),
	}

	if m.returnError {
		call.Err = errors.New("connection to koolthingd failed")
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
		"cpu_temp":        52.5,
		"fan1_rpm":        int32(2600),
		"fan2_rpm":        int32(2400),
		"battery_percent": int32(80),
		"battery_limit":   int32(80),
		"on_ac":           true,
		"active_mode":     "standard",
		"auto_mode":       false,
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
		"Disabled",
	}

	for _, sub := range expectedSubstrings {
		if !strings.Contains(output, sub) {
			t.Errorf("status output missing %q, got:\n%s", sub, output)
		}
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
	if !strings.Contains(err.Error(), "koolthingd daemon is not running") {
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
	if !strings.Contains(err.Error(), "koolthingd") {
		t.Errorf("expected error to mention koolthingd daemon, got: %v", err)
	}
}
