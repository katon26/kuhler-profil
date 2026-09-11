package driver

import (
	"kuhlerprofil/pkg/models"
)

// DriverCaps summarizes the detected hardware control capabilities and sysfs paths.
type DriverCaps struct {
	HasThermalPolicy   bool   `json:"has_thermal_policy"`
	HasFanBoostMode    bool   `json:"has_fan_boost_mode"`
	HasPlatformProfile bool   `json:"has_platform_profile"`
	HasBatteryLimit    bool   `json:"has_battery_limit"`
	HasCPUTemp         bool   `json:"has_cpu_temp"`
	HasFan1RPM         bool   `json:"has_fan1_rpm"`
	HasFan2RPM         bool   `json:"has_fan2_rpm"`
	ThermalPath        string `json:"thermal_path"`
	BatteryPath        string `json:"battery_path"`
	HwmonPath          string `json:"hwmon_path"`
}

// HardwareDriver provides unified access to ASUS platform thermal policies,
// battery charge threshold limits, and hwmon telemetry.
type HardwareDriver interface {
	GetThermalMode() (models.ThermalMode, error)
	SetThermalMode(mode models.ThermalMode) error
	GetBatteryLimit() (int32, error)
	SetBatteryLimit(limit int32) error
	ReadTelemetry() (models.Telemetry, error)
	ProbeCapabilities() DriverCaps
}

type sysfsDriver struct {
	fs FileSystem
}

// NewDriver constructs a HardwareDriver backed by the real OS sysfs.
func NewDriver() HardwareDriver {
	return &sysfsDriver{
		fs: NewRealFS(),
	}
}

// NewCustomDriver constructs a HardwareDriver backed by a custom or mock FileSystem.
func NewCustomDriver(fs FileSystem) HardwareDriver {
	return &sysfsDriver{
		fs: fs,
	}
}

// GetThermalMode reads the active thermal mode.
func (d *sysfsDriver) GetThermalMode() (models.ThermalMode, error) {
	return ReadThermalMode(d.fs)
}

// SetThermalMode writes the desired thermal profile to hardware sysfs.
func (d *sysfsDriver) SetThermalMode(mode models.ThermalMode) error {
	return WriteThermalMode(d.fs, mode)
}

// GetBatteryLimit reads the active battery charge threshold percentage.
func (d *sysfsDriver) GetBatteryLimit() (int32, error) {
	return ReadBatteryLimit(d.fs)
}

// SetBatteryLimit updates the battery charge threshold (e.g. 60, 80, 100).
func (d *sysfsDriver) SetBatteryLimit(limit int32) error {
	return WriteBatteryLimit(d.fs, limit)
}

// ReadTelemetry collects hardware sensor telemetry (CPU temp, fan RPMs, battery, AC state, thermal mode).
func (d *sysfsDriver) ReadTelemetry() (models.Telemetry, error) {
	var telem models.Telemetry

	// Read thermal mode if available (do not hard-fail if not present yet)
	if mode, err := ReadThermalMode(d.fs); err == nil {
		telem.ActiveMode = mode
	}

	// Read hwmon sensors (CPU temp & fans)
	cpuTemp, fan1, fan2, _ := ReadHwmonTelemetry(d.fs)
	telem.CPUTemp = cpuTemp
	telem.Fan1RPM = fan1
	telem.Fan2RPM = fan2

	// Read battery & power supply metrics
	percent, limit, onAC, _ := ReadBatteryTelemetry(d.fs)
	telem.BatteryPercent = percent
	telem.BatteryLimit = limit
	telem.OnAC = onAC

	return telem, nil
}

// ProbeCapabilities inspects sysfs and reports supported hardware features.
func (d *sysfsDriver) ProbeCapabilities() DriverCaps {
	var caps DriverCaps

	if path, ifType, err := DetectThermalInterface(d.fs); err == nil {
		caps.ThermalPath = path
		switch ifType {
		case InterfaceThrottleThermalPolicy:
			caps.HasThermalPolicy = true
		case InterfaceFanBoostMode:
			caps.HasFanBoostMode = true
		case InterfacePlatformProfile:
			caps.HasPlatformProfile = true
		}
	}

	if path, err := DetectBatteryThresholdPath(d.fs); err == nil {
		caps.HasBatteryLimit = true
		caps.BatteryPath = path
	}

	if devices, err := ListHwmonDevices(d.fs); err == nil && len(devices) > 0 {
		caps.HwmonPath = devices[0].Path
		for _, dev := range devices {
			if _, ok := readHwmonTemp(d.fs, dev.Path); ok {
				caps.HasCPUTemp = true
			}
			if _, ok := readHwmonFan(d.fs, dev.Path, 1); ok {
				caps.HasFan1RPM = true
			}
			if _, ok := readHwmonFan(d.fs, dev.Path, 2); ok {
				caps.HasFan2RPM = true
			}
		}
	}

	return caps
}
