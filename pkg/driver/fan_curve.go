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
	// ErrHardwareCurveUnsupported indicates that hardware ACPI custom fan curve registers are not supported.
	ErrHardwareCurveUnsupported = errors.New("hardware ACPI custom fan curve registers not supported on this model")
)

// HardwareCurveCaps summarizes detected hardware fan curve support in hwmon sysfs.
type HardwareCurveCaps struct {
	Supported bool   `json:"supported"`
	HwmonPath string `json:"hwmon_path,omitempty"`
	FanCount  int    `json:"fan_count"`
}

// DetectHardwareCurveSupport searches hwmon devices for ASUS custom fan curve sysfs endpoints.
func DetectHardwareCurveSupport(fs FileSystem) (HardwareCurveCaps, error) {
	devices, err := ListHwmonDevices(fs)
	if err != nil {
		return HardwareCurveCaps{}, err
	}

	for _, dev := range devices {
		// Look for pwm1_auto_point1_temp and pwm1_enable
		point1Temp := filepath.Join(dev.Path, "pwm1_auto_point1_temp")
		pwm1Enable := filepath.Join(dev.Path, "pwm1_enable")

		if fs.Exists(point1Temp) && fs.Exists(pwm1Enable) {
			fanCount := 1
			point2Temp := filepath.Join(dev.Path, "pwm2_auto_point1_temp")
			if fs.Exists(point2Temp) {
				fanCount = 2
				point3Temp := filepath.Join(dev.Path, "pwm3_auto_point1_temp")
				if fs.Exists(point3Temp) {
					fanCount = 3
				}
			}

			return HardwareCurveCaps{
				Supported: true,
				HwmonPath: dev.Path,
				FanCount:  fanCount,
			}, nil
		}
	}

	return HardwareCurveCaps{Supported: false}, nil
}

// ReadHardwareCurve reads the 8-point temperature-to-PWM curve from hardware sysfs.
func ReadHardwareCurve(fs FileSystem, fanIdx int) ([]models.CurvePoint, error) {
	caps, err := DetectHardwareCurveSupport(fs)
	if err != nil || !caps.Supported {
		return nil, ErrHardwareCurveUnsupported
	}

	if fanIdx < 1 || fanIdx > caps.FanCount {
		fanIdx = 1
	}

	var points []models.CurvePoint
	for i := 1; i <= 8; i++ {
		tFile := filepath.Join(caps.HwmonPath, fmt.Sprintf("pwm%d_auto_point%d_temp", fanIdx, i))
		pFile := filepath.Join(caps.HwmonPath, fmt.Sprintf("pwm%d_auto_point%d_pwm", fanIdx, i))

		tData, err1 := fs.ReadFile(tFile)
		pData, err2 := fs.ReadFile(pFile)
		if err1 != nil || err2 != nil {
			break
		}

		temp, err1 := strconv.Atoi(strings.TrimSpace(string(tData)))
		pwm, err2 := strconv.Atoi(strings.TrimSpace(string(pData)))
		if err1 != nil || err2 != nil {
			break
		}

		points = append(points, models.CurvePoint{
			TempC: temp,
			PWM:   pwm,
		})
	}

	if len(points) == 0 {
		return nil, ErrHardwareCurveUnsupported
	}

	return points, nil
}

// WriteHardwareCurve writes the temperature and PWM points to hardware sysfs.
// Automatically interpolates/expands points to all 8 hardware registers to satisfy Linux kernel asus-wmi.
func WriteHardwareCurve(fs FileSystem, fanIdx int, points []models.CurvePoint) error {
	caps, err := DetectHardwareCurveSupport(fs)
	if err != nil || !caps.Supported {
		return ErrHardwareCurveUnsupported
	}

	if fanIdx < 1 || fanIdx > caps.FanCount {
		fanIdx = 1
	}

	points8 := models.ExpandPointsTo8(points)
	if err := models.ValidateCurvePoints(points8); err != nil {
		return fmt.Errorf("invalid fan curve points: %w", err)
	}

	for i, pt := range points8 {
		tFile := filepath.Join(caps.HwmonPath, fmt.Sprintf("pwm%d_auto_point%d_temp", fanIdx, i+1))
		pFile := filepath.Join(caps.HwmonPath, fmt.Sprintf("pwm%d_auto_point%d_pwm", fanIdx, i+1))

		if err := fs.WriteFile(tFile, []byte(fmt.Sprintf("%d\n", pt.TempC)), 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", tFile, err)
		}
		if err := fs.WriteFile(pFile, []byte(fmt.Sprintf("%d\n", pt.PWM)), 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", pFile, err)
		}
	}

	return nil
}

// SetHardwareCurveEnabled configures pwmX_enable (1=custom curve, 2=factory auto, 3=reset factory).
func SetHardwareCurveEnabled(fs FileSystem, fanIdx int, mode int) error {
	caps, err := DetectHardwareCurveSupport(fs)
	if err != nil || !caps.Supported {
		return ErrHardwareCurveUnsupported
	}

	if fanIdx < 1 || fanIdx > caps.FanCount {
		fanIdx = 1
	}

	pwmEnableFile := filepath.Join(caps.HwmonPath, fmt.Sprintf("pwm%d_enable", fanIdx))
	return fs.WriteFile(pwmEnableFile, []byte(fmt.Sprintf("%d\n", mode)), 0644)
}

// IsHardwareCurveEnabled checks if custom curve mode (pwmX_enable == 1) is currently active.
func IsHardwareCurveEnabled(fs FileSystem, fanIdx int) (bool, error) {
	caps, err := DetectHardwareCurveSupport(fs)
	if err != nil || !caps.Supported {
		return false, ErrHardwareCurveUnsupported
	}

	if fanIdx < 1 || fanIdx > caps.FanCount {
		fanIdx = 1
	}

	pwmEnableFile := filepath.Join(caps.HwmonPath, fmt.Sprintf("pwm%d_enable", fanIdx))
	data, err := fs.ReadFile(pwmEnableFile)
	if err != nil {
		return false, err
	}

	val, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return false, err
	}

	return val == 1, nil
}
