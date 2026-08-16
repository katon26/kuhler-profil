package models

import (
	"fmt"
	"strings"
)

// ThermalMode represents an ASUS platform thermal profile mode.
type ThermalMode string

const (
	// ModeStandard represents the standard/balanced thermal profile (sysfs 0).
	ModeStandard ThermalMode = "standard"
	// ModeBoost represents the performance/boost thermal profile (sysfs 1).
	ModeBoost ThermalMode = "boost"
	// ModeSilent represents the quiet/whisper thermal profile (sysfs 2).
	ModeSilent ThermalMode = "silent"
)

// SysfsValue returns the integer code used by the Linux kernel sysfs interface
// (/sys/devices/platform/asus-nb-wmi/throttle_thermal_policy).
func (m ThermalMode) SysfsValue() int {
	switch m {
	case ModeStandard:
		return 0
	case ModeBoost:
		return 1
	case ModeSilent:
		return 2
	default:
		return 0
	}
}

// ParseThermalMode normalizes and parses a string or number into a ThermalMode enum.
// Supports aliases: 0/standard/normal/balanced, 1/boost/performance/overboost, 2/silent/whisper/quiet.
func ParseThermalMode(val string) (ThermalMode, error) {
	switch strings.ToLower(strings.TrimSpace(val)) {
	case "0", "standard", "normal", "balanced":
		return ModeStandard, nil
	case "1", "boost", "performance", "overboost":
		return ModeBoost, nil
	case "2", "silent", "whisper", "quiet":
		return ModeSilent, nil
	default:
		return "", fmt.Errorf("unknown thermal mode: %q (valid: standard, boost, silent)", val)
	}
}

// ParseSysfsThermalMode maps a raw sysfs integer code to a ThermalMode enum.
func ParseSysfsThermalMode(val int) ThermalMode {
	switch val {
	case 1:
		return ModeBoost
	case 2:
		return ModeSilent
	default:
		return ModeStandard
	}
}

// ValidateBatteryLimit verifies that the charge limit is one of the ASUS supported thresholds (60%, 80%, 100%).
func ValidateBatteryLimit(limit int32) error {
	switch limit {
	case 60, 80, 100:
		return nil
	default:
		return fmt.Errorf("invalid battery limit %d%% (supported: 60, 80, 100)", limit)
	}
}

// Telemetry captures live system metrics for fans, thermal sensors, battery, and daemon state.
type Telemetry struct {
	CPUTemp        float64     `json:"cpu_temp"`
	Fan1RPM        int32       `json:"fan1_rpm"`
	Fan2RPM        int32       `json:"fan2_rpm"`
	BatteryPercent int32       `json:"battery_percent"`
	BatteryLimit   int32       `json:"battery_limit"`
	OnAC           bool        `json:"on_ac"`
	ActiveMode     ThermalMode `json:"active_mode"`
	AutoMode       bool        `json:"auto_mode"`
}

// Config represents persistent daemon settings.
type Config struct {
	DefaultMode         ThermalMode `toml:"default_mode"`
	DefaultBatteryLimit int32       `toml:"default_battery_limit"`
	PollIntervalMs      int         `toml:"poll_interval_ms"`
	AutoCurve           bool        `toml:"auto_curve"`
}

// DefaultConfig returns safe out-of-the-box configuration values.
func DefaultConfig() Config {
	return Config{
		DefaultMode:         ModeStandard,
		DefaultBatteryLimit: 80,
		PollIntervalMs:      1500,
		AutoCurve:           false,
	}
}
