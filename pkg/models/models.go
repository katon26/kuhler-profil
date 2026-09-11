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

// CurvePoint defines a single temperature-to-PWM or temperature-to-mode mapping point.
type CurvePoint struct {
	TempC int         `json:"temp_c" toml:"temp_c"`
	PWM   int         `json:"pwm" toml:"pwm"`
	Mode  ThermalMode `json:"mode,omitempty" toml:"mode,omitempty"`
}

// CurveProfile defines a named fan curve profile.
type CurveProfile struct {
	Name        string       `json:"name" toml:"name"`
	Description string       `json:"description" toml:"description"`
	Points      []CurvePoint `json:"points" toml:"points"`
}

// ValidateCurvePoints verifies that curve points are monotonic, non-empty, and within safe operating limits.
func ValidateCurvePoints(points []CurvePoint) error {
	if len(points) < 2 {
		return fmt.Errorf("curve profile must contain at least 2 points, got %d", len(points))
	}

	lastTemp := -1
	for i, pt := range points {
		if pt.TempC < 30 || pt.TempC > 100 {
			return fmt.Errorf("point %d: temperature %d°C out of safe range (30..100)", i, pt.TempC)
		}
		if pt.TempC <= lastTemp {
			return fmt.Errorf("point %d: temperatures must be strictly increasing (%d°C <= %d°C)", i, pt.TempC, lastTemp)
		}
		if pt.PWM < 0 || pt.PWM > 255 {
			return fmt.Errorf("point %d: PWM %d out of bounds (0..255)", i, pt.PWM)
		}
		// Safety check: Never allow completely stopped fan above 55°C
		if pt.TempC > 55 && pt.PWM <= 0 {
			return fmt.Errorf("point %d: dangerous 0 PWM not allowed at elevated temperature %d°C", i, pt.TempC)
		}
		if pt.Mode != "" && pt.Mode != ModeSilent && pt.Mode != ModeStandard && pt.Mode != ModeBoost {
			return fmt.Errorf("point %d: invalid thermal mode %q (supported: silent, standard, boost)", i, pt.Mode)
		}
		lastTemp = pt.TempC
	}

	return nil
}

// ExpandPointsTo8 expands or interpolates a slice of CurvePoints to exactly 8 strictly monotonic points.
// Required by Linux kernel asus-wmi platform driver which requires all 8 curve registers.
func ExpandPointsTo8(points []CurvePoint) []CurvePoint {
	if len(points) == 8 {
		return points
	}
	if len(points) == 0 {
		return ExpandPointsTo8(DefaultCurveProfiles()["balanced"].Points)
	}
	if len(points) > 8 {
		return points[:8]
	}

	res := make([]CurvePoint, 8)
	tMin := points[0].TempC
	tMax := points[len(points)-1].TempC
	if tMax-tMin < 7 {
		tMax = tMin + 7
	}

	for i := 0; i < 8; i++ {
		t := tMin + (i*(tMax-tMin))/7
		if i > 0 && t <= res[i-1].TempC {
			t = res[i-1].TempC + 1
		}

		// Interpolate PWM
		pwm := points[0].PWM
		mode := points[0].Mode
		for j := 0; j < len(points)-1; j++ {
			p1 := points[j]
			p2 := points[j+1]
			if t >= p1.TempC && t <= p2.TempC {
				if p2.TempC > p1.TempC {
					ratio := float64(t-p1.TempC) / float64(p2.TempC-p1.TempC)
					pwm = p1.PWM + int(ratio*float64(p2.PWM-p1.PWM))
				} else {
					pwm = p1.PWM
				}
				mode = p2.Mode
				break
			} else if t > p2.TempC {
				pwm = p2.PWM
				mode = p2.Mode
			}
		}

		if i > 0 && pwm < res[i-1].PWM {
			pwm = res[i-1].PWM
		}
		if pwm < 0 {
			pwm = 0
		}
		if pwm > 255 {
			pwm = 255
		}

		res[i] = CurvePoint{
			TempC: t,
			PWM:   pwm,
			Mode:  mode,
		}
	}

	return res
}

// DefaultCurveProfiles returns safe, pre-tuned curve profiles for ASUS laptops.
func DefaultCurveProfiles() map[string]CurveProfile {
	return map[string]CurveProfile{
		"quiet": {
			Name:        "quiet",
			Description: "Acoustic-focused profile prioritizing silent fan operation at mild temperatures",
			Points: []CurvePoint{
				{TempC: 40, PWM: 50, Mode: ModeSilent},
				{TempC: 55, PWM: 100, Mode: ModeSilent},
				{TempC: 70, PWM: 160, Mode: ModeStandard},
				{TempC: 85, PWM: 255, Mode: ModeBoost},
			},
		},
		"balanced": {
			Name:        "balanced",
			Description: "Default balanced profile balancing cooling efficiency and acoustic comfort",
			Points: []CurvePoint{
				{TempC: 40, PWM: 70, Mode: ModeSilent},
				{TempC: 55, PWM: 120, Mode: ModeStandard},
				{TempC: 70, PWM: 180, Mode: ModeStandard},
				{TempC: 75, PWM: 255, Mode: ModeBoost},
			},
		},
		"aggressive": {
			Name:        "aggressive",
			Description: "High-performance profile maintaining lower thermals under heavy system load",
			Points: []CurvePoint{
				{TempC: 35, PWM: 100, Mode: ModeStandard},
				{TempC: 50, PWM: 170, Mode: ModeBoost},
				{TempC: 68, PWM: 255, Mode: ModeBoost},
			},
		},
	}
}

// Telemetry captures live system metrics for fans, thermal sensors, battery, and daemon state.
type Telemetry struct {
	CPUTemp            float64     `json:"cpu_temp"`
	Fan1RPM            int32       `json:"fan1_rpm"`
	Fan2RPM            int32       `json:"fan2_rpm"`
	BatteryPercent     int32       `json:"battery_percent"`
	BatteryLimit       int32       `json:"battery_limit"`
	OnAC               bool        `json:"on_ac"`
	ActiveMode         ThermalMode `json:"active_mode"`
	AutoMode           bool        `json:"auto_mode"`
	ActiveCurveProfile string      `json:"active_curve_profile,omitempty"`
	HasHardwareCurve   bool        `json:"has_hardware_curve"`
}

// Config represents persistent daemon settings.
type Config struct {
	DefaultMode         ThermalMode             `toml:"default_mode"`
	DefaultBatteryLimit int32                   `toml:"default_battery_limit"`
	PollIntervalMs      int                     `toml:"poll_interval_ms"`
	AutoCurve           bool                    `toml:"auto_curve"`
	ActiveCurveProfile  string                  `toml:"active_curve_profile"`
	Curves              map[string]CurveProfile `toml:"curves,omitempty"`
}

// DefaultConfig returns safe out-of-the-box configuration values.
func DefaultConfig() Config {
	return Config{
		DefaultMode:         ModeStandard,
		DefaultBatteryLimit: 80,
		PollIntervalMs:      1500,
		AutoCurve:           false,
		ActiveCurveProfile:  "balanced",
		Curves:              DefaultCurveProfiles(),
	}
}

