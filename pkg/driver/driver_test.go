package driver_test

import (
	"os"
	"path/filepath"
	"testing"

	"koolthing/pkg/driver"
	"koolthing/pkg/models"
)

func TestDriverWithMockFS(t *testing.T) {
	mockFS := driver.NewMockFS()
	// Setup standard ASUS VivoBook sysfs structure in mock
	mockFS.WriteFile("/sys/devices/platform/asus-nb-wmi/throttle_thermal_policy", []byte("0\n"))
	mockFS.WriteFile("/sys/class/power_supply/BAT0/charge_control_end_threshold", []byte("80\n"))
	mockFS.WriteFile("/sys/class/power_supply/BAT0/capacity", []byte("78\n"))
	mockFS.WriteFile("/sys/class/power_supply/BAT0/status", []byte("Charging\n"))
	mockFS.WriteFile("/sys/class/hwmon/hwmon1/temp1_input", []byte("54000\n"))
	mockFS.WriteFile("/sys/class/hwmon/hwmon1/fan1_input", []byte("2400\n"))
	mockFS.WriteFile("/sys/class/hwmon/hwmon1/name", []byte("asus\n"))

	d := driver.NewCustomDriver(mockFS)

	// Test GetThermalMode
	mode, err := d.GetThermalMode()
	if err != nil {
		t.Fatalf("GetThermalMode error: %v", err)
	}
	if mode != models.ModeStandard {
		t.Errorf("GetThermalMode = %v, want %v", mode, models.ModeStandard)
	}

	// Test SetThermalMode to Boost
	if err := d.SetThermalMode(models.ModeBoost); err != nil {
		t.Fatalf("SetThermalMode error: %v", err)
	}
	content, err := mockFS.ReadFile("/sys/devices/platform/asus-nb-wmi/throttle_thermal_policy")
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if string(content) != "1\n" {
		t.Errorf("Written sysfs = %q, want '1\\n'", string(content))
	}

	// Test Battery Limit
	limit, err := d.GetBatteryLimit()
	if err != nil {
		t.Fatalf("GetBatteryLimit error: %v", err)
	}
	if limit != 80 {
		t.Errorf("GetBatteryLimit = %d, want 80", limit)
	}

	if err := d.SetBatteryLimit(60); err != nil {
		t.Fatalf("SetBatteryLimit error: %v", err)
	}
	batContent, err := mockFS.ReadFile("/sys/class/power_supply/BAT0/charge_control_end_threshold")
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if string(batContent) != "60\n" {
		t.Errorf("Written battery sysfs = %q, want '60\\n'", string(batContent))
	}

	// Test Telemetry
	telem, err := d.ReadTelemetry()
	if err != nil {
		t.Fatalf("ReadTelemetry error: %v", err)
	}
	if telem.CPUTemp != 54.0 {
		t.Errorf("CPUTemp = %v, want 54.0", telem.CPUTemp)
	}
	if telem.Fan1RPM != 2400 {
		t.Errorf("Fan1RPM = %d, want 2400", telem.Fan1RPM)
	}
	if telem.BatteryPercent != 78 {
		t.Errorf("BatteryPercent = %d, want 78", telem.BatteryPercent)
	}
	if telem.BatteryLimit != 60 {
		t.Errorf("BatteryLimit = %d, want 60", telem.BatteryLimit)
	}
	if !telem.OnAC {
		t.Errorf("OnAC = %v, want true", telem.OnAC)
	}
	if telem.ActiveMode != models.ModeBoost {
		t.Errorf("ActiveMode = %v, want %v", telem.ActiveMode, models.ModeBoost)
	}

	// Test ProbeCapabilities
	caps := d.ProbeCapabilities()
	if !caps.HasThermalPolicy {
		t.Errorf("ProbeCapabilities HasThermalPolicy = false, want true")
	}
	if !caps.HasBatteryLimit {
		t.Errorf("ProbeCapabilities HasBatteryLimit = false, want true")
	}
	if !caps.HasCPUTemp {
		t.Errorf("ProbeCapabilities HasCPUTemp = false, want true")
	}
	if !caps.HasFan1RPM {
		t.Errorf("ProbeCapabilities HasFan1RPM = false, want true")
	}
	if caps.HasFan2RPM {
		t.Errorf("ProbeCapabilities HasFan2RPM = true, want false")
	}
}

func TestThermalMode_FanBoostModeFallback(t *testing.T) {
	mockFS := driver.NewMockFS()
	mockFS.WriteFile("/sys/devices/platform/asus-nb-wmi/fan_boost_mode", []byte("2\n"))

	d := driver.NewCustomDriver(mockFS)

	mode, err := d.GetThermalMode()
	if err != nil {
		t.Fatalf("GetThermalMode error: %v", err)
	}
	if mode != models.ModeSilent {
		t.Errorf("GetThermalMode = %v, want %v", mode, models.ModeSilent)
	}

	if err := d.SetThermalMode(models.ModeStandard); err != nil {
		t.Fatalf("SetThermalMode error: %v", err)
	}
	data, _ := mockFS.ReadFile("/sys/devices/platform/asus-nb-wmi/fan_boost_mode")
	if string(data) != "0\n" {
		t.Errorf("Written sysfs = %q, want '0\\n'", string(data))
	}

	caps := d.ProbeCapabilities()
	if !caps.HasFanBoostMode {
		t.Errorf("HasFanBoostMode = false, want true")
	}
	if caps.HasThermalPolicy {
		t.Errorf("HasThermalPolicy = true, want false")
	}
}

func TestThermalMode_ACPIPlatformProfileFallback(t *testing.T) {
	mockFS := driver.NewMockFS()
	mockFS.WriteFile("/sys/firmware/acpi/platform_profile", []byte("performance\n"))

	d := driver.NewCustomDriver(mockFS)

	mode, err := d.GetThermalMode()
	if err != nil {
		t.Fatalf("GetThermalMode error: %v", err)
	}
	if mode != models.ModeBoost {
		t.Errorf("GetThermalMode = %v, want %v", mode, models.ModeBoost)
	}

	// Test Setting Silent mode
	if err := d.SetThermalMode(models.ModeSilent); err != nil {
		t.Fatalf("SetThermalMode error: %v", err)
	}
	data, _ := mockFS.ReadFile("/sys/firmware/acpi/platform_profile")
	if string(data) != "quiet\n" {
		t.Errorf("Written sysfs = %q, want 'quiet\\n'", string(data))
	}

	// Test Setting Standard mode
	if err := d.SetThermalMode(models.ModeStandard); err != nil {
		t.Fatalf("SetThermalMode error: %v", err)
	}
	data, _ = mockFS.ReadFile("/sys/firmware/acpi/platform_profile")
	if string(data) != "balanced\n" {
		t.Errorf("Written sysfs = %q, want 'balanced\\n'", string(data))
	}

	caps := d.ProbeCapabilities()
	if !caps.HasPlatformProfile {
		t.Errorf("HasPlatformProfile = false, want true")
	}

	// Test Setting Invalid mode
	if err := d.SetThermalMode(models.ThermalMode("invalid_mode")); err == nil {
		t.Errorf("Expected error writing invalid thermal mode to ACPI, got nil")
	}
}

func TestThermalMode_Errors(t *testing.T) {
	mockFS := driver.NewMockFS()
	d := driver.NewCustomDriver(mockFS)

	// No thermal interface found
	_, err := d.GetThermalMode()
	if err == nil {
		t.Errorf("Expected error for missing thermal interface, got nil")
	}

	if err := d.SetThermalMode(models.ModeBoost); err == nil {
		t.Errorf("Expected error setting mode with no interface, got nil")
	}

	// Corrupted file
	mockFS.WriteFile("/sys/devices/platform/asus-nb-wmi/throttle_thermal_policy", []byte("corrupt\n"))
	_, err = d.GetThermalMode()
	if err == nil {
		t.Errorf("Expected error parsing corrupt thermal mode, got nil")
	}

	// Corrupted ACPI file
	mockFS2 := driver.NewMockFS()
	mockFS2.WriteFile("/sys/firmware/acpi/platform_profile", []byte("unrecognized_gibberish\n"))
	d2 := driver.NewCustomDriver(mockFS2)
	_, err = d2.GetThermalMode()
	if err == nil {
		t.Errorf("Expected error parsing unrecognized ACPI platform profile, got nil")
	}
}

func TestBatteryLimit_FallbacksAndValidation(t *testing.T) {
	mockFS := driver.NewMockFS()
	// Fallback to BAT1
	mockFS.WriteFile("/sys/class/power_supply/BAT1/charge_control_end_threshold", []byte("100\n"))
	mockFS.WriteFile("/sys/class/power_supply/BAT1/capacity", []byte("95\n"))
	mockFS.WriteFile("/sys/class/power_supply/BAT1/status", []byte("Discharging\n"))
	mockFS.WriteFile("/sys/class/power_supply/ADP0/online", []byte("1\n"))

	d := driver.NewCustomDriver(mockFS)

	limit, err := d.GetBatteryLimit()
	if err != nil {
		t.Fatalf("GetBatteryLimit on BAT1 error: %v", err)
	}
	if limit != 100 {
		t.Errorf("GetBatteryLimit = %d, want 100", limit)
	}

	// Test setting invalid limit
	if err := d.SetBatteryLimit(75); err == nil {
		t.Errorf("Expected error for invalid limit 75, got nil")
	}

	// Test valid limit 80
	if err := d.SetBatteryLimit(80); err != nil {
		t.Fatalf("SetBatteryLimit(80) error: %v", err)
	}
	batData, _ := mockFS.ReadFile("/sys/class/power_supply/BAT1/charge_control_end_threshold")
	if string(batData) != "80\n" {
		t.Errorf("Written BAT1 threshold = %q, want '80\\n'", string(batData))
	}

	// Read Telemetry
	telem, err := d.ReadTelemetry()
	if err != nil {
		t.Fatalf("ReadTelemetry error: %v", err)
	}
	if telem.BatteryPercent != 95 {
		t.Errorf("BatteryPercent = %d, want 95", telem.BatteryPercent)
	}
	if !telem.OnAC {
		t.Errorf("OnAC = false, want true (from ADP0/online)")
	}
}

func TestBattery_MissingOrCorrupt(t *testing.T) {
	mockFS := driver.NewMockFS()
	d := driver.NewCustomDriver(mockFS)

	_, err := d.GetBatteryLimit()
	if err == nil {
		t.Errorf("Expected error when no battery threshold exists, got nil")
	}

	if err := d.SetBatteryLimit(80); err == nil {
		t.Errorf("Expected error writing battery limit when no battery exists, got nil")
	}

	mockFS.WriteFile("/sys/class/power_supply/BAT0/charge_control_end_threshold", []byte("invalid\n"))
	_, err = d.GetBatteryLimit()
	if err == nil {
		t.Errorf("Expected error reading corrupt battery threshold, got nil")
	}
}

func TestHwmonScanner_DualFanAndMultiSensors(t *testing.T) {
	mockFS := driver.NewMockFS()
	// hwmon0: amdgpu
	mockFS.WriteFile("/sys/class/hwmon/hwmon0/name", []byte("amdgpu\n"))
	mockFS.WriteFile("/sys/class/hwmon/hwmon0/temp1_input", []byte("48000\n"))

	// hwmon1: coretemp
	mockFS.WriteFile("/sys/class/hwmon/hwmon1/name", []byte("coretemp\n"))
	mockFS.WriteFile("/sys/class/hwmon/hwmon1/temp1_input", []byte("62500\n"))

	// hwmon2: asus
	mockFS.WriteFile("/sys/class/hwmon/hwmon2/name", []byte("asus\n"))
	mockFS.WriteFile("/sys/class/hwmon/hwmon2/fan1_input", []byte("3200\n"))
	mockFS.WriteFile("/sys/class/hwmon/hwmon2/fan2_input", []byte("2800\n"))

	d := driver.NewCustomDriver(mockFS)

	telem, err := d.ReadTelemetry()
	if err != nil {
		t.Fatalf("ReadTelemetry error: %v", err)
	}

	// Should match coretemp for CPU temp and asus for dual fans
	if telem.CPUTemp != 62.5 {
		t.Errorf("CPUTemp = %v, want 62.5", telem.CPUTemp)
	}
	if telem.Fan1RPM != 3200 {
		t.Errorf("Fan1RPM = %d, want 3200", telem.Fan1RPM)
	}
	if telem.Fan2RPM != 2800 {
		t.Errorf("Fan2RPM = %d, want 2800", telem.Fan2RPM)
	}

	caps := d.ProbeCapabilities()
	if !caps.HasCPUTemp {
		t.Errorf("HasCPUTemp = false, want true")
	}
	if !caps.HasFan1RPM {
		t.Errorf("HasFan1RPM = false, want true")
	}
	if !caps.HasFan2RPM {
		t.Errorf("HasFan2RPM = false, want true")
	}
}

func TestHwmonScanner_FallbackTemp(t *testing.T) {
	mockFS := driver.NewMockFS()
	// hwmon0 has no name, but temp2_input
	mockFS.WriteFile("/sys/class/hwmon/hwmon0/temp2_input", []byte("43000\n"))
	mockFS.WriteFile("/sys/class/hwmon/hwmon0/fan1_input", []byte("1500\n"))

	d := driver.NewCustomDriver(mockFS)
	telem, err := d.ReadTelemetry()
	if err != nil {
		t.Fatalf("ReadTelemetry error: %v", err)
	}
	if telem.CPUTemp != 43.0 {
		t.Errorf("CPUTemp = %v, want 43.0", telem.CPUTemp)
	}
	if telem.Fan1RPM != 1500 {
		t.Errorf("Fan1RPM = %d, want 1500", telem.Fan1RPM)
	}
}

func TestMockFS_Operations(t *testing.T) {
	m := driver.NewMockFS()

	// Test Exists before write
	if m.Exists("/foo/bar.txt") {
		t.Errorf("Exists(/foo/bar.txt) = true, want false")
	}

	// Test Write and Read
	err := m.WriteFile("/foo/bar.txt", []byte("hello world"))
	if err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	if !m.Exists("/foo/bar.txt") {
		t.Errorf("Exists(/foo/bar.txt) = false, want true")
	}
	if !m.Exists("/foo") {
		t.Errorf("Exists(/foo) = false, want true")
	}

	data, err := m.ReadFile("/foo/bar.txt")
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("ReadFile = %q, want 'hello world'", string(data))
	}

	// Read non-existent file
	_, err = m.ReadFile("/nonexistent")
	if err == nil {
		t.Errorf("Expected error for /nonexistent, got nil")
	}

	// Read directory as file
	_, err = m.ReadFile("/foo")
	if err == nil {
		t.Errorf("Expected error reading directory as file, got nil")
	}

	// Stat file
	fi, err := m.Stat("/foo/bar.txt")
	if err != nil {
		t.Fatalf("Stat(/foo/bar.txt) error: %v", err)
	}
	if fi.IsDir() {
		t.Errorf("fi.IsDir() = true, want false")
	}
	if fi.Size() != int64(len("hello world")) {
		t.Errorf("fi.Size() = %d, want %d", fi.Size(), len("hello world"))
	}
	if fi.Name() != "bar.txt" {
		t.Errorf("fi.Name() = %q, want 'bar.txt'", fi.Name())
	}
	if fi.Sys() != nil {
		t.Errorf("fi.Sys() expected nil")
	}
	if fi.Mode() != 0644 {
		t.Errorf("fi.Mode() = %v, want 0644", fi.Mode())
	}
	if fi.ModTime().IsZero() {
		t.Errorf("fi.ModTime() is zero")
	}

	// Stat dir
	dirFi, err := m.Stat("/foo")
	if err != nil {
		t.Fatalf("Stat(/foo) error: %v", err)
	}
	if !dirFi.IsDir() {
		t.Errorf("dirFi.IsDir() = false, want true")
	}

	// Stat non-existent
	_, err = m.Stat("/missing")
	if err == nil {
		t.Errorf("Expected error for Stat(/missing), got nil")
	}

	// ReadDir
	entries, err := m.ReadDir("/foo")
	if err != nil {
		t.Fatalf("ReadDir(/foo) error: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "bar.txt" {
		t.Errorf("ReadDir(/foo) = %v, want ['bar.txt']", entries)
	}
	if entries[0].IsDir() {
		t.Errorf("entries[0].IsDir() = true, want false")
	}
	if entries[0].Type() != 0 {
		t.Errorf("entries[0].Type() = %v, want 0", entries[0].Type())
	}
	entryInfo, err := entries[0].Info()
	if err != nil || entryInfo.Name() != "bar.txt" {
		t.Errorf("entryInfo error = %v, name = %v", err, entryInfo.Name())
	}

	// ReadDir on non-directory
	_, err = m.ReadDir("/foo/bar.txt")
	if err == nil {
		t.Errorf("Expected error ReadDir on file, got nil")
	}

	// ReadDir on non-existent dir
	_, err = m.ReadDir("/not_found_dir")
	if err == nil {
		t.Errorf("Expected error ReadDir on non-existent dir, got nil")
	}

	// MkdirAll
	m.MkdirAll("/a/b/c/d")
	if !m.Exists("/a/b/c/d") {
		t.Errorf("Exists(/a/b/c/d) = false, want true")
	}
}

func TestRealFS_SmokeTest(t *testing.T) {
	rfs := driver.NewRealFS()

	tmpDir, err := os.MkdirTemp("", "koolthing_realfs_test")
	if err != nil {
		t.Fatalf("MkdirTemp error: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	filePath := filepath.Join(tmpDir, "test.txt")
	if err := rfs.WriteFile(filePath, []byte("realfs_test")); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	if !rfs.Exists(filePath) {
		t.Errorf("Exists(%s) = false, want true", filePath)
	}

	data, err := rfs.ReadFile(filePath)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if string(data) != "realfs_test" {
		t.Errorf("ReadFile = %q, want 'realfs_test'", string(data))
	}

	entries, err := rfs.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("ReadDir error: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "test.txt" {
		t.Errorf("ReadDir = %v, want ['test.txt']", entries)
	}

	fi, err := rfs.Stat(filePath)
	if err != nil {
		t.Fatalf("Stat error: %v", err)
	}
	if fi.Size() != int64(len("realfs_test")) {
		t.Errorf("Stat Size = %d, want %d", fi.Size(), len("realfs_test"))
	}

	// Smoke test default driver constructor
	defaultDriver := driver.NewDriver()
	if defaultDriver == nil {
		t.Errorf("NewDriver() returned nil")
	}
}
