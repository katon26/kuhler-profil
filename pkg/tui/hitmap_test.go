package tui_test

import (
	"testing"

	"kuhlerprofil/pkg/tui"
)

func TestDetectClickTarget(t *testing.T) {
	width := 80
	height := 24
	hasDualFan := false

	t.Run("Logo Click Full Width", func(t *testing.T) {
		target := tui.DetectClickTarget(10, 1, width, height, hasDualFan)
		if target != tui.TargetLogo {
			t.Errorf("expected TargetLogo, got %v", target)
		}
	})

	t.Run("Thermal & Fan Cards Refresh", func(t *testing.T) {
		targetThermal := tui.DetectClickTarget(10, 5, width, height, hasDualFan)
		if targetThermal != tui.TargetRefresh {
			t.Errorf("expected TargetRefresh on thermal card, got %v", targetThermal)
		}

		targetFan := tui.DetectClickTarget(50, 5, width, height, hasDualFan)
		if targetFan != tui.TargetRefresh {
			t.Errorf("expected TargetRefresh on fan card, got %v", targetFan)
		}
	})

	t.Run("Battery Card", func(t *testing.T) {
		target := tui.DetectClickTarget(15, 12, width, height, hasDualFan)
		if target != tui.TargetBattery {
			t.Errorf("expected TargetBattery, got %v", target)
		}
	})

	t.Run("Mode Pills in Profile Card", func(t *testing.T) {
		// modeCard starts at cardWidth + 1
		targetSilent := tui.DetectClickTarget(44, 13, width, height, hasDualFan)
		if targetSilent != tui.TargetSilent {
			t.Errorf("expected TargetSilent, got %v", targetSilent)
		}

		targetStd := tui.DetectClickTarget(55, 13, width, height, hasDualFan)
		if targetStd != tui.TargetStandard {
			t.Errorf("expected TargetStandard, got %v", targetStd)
		}

		targetBoost := tui.DetectClickTarget(70, 13, width, height, hasDualFan)
		if targetBoost != tui.TargetBoost {
			t.Errorf("expected TargetBoost, got %v", targetBoost)
		}
	})

	t.Run("Governor & Cooldown Badges", func(t *testing.T) {
		targetGov := tui.DetectClickTarget(46, 15, width, height, hasDualFan)
		if targetGov != tui.TargetGovernor {
			t.Errorf("expected TargetGovernor, got %v", targetGov)
		}

		targetCD := tui.DetectClickTarget(68, 15, width, height, hasDualFan)
		if targetCD != tui.TargetCooldown {
			t.Errorf("expected TargetCooldown, got %v", targetCD)
		}
	})

	t.Run("Compact Mode Hits", func(t *testing.T) {
		compactW := 60
		targetLogo := tui.DetectClickTarget(5, 1, compactW, height, hasDualFan)
		if targetLogo != tui.TargetLogo {
			t.Errorf("expected TargetLogo in compact, got %v", targetLogo)
		}

		targetThermal := tui.DetectClickTarget(10, 5, compactW, height, hasDualFan)
		if targetThermal != tui.TargetRefresh {
			t.Errorf("expected TargetRefresh in compact thermal, got %v", targetThermal)
		}
	})

	t.Run("Out of Bounds", func(t *testing.T) {
		target := tui.DetectClickTarget(5, 23, width, height, hasDualFan)
		if target != tui.TargetNone {
			t.Errorf("expected TargetNone on footer, got %v", target)
		}

		targetNeg := tui.DetectClickTarget(-1, -1, width, height, hasDualFan)
		if targetNeg != tui.TargetNone {
			t.Errorf("expected TargetNone on negative coords, got %v", targetNeg)
		}
	})
}
