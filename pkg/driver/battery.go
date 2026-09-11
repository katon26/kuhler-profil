package driver

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"kuhlerprofil/pkg/models"
)

var (
	// ErrNoBatteryThreshold indicates that no battery threshold sysfs interface was found.
	ErrNoBatteryThreshold = errors.New("no battery charge_control_end_threshold sysfs interface found")

	// Standard candidate battery names in Linux power_supply.
	candidateBatteries = []string{"BAT0", "BAT1", "BATC", "BATT"}

	// Standard candidate AC power supply names in Linux power_supply.
	candidateACAdapters = []string{"AC0", "AC", "ADP0", "ADP1", "ACAD"}
)

// DetectBatteryThresholdPath locates the charge_control_end_threshold file.
func DetectBatteryThresholdPath(fs FileSystem) (string, error) {
	// First check common candidate paths
	for _, bat := range candidateBatteries {
		p := filepath.Join("/sys/class/power_supply", bat, "charge_control_end_threshold")
		if fs.Exists(p) {
			return p, nil
		}
	}

	// Dynamic search in /sys/class/power_supply
	entries, err := fs.ReadDir("/sys/class/power_supply")
	if err == nil {
		for _, entry := range entries {
			p := filepath.Join("/sys/class/power_supply", entry.Name(), "charge_control_end_threshold")
			if fs.Exists(p) {
				return p, nil
			}
		}
	}

	return "", ErrNoBatteryThreshold
}

// DetectBatteryPath locates the primary battery power supply directory.
func DetectBatteryPath(fs FileSystem) (string, error) {
	for _, bat := range candidateBatteries {
		p := filepath.Join("/sys/class/power_supply", bat)
		if fs.Exists(p) {
			return p, nil
		}
	}

	entries, err := fs.ReadDir("/sys/class/power_supply")
	if err == nil {
		for _, entry := range entries {
			name := entry.Name()
			if strings.HasPrefix(strings.ToUpper(name), "BAT") {
				return filepath.Join("/sys/class/power_supply", name), nil
			}
		}
	}

	return "", errors.New("no battery device found in /sys/class/power_supply")
}

// ReadBatteryLimit reads the current battery charge limit threshold.
func ReadBatteryLimit(fs FileSystem) (int32, error) {
	path, err := DetectBatteryThresholdPath(fs)
	if err != nil {
		return 0, err
	}

	data, err := fs.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("failed to read battery limit from %s: %w", path, err)
	}

	valStr := strings.TrimSpace(string(data))
	val, err := strconv.ParseInt(valStr, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid integer battery limit %q at %s: %w", valStr, path, err)
	}

	return int32(val), nil
}

// WriteBatteryLimit updates the battery charge threshold after validating the value.
func WriteBatteryLimit(fs FileSystem, limit int32) error {
	if err := models.ValidateBatteryLimit(limit); err != nil {
		return err
	}

	path, err := DetectBatteryThresholdPath(fs)
	if err != nil {
		return err
	}

	data := []byte(fmt.Sprintf("%d\n", limit))
	if err := fs.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write battery limit %d to %s: %w", limit, path, err)
	}

	return nil
}

// ReadBatteryTelemetry gathers battery capacity, active charge threshold, and AC status.
func ReadBatteryTelemetry(fs FileSystem) (batteryPercent int32, batteryLimit int32, onAC bool, err error) {
	batDir, batErr := DetectBatteryPath(fs)
	if batErr == nil {
		// Read capacity
		capPath := filepath.Join(batDir, "capacity")
		if capData, err := fs.ReadFile(capPath); err == nil {
			if parsed, err := strconv.ParseInt(strings.TrimSpace(string(capData)), 10, 32); err == nil {
				batteryPercent = int32(parsed)
			}
		}

		// Read status
		statPath := filepath.Join(batDir, "status")
		if statData, err := fs.ReadFile(statPath); err == nil {
			status := strings.TrimSpace(string(statData))
			if strings.EqualFold(status, "Charging") || strings.EqualFold(status, "Full") {
				onAC = true
			}
		}
	}

	// Read threshold limit (if available)
	if limit, err := ReadBatteryLimit(fs); err == nil {
		batteryLimit = limit
	}

	// Check AC adapters if not already detected on AC
	if !onAC {
		for _, ac := range candidateACAdapters {
			onlinePath := filepath.Join("/sys/class/power_supply", ac, "online")
			if data, err := fs.ReadFile(onlinePath); err == nil {
				if strings.TrimSpace(string(data)) == "1" {
					onAC = true
					break
				}
			}
		}
	}

	// If dynamic power_supply directory search is needed for AC
	if !onAC {
		if entries, err := fs.ReadDir("/sys/class/power_supply"); err == nil {
			for _, entry := range entries {
				name := strings.ToUpper(entry.Name())
				if strings.HasPrefix(name, "AC") || strings.HasPrefix(name, "ADP") {
					onlinePath := filepath.Join("/sys/class/power_supply", entry.Name(), "online")
					if data, err := fs.ReadFile(onlinePath); err == nil {
						if strings.TrimSpace(string(data)) == "1" {
							onAC = true
							break
						}
					}
				}
			}
		}
	}

	return batteryPercent, batteryLimit, onAC, nil
}
