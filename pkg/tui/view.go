package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"kuhlerprofil/pkg/models"
)

// RenderDashboard renders the entire KühlerProfil terminal dashboard with the default theme.
func RenderDashboard(status models.Telemetry, width int, height int) string {
	return RenderDashboardWithHover(status, width, height, 0, TargetNone)
}

// RenderDashboardWithTheme renders the KühlerProfil terminal dashboard with a specific theme.
func RenderDashboardWithTheme(status models.Telemetry, width int, height int, themeIndex int) string {
	return RenderDashboardWithHover(status, width, height, themeIndex, TargetNone)
}

// RenderDashboardWithHover renders the dashboard with theme and interactive hover highlighting.
func RenderDashboardWithHover(status models.Telemetry, width int, height int, themeIndex int, hoverTarget ClickTarget) string {
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}

	isCompact := width < 75

	// 1. Header Section
	headerTitle := " KühlerProfil ASUS Control "
	if hoverTarget == TargetLogo {
		headerTitle = " KühlerProfil [Click to Cycle Theme] "
	}
	var header string
	if isCompact {
		header = lipgloss.JoinVertical(
			lipgloss.Left,
			LogoBannerWithTheme(true, themeIndex),
			SubtitleStyle.Render(headerTitle),
			"",
		)
	} else {
		logo := LogoBannerWithTheme(false, themeIndex)
		sub := SubtitleStyle.Render(headerTitle)
		header = lipgloss.JoinVertical(lipgloss.Left, logo, sub, "")
	}

	// 2. Panels
	cardWidth := width - 4
	if !isCompact {
		cardWidth = (width - 6) / 2
		if cardWidth < 34 {
			cardWidth = 34
		}
	}

	// Panel A: CPU Thermal
	thermalTitle := CardHeaderStyle.Render("🔥 THERMAL STATUS")
	statusTag := "NORM"
	tempColor := ColorEmerald
	if status.CPUTemp >= 85 {
		statusTag = "CRIT"
		tempColor = ColorCrimson
	} else if status.CPUTemp >= 70 {
		statusTag = "WARM"
		tempColor = ColorAmber
	}
	tempText := fmt.Sprintf("CPU Package: %5.1f°C [%s]", status.CPUTemp, statusTag)
	tempBar := RenderProgressBar(status.CPUTemp/100.0, cardWidth-4, tempColor, ColorTrackEmpty, 0.0, "")
	thermalContent := lipgloss.JoinVertical(
		lipgloss.Left,
		thermalTitle,
		"",
		lipgloss.NewStyle().Foreground(ColorTextLight).Render(tempText),
		tempBar,
	)
	thermalCard := CardStyle.Width(cardWidth).Render(thermalContent)

	// Panel B: Fan Tachometers
	fanTitle := CardHeaderStyle.Render("🌀 FAN TACHOMETER")
	fanPct := int(float64(status.Fan1RPM) / 5500.0 * 100.0)
	fan1Text := fmt.Sprintf("CPU Fan: %4d RPM (%d%%)", status.Fan1RPM, fanPct)
	fan1Bar := RenderProgressBar(float64(status.Fan1RPM)/5500.0, cardWidth-4, ColorCyan, ColorTrackEmpty, 0.0, "")
	var fanRows []string
	fanRows = append(fanRows, fanTitle, "", lipgloss.NewStyle().Foreground(ColorTextLight).Render(fan1Text), fan1Bar)
	if status.Fan2RPM > 0 {
		fan2Pct := int(float64(status.Fan2RPM) / 5500.0 * 100.0)
		fan2Text := fmt.Sprintf("GPU Fan: %4d RPM (%d%%)", status.Fan2RPM, fan2Pct)
		fan2Bar := RenderProgressBar(float64(status.Fan2RPM)/5500.0, cardWidth-4, ColorCyan, ColorTrackEmpty, 0.0, "")
		fanRows = append(fanRows, lipgloss.NewStyle().Foreground(ColorTextLight).Render(fan2Text), fan2Bar)
	}
	fanCard := CardStyle.Width(cardWidth).Render(lipgloss.JoinVertical(lipgloss.Left, fanRows...))

	// Panel C: Battery & Power
	batTitle := CardHeaderSecondary.Render("⚡ POWER & BATTERY")
	if hoverTarget == TargetBattery {
		batTitle = lipgloss.NewStyle().Bold(true).Foreground(ColorCyan).Render("⚡ POWER & BATTERY [Click to Cap]")
	}
	acBadge := "🔋 BAT"
	if status.OnAC {
		acBadge = "⚡ AC"
	}
	batText := fmt.Sprintf("Charge: %d%%  %s  [Cap: %d%%]", status.BatteryPercent, acBadge, status.BatteryLimit)
	if hoverTarget == TargetBattery {
		batText = fmt.Sprintf("Charge: %d%%  %s  %s", status.BatteryPercent, acBadge, BadgeHover.Render(fmt.Sprintf("[Cap: %d%% ↻]", status.BatteryLimit)))
	}
	batBar := RenderProgressBar(float64(status.BatteryPercent)/100.0, cardWidth-4, ColorEmerald, ColorTrackEmpty, float64(status.BatteryLimit)/100.0, "│")
	careText := fmt.Sprintf("Hardware Health: %d%% Cap", status.BatteryLimit)

	batCardStyle := CardStyle
	if hoverTarget == TargetBattery {
		batCardStyle = CardStyleHover
	}
	batContent := lipgloss.JoinVertical(
		lipgloss.Left,
		batTitle,
		"",
		lipgloss.NewStyle().Foreground(ColorTextLight).Render(batText),
		batBar,
		"",
		MetricLabelStyle.Render(careText),
	)
	batCard := batCardStyle.Width(cardWidth).Render(batContent)


	// Panel D: Thermal Profile & Governor Mode
	modeTitle := CardHeaderSecondary.Render("🎮 PROFILE & GOVERNOR")
	silentPill := ModePillWithHover(models.ModeSilent, status.ActiveMode, hoverTarget == TargetSilent)
	standardPill := ModePillWithHover(models.ModeStandard, status.ActiveMode, hoverTarget == TargetStandard)
	boostPill := ModePillWithHover(models.ModeBoost, status.ActiveMode, hoverTarget == TargetBoost)
	pillsRow := lipgloss.JoinHorizontal(lipgloss.Left, silentPill, " ", standardPill, " ", boostPill)

	profName := status.ActiveCurveProfile
	if profName == "" {
		profName = "balanced"
	}
	profFormatted := strings.ToUpper(profName[:1]) + profName[1:]

	var autoGovBadge string
	if status.AutoMode {
		if hoverTarget == TargetGovernor {
			autoGovBadge = BadgeHover.Render(fmt.Sprintf("⚡ AUTO-CURVE: %s [TOGGLE]", strings.ToUpper(profName)))
		} else {
			autoGovBadge = AutoGovernorOn.Render(fmt.Sprintf("⚡ AUTO-CURVE: %s", strings.ToUpper(profName)))
		}
	} else {
		if hoverTarget == TargetGovernor {
			autoGovBadge = BadgeHover.Render(fmt.Sprintf("○ GOVERNOR: OFF (%s) [TOGGLE]", profFormatted))
		} else {
			autoGovBadge = AutoGovernorOff.Render(fmt.Sprintf("○ GOVERNOR: OFF (%s)", profFormatted))
		}
	}

	cdMode := status.CooldownMode
	if cdMode == "" {
		cdMode = models.CooldownKick
	}
	var cdBadge string
	cdText := fmt.Sprintf("❄ COOLDOWN: %s", strings.ToUpper(string(cdMode)))
	if hoverTarget == TargetCooldown {
		cdBadge = BadgeHover.Render(cdText + " [ACTIVATE]")
	} else {
		cdBadge = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")).
			Bold(true).
			Render(cdText)
	}

	modeContent := lipgloss.JoinVertical(
		lipgloss.Left,
		modeTitle,
		"",
		pillsRow,
		"",
		autoGovBadge,
		cdBadge,
	)
	modeCard := CardStyle.Width(cardWidth).Render(modeContent)


	// 3. Assemble Layout
	var body string
	if isCompact {
		body = lipgloss.JoinVertical(
			lipgloss.Left,
			thermalCard,
			fanCard,
			batCard,
			modeCard,
		)
	} else {
		row1 := lipgloss.JoinHorizontal(lipgloss.Top, thermalCard, " ", fanCard)
		row2 := lipgloss.JoinHorizontal(lipgloss.Top, batCard, " ", modeCard)
		body = lipgloss.JoinVertical(lipgloss.Left, row1, "", row2)
	}

	// 4. Help / Keybindings Footer
	k1 := HelpKeyStyle.Render("[1]") + HelpDescStyle.Render(" Silent")
	k2 := HelpKeyStyle.Render("[2]") + HelpDescStyle.Render(" Standard")
	k3 := HelpKeyStyle.Render("[3]") + HelpDescStyle.Render(" Boost")
	kb := HelpKeyStyle.Render("[b]") + HelpDescStyle.Render(" Bat Limit")
	ka := HelpKeyStyle.Render("[a]") + HelpDescStyle.Render(" Auto")
	kc := HelpKeyStyle.Render("[c]") + HelpDescStyle.Render(" Cooldown")
	kq := HelpKeyStyle.Render("[q]") + HelpDescStyle.Render(" Quit")

	helpItems := []string{k1, k2, k3, kb, ka, kc, kq}
	helpRow := strings.Join(helpItems, HelpDivider.String())
	footer := HelpBarStyle.Render(helpRow)

	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		body,
		"",
		footer,
	)
}
