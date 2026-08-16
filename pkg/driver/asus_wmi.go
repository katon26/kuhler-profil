package driver

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"koolthing/pkg/models"
)

// ThermalInterfaceType indicates the specific sysfs mechanism used for thermal control.
type ThermalInterfaceType int

const (
	// InterfaceUnknown represents an undetected or unsupported interface.
	InterfaceUnknown ThermalInterfaceType = iota
	// InterfaceThrottleThermalPolicy is /sys/devices/platform/asus-nb-wmi/throttle_thermal_policy (0: Standard, 1: Boost, 2: Silent).
	InterfaceThrottleThermalPolicy
	// InterfaceFanBoostMode is /sys/devices/platform/asus-nb-wmi/fan_boost_mode (0: Normal, 1: Overboost, 2: Silent).
	InterfaceFanBoostMode
	// InterfacePlatformProfile is /sys/firmware/acpi/platform_profile (balanced, performance, quiet/low-power).
	InterfacePlatformProfile
)

var (
	// ErrNoThermalInterface indicates that none of the supported thermal sysfs interfaces could be found.
	ErrNoThermalInterface = errors.New("no supported ASUS thermal policy or ACPI platform_profile sysfs interface found")

	// Possible sysfs paths for throttle_thermal_policy across various ASUS kernels/drivers.
	throttleThermalPolicyPaths = []string{
		"/sys/devices/platform/asus-nb-wmi/throttle_thermal_policy",
		"/sys/devices/platform/asus_wmi/throttle_thermal_policy",
		"/sys/devices/platform/asus-wmi/throttle_thermal_policy",
	}

	// Possible sysfs paths for fan_boost_mode across various ASUS kernels/drivers.
	fanBoostModePaths = []string{
		"/sys/devices/platform/asus-nb-wmi/fan_boost_mode",
		"/sys/devices/platform/asus_wmi/fan_boost_mode",
		"/sys/devices/platform/asus-wmi/fan_boost_mode",
	}

	// ACPI platform profile path.
	platformProfilePath = "/sys/firmware/acpi/platform_profile"
)

// DetectThermalInterface probes the filesystem for available thermal control interfaces.
func DetectThermalInterface(fs FileSystem) (string, ThermalInterfaceType, error) {
	for _, p := range throttleThermalPolicyPaths {
		if fs.Exists(p) {
			return p, InterfaceThrottleThermalPolicy, nil
		}
	}

	for _, p := range fanBoostModePaths {
		if fs.Exists(p) {
			return p, InterfaceFanBoostMode, nil
		}
	}

	if fs.Exists(platformProfilePath) {
		return platformProfilePath, InterfacePlatformProfile, nil
	}

	return "", InterfaceUnknown, ErrNoThermalInterface
}

// ReadThermalMode queries the active thermal profile from the detected sysfs interface.
func ReadThermalMode(fs FileSystem) (models.ThermalMode, error) {
	path, ifType, err := DetectThermalInterface(fs)
	if err != nil {
		return "", err
	}

	data, err := fs.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read thermal mode from %s: %w", path, err)
	}

	strVal := strings.TrimSpace(string(data))

	switch ifType {
	case InterfaceThrottleThermalPolicy, InterfaceFanBoostMode:
		intVal, err := strconv.Atoi(strVal)
		if err != nil {
			return "", fmt.Errorf("invalid integer thermal mode value %q at %s: %w", strVal, path, err)
		}
		return models.ParseSysfsThermalMode(intVal), nil

	case InterfacePlatformProfile:
		switch strings.ToLower(strVal) {
		case "balanced":
			return models.ModeStandard, nil
		case "performance":
			return models.ModeBoost, nil
		case "quiet", "low-power":
			return models.ModeSilent, nil
		default:
			// Fallback parsing
			mode, err := models.ParseThermalMode(strVal)
			if err != nil {
				return "", fmt.Errorf("unknown ACPI platform profile %q at %s: %w", strVal, path, err)
			}
			return mode, nil
		}

	default:
		return "", ErrNoThermalInterface
	}
}

// WriteThermalMode sets the thermal profile on the detected sysfs interface.
func WriteThermalMode(fs FileSystem, mode models.ThermalMode) error {
	path, ifType, err := DetectThermalInterface(fs)
	if err != nil {
		return err
	}

	var writeData []byte
	switch ifType {
	case InterfaceThrottleThermalPolicy, InterfaceFanBoostMode:
		writeData = []byte(fmt.Sprintf("%d\n", mode.SysfsValue()))

	case InterfacePlatformProfile:
		var profile string
		switch mode {
		case models.ModeStandard:
			profile = "balanced"
		case models.ModeBoost:
			profile = "performance"
		case models.ModeSilent:
			profile = "quiet"
		default:
			return fmt.Errorf("unsupported thermal mode for ACPI platform_profile: %s", mode)
		}
		writeData = []byte(profile + "\n")

	default:
		return ErrNoThermalInterface
	}

	if err := fs.WriteFile(path, writeData, 0644); err != nil {
		return fmt.Errorf("failed to write thermal mode %s to %s: %w", mode, path, err)
	}

	return nil
}
