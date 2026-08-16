package driver

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// HwmonDevice represents a discovered Linux hwmon sysfs node.
type HwmonDevice struct {
	Path string
	Name string
}

// ListHwmonDevices scans /sys/class/hwmon for all available hwmon directories.
func ListHwmonDevices(fs FileSystem) ([]HwmonDevice, error) {
	hwmonRoot := "/sys/class/hwmon"
	entries, err := fs.ReadDir(hwmonRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", hwmonRoot, err)
	}

	var devices []HwmonDevice
	for _, entry := range entries {
		devPath := filepath.Join(hwmonRoot, entry.Name())
		nameFile := filepath.Join(devPath, "name")
		var devName string
		if data, err := fs.ReadFile(nameFile); err == nil {
			devName = strings.TrimSpace(string(data))
		}

		devices = append(devices, HwmonDevice{
			Path: devPath,
			Name: devName,
		})
	}

	// Sort for deterministic behavior
	sort.Slice(devices, func(i, j int) bool {
		return devices[i].Path < devices[j].Path
	})

	return devices, nil
}

// ReadHwmonTelemetry scans all hwmon nodes and extracts CPU temp and fan RPMs.
func ReadHwmonTelemetry(fs FileSystem) (cpuTemp float64, fan1RPM int32, fan2RPM int32, err error) {
	devices, err := ListHwmonDevices(fs)
	if err != nil {
		return 0, 0, 0, err
	}

	var foundTemp bool
	var foundFan1 bool
	var foundFan2 bool

	// Priority 1: Check ASUS hwmon device first (typical ASUS VivoBook layout)
	for _, dev := range devices {
		if strings.EqualFold(dev.Name, "asus") || strings.EqualFold(dev.Name, "asus-nb-wmi") || strings.EqualFold(dev.Name, "asus_custom") {
			if !foundTemp {
				if t, ok := readHwmonTemp(fs, dev.Path); ok {
					cpuTemp = t
					foundTemp = true
				}
			}
			if !foundFan1 {
				if f, ok := readHwmonFan(fs, dev.Path, 1); ok {
					fan1RPM = f
					foundFan1 = true
				}
			}
			if !foundFan2 {
				if f, ok := readHwmonFan(fs, dev.Path, 2); ok {
					fan2RPM = f
					foundFan2 = true
				}
			}
		}
	}

	// Priority 2: CPU temp from coretemp, k10temp, zenpower, cpu_thermal
	if !foundTemp {
		for _, dev := range devices {
			nameLower := strings.ToLower(dev.Name)
			if strings.Contains(nameLower, "coretemp") ||
				strings.Contains(nameLower, "k10temp") ||
				strings.Contains(nameLower, "zenpower") ||
				strings.Contains(nameLower, "cpu_thermal") {
				if t, ok := readHwmonTemp(fs, dev.Path); ok {
					cpuTemp = t
					foundTemp = true
					break
				}
			}
		}
	}

	// Priority 3: Any hwmon device with fans if not found yet
	if !foundFan1 || !foundFan2 {
		for _, dev := range devices {
			if !foundFan1 {
				if f, ok := readHwmonFan(fs, dev.Path, 1); ok {
					fan1RPM = f
					foundFan1 = true
				}
			}
			if !foundFan2 {
				if f, ok := readHwmonFan(fs, dev.Path, 2); ok {
					fan2RPM = f
					foundFan2 = true
				}
			}
		}
	}

	// Priority 4: Fallback CPU temp to any hwmon device with valid temp
	if !foundTemp {
		for _, dev := range devices {
			if t, ok := readHwmonTemp(fs, dev.Path); ok {
				cpuTemp = t
				foundTemp = true
				break
			}
		}
	}

	return cpuTemp, fan1RPM, fan2RPM, nil
}

// readHwmonTemp reads temp1_input (or first available temp*_input) and converts millidegrees to degrees C.
func readHwmonTemp(fs FileSystem, devPath string) (float64, bool) {
	// Try temp1_input first
	temp1Path := filepath.Join(devPath, "temp1_input")
	if data, err := fs.ReadFile(temp1Path); err == nil {
		if milli, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64); err == nil {
			return float64(milli) / 1000.0, true
		}
	}

	// Try scanning temp2_input .. temp8_input
	for i := 2; i <= 8; i++ {
		tPath := filepath.Join(devPath, fmt.Sprintf("temp%d_input", i))
		if data, err := fs.ReadFile(tPath); err == nil {
			if milli, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64); err == nil {
				return float64(milli) / 1000.0, true
			}
		}
	}

	return 0, false
}

// readHwmonFan reads fan<idx>_input and returns RPM.
func readHwmonFan(fs FileSystem, devPath string, index int) (int32, bool) {
	fanPath := filepath.Join(devPath, fmt.Sprintf("fan%d_input", index))
	if data, err := fs.ReadFile(fanPath); err == nil {
		if rpm, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 32); err == nil {
			return int32(rpm), true
		}
	}
	return 0, false
}
