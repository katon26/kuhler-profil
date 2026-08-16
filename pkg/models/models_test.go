package models_test

import (
	"testing"

	"koolthing/pkg/models"
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
