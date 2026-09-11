package models_test

import (
	"testing"

	"kuhlerprofil/pkg/models"
)

func TestParseThermalMode(t *testing.T) {
	tests := []struct {
		input    string
		expected models.ThermalMode
		sysfs    int
		wantErr  bool
	}{
		{"standard", models.ModeStandard, 0, false},
		{"normal", models.ModeStandard, 0, false},
		{"balanced", models.ModeStandard, 0, false},
		{"0", models.ModeStandard, 0, false},
		{"boost", models.ModeBoost, 1, false},
		{"performance", models.ModeBoost, 1, false},
		{"overboost", models.ModeBoost, 1, false},
		{"1", models.ModeBoost, 1, false},
		{"silent", models.ModeSilent, 2, false},
		{"whisper", models.ModeSilent, 2, false},
		{"quiet", models.ModeSilent, 2, false},
		{"2", models.ModeSilent, 2, false},
		{" Standard ", models.ModeStandard, 0, false},
		{"BOOST", models.ModeBoost, 1, false},
		{"Silent", models.ModeSilent, 2, false},
		{"invalid", "", -1, true},
		{"99", "", -1, true},
	}

	for _, tt := range tests {
		mode, err := models.ParseThermalMode(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseThermalMode(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if !tt.wantErr {
			if mode != tt.expected {
				t.Errorf("ParseThermalMode(%q) = %v, want %v", tt.input, mode, tt.expected)
			}
			if mode.SysfsValue() != tt.sysfs {
				t.Errorf("SysfsValue for %v = %v, want %v", mode, mode.SysfsValue(), tt.sysfs)
			}
		}
	}
}

func TestThermalModeSysfsValueDefault(t *testing.T) {
	invalidMode := models.ThermalMode("unknown_mode")
	if invalidMode.SysfsValue() != 0 {
		t.Errorf("SysfsValue for unknown mode = %d, want 0", invalidMode.SysfsValue())
	}
}

func TestParseSysfsThermalMode(t *testing.T) {
	tests := []struct {
		input    int
		expected models.ThermalMode
	}{
		{0, models.ModeStandard},
		{1, models.ModeBoost},
		{2, models.ModeSilent},
		{99, models.ModeStandard},
		{-1, models.ModeStandard},
	}

	for _, tt := range tests {
		got := models.ParseSysfsThermalMode(tt.input)
		if got != tt.expected {
			t.Errorf("ParseSysfsThermalMode(%d) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func TestValidateBatteryThreshold(t *testing.T) {
	valid := []int32{60, 80, 100}
	for _, v := range valid {
		if err := models.ValidateBatteryLimit(v); err != nil {
			t.Errorf("ValidateBatteryLimit(%d) unexpected error: %v", v, err)
		}
	}

	invalid := []int32{0, 50, 75, 90, 105, -10}
	for _, v := range invalid {
		if err := models.ValidateBatteryLimit(v); err == nil {
			t.Errorf("ValidateBatteryLimit(%d) expected error, got nil", v)
		}
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := models.DefaultConfig()
	if cfg.DefaultMode != models.ModeStandard {
		t.Errorf("DefaultConfig().DefaultMode = %v, want %v", cfg.DefaultMode, models.ModeStandard)
	}
	if cfg.DefaultBatteryLimit != 80 {
		t.Errorf("DefaultConfig().DefaultBatteryLimit = %d, want 80", cfg.DefaultBatteryLimit)
	}
	if cfg.PollIntervalMs != 1500 {
		t.Errorf("DefaultConfig().PollIntervalMs = %d, want 1500", cfg.PollIntervalMs)
	}
	if cfg.AutoCurve != false {
		t.Errorf("DefaultConfig().AutoCurve = %v, want false", cfg.AutoCurve)
	}
}

func TestTelemetryStruct(t *testing.T) {
	telem := models.Telemetry{
		CPUTemp:        55.5,
		Fan1RPM:        2400,
		Fan2RPM:        2500,
		BatteryPercent: 82,
		BatteryLimit:   80,
		OnAC:           true,
		ActiveMode:     models.ModeStandard,
		AutoMode:       false,
	}

	if telem.CPUTemp != 55.5 || telem.Fan1RPM != 2400 || telem.Fan2RPM != 2500 ||
		telem.BatteryPercent != 82 || telem.BatteryLimit != 80 || !telem.OnAC ||
		telem.ActiveMode != models.ModeStandard || telem.AutoMode {
		t.Errorf("Telemetry struct fields mismatch: %+v", telem)
	}
}

func TestValidateCurvePoints(t *testing.T) {
	validPoints := []models.CurvePoint{
		{TempC: 40, PWM: 50, Mode: models.ModeSilent},
		{TempC: 55, PWM: 100, Mode: models.ModeSilent},
		{TempC: 70, PWM: 160, Mode: models.ModeStandard},
		{TempC: 85, PWM: 255, Mode: models.ModeBoost},
	}
	if err := models.ValidateCurvePoints(validPoints); err != nil {
		t.Errorf("ValidateCurvePoints(validPoints) unexpected error: %v", err)
	}

	// Too few points
	if err := models.ValidateCurvePoints([]models.CurvePoint{{TempC: 50, PWM: 100}}); err == nil {
		t.Errorf("ValidateCurvePoints with 1 point expected error, got nil")
	}

	// Temperature too low (< 30)
	tooLow := []models.CurvePoint{
		{TempC: 25, PWM: 50},
		{TempC: 60, PWM: 150},
	}
	if err := models.ValidateCurvePoints(tooLow); err == nil {
		t.Errorf("ValidateCurvePoints with temp < 30 expected error, got nil")
	}

	// Temperature too high (> 100)
	tooHigh := []models.CurvePoint{
		{TempC: 40, PWM: 50},
		{TempC: 105, PWM: 255},
	}
	if err := models.ValidateCurvePoints(tooHigh); err == nil {
		t.Errorf("ValidateCurvePoints with temp > 100 expected error, got nil")
	}

	// Non-monotonic temperatures
	nonMonotonic := []models.CurvePoint{
		{TempC: 50, PWM: 50},
		{TempC: 45, PWM: 100},
	}
	if err := models.ValidateCurvePoints(nonMonotonic); err == nil {
		t.Errorf("ValidateCurvePoints non-monotonic expected error, got nil")
	}

	// Duplicate temperatures
	duplicateTemp := []models.CurvePoint{
		{TempC: 50, PWM: 50},
		{TempC: 50, PWM: 100},
	}
	if err := models.ValidateCurvePoints(duplicateTemp); err == nil {
		t.Errorf("ValidateCurvePoints duplicate temp expected error, got nil")
	}

	// PWM negative
	negativePWM := []models.CurvePoint{
		{TempC: 40, PWM: -10},
		{TempC: 60, PWM: 100},
	}
	if err := models.ValidateCurvePoints(negativePWM); err == nil {
		t.Errorf("ValidateCurvePoints negative PWM expected error, got nil")
	}

	// PWM > 255
	overPWM := []models.CurvePoint{
		{TempC: 40, PWM: 50},
		{TempC: 60, PWM: 300},
	}
	if err := models.ValidateCurvePoints(overPWM); err == nil {
		t.Errorf("ValidateCurvePoints PWM > 255 expected error, got nil")
	}

	// Zero PWM above 55C
	zeroHotPWM := []models.CurvePoint{
		{TempC: 40, PWM: 50},
		{TempC: 65, PWM: 0},
	}
	if err := models.ValidateCurvePoints(zeroHotPWM); err == nil {
		t.Errorf("ValidateCurvePoints zero PWM at 65C expected error, got nil")
	}
}

func TestDefaultCurveProfiles(t *testing.T) {
	profiles := models.DefaultCurveProfiles()
	requiredProfiles := []string{"quiet", "balanced", "aggressive"}

	for _, name := range requiredProfiles {
		p, exists := profiles[name]
		if !exists {
			t.Errorf("DefaultCurveProfiles missing %q", name)
			continue
		}
		if p.Name != name {
			t.Errorf("Profile name mismatch: %q != %q", p.Name, name)
		}
		if err := models.ValidateCurvePoints(p.Points); err != nil {
			t.Errorf("Default profile %q has invalid points: %v", name, err)
		}
	}
}

func TestExpandPointsTo8(t *testing.T) {
	// Case 1: 3-point profile (e.g. aggressive) expands to 8 valid points
	aggPoints := []models.CurvePoint{
		{TempC: 35, PWM: 100, Mode: models.ModeStandard},
		{TempC: 50, PWM: 170, Mode: models.ModeBoost},
		{TempC: 68, PWM: 255, Mode: models.ModeBoost},
	}
	expanded := models.ExpandPointsTo8(aggPoints)
	if len(expanded) != 8 {
		t.Fatalf("ExpandPointsTo8 returned %d points, want 8", len(expanded))
	}
	if err := models.ValidateCurvePoints(expanded); err != nil {
		t.Fatalf("ExpandPointsTo8 produced invalid points: %v", err)
	}

	// Verify strict monotonicity
	for i := 1; i < len(expanded); i++ {
		if expanded[i].TempC <= expanded[i-1].TempC {
			t.Errorf("Points not strictly increasing: pt[%d]=%d <= pt[%d]=%d",
				i, expanded[i].TempC, i-1, expanded[i-1].TempC)
		}
		if expanded[i].PWM < expanded[i-1].PWM {
			t.Errorf("PWM decreased: pt[%d]=%d < pt[%d]=%d",
				i, expanded[i].PWM, i-1, expanded[i-1].PWM)
		}
	}

	// Case 2: 4-point profile (e.g. quiet)
	quietPoints := models.DefaultCurveProfiles()["quiet"].Points
	expandedQuiet := models.ExpandPointsTo8(quietPoints)
	if len(expandedQuiet) != 8 {
		t.Fatalf("ExpandPointsTo8 for quiet returned %d points, want 8", len(expandedQuiet))
	}
	if err := models.ValidateCurvePoints(expandedQuiet); err != nil {
		t.Fatalf("ExpandPointsTo8 for quiet produced invalid points: %v", err)
	}

	// Case 3: Exactly 8 points returns identical points
	exact8 := expandedQuiet
	same8 := models.ExpandPointsTo8(exact8)
	if len(same8) != 8 {
		t.Fatalf("ExpandPointsTo8 with 8 points returned %d points", len(same8))
	}
	for i := range exact8 {
		if same8[i] != exact8[i] {
			t.Errorf("pt[%d] changed: %+v != %+v", i, same8[i], exact8[i])
		}
	}

	// Case 4: Empty points returns 8 valid points from default balanced
	emptyExpanded := models.ExpandPointsTo8(nil)
	if len(emptyExpanded) != 8 {
		t.Fatalf("ExpandPointsTo8(nil) returned %d points, want 8", len(emptyExpanded))
	}
	if err := models.ValidateCurvePoints(emptyExpanded); err != nil {
		t.Fatalf("ExpandPointsTo8(nil) produced invalid points: %v", err)
	}
}

