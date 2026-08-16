package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"koolthing/pkg/models"
)

// RenderDashboard renders the entire KoolThing terminal dashboard.
func RenderDashboard(status models.Telemetry, width int, height int) string {
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}

	isCompact := width < 75

	// 1. Header Section
	var header string
	if isCompact {
		header = lipgloss.JoinVertical(
			lipgloss.Left,
			LogoBanner(true),
			"",
		)
	} else {
		logo := LogoBanner(false)
		sub := SubtitleStyle.Render("ASUS VivoBook Linux Thermal & Battery Control Suite")
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

	gaugeInnerWidth := cardWidth - 6
	if gaugeInnerWidth < 18 {
		gaugeInnerWidth = 18
	}

	// Panel A: CPU Thermal
	thermalTitle := CardHeaderStyle.Render("🔥 THERMAL STATUS")
	tempGauge := RenderTempGauge(status.CPUTemp, gaugeInnerWidth)
	thermalContent := lipgloss.JoinVertical(
		lipgloss.Left,
		thermalTitle,
		"",
		fmt.Sprintf("%s %s", MetricLabelStyle.Render("CPU Package:"), tempGauge),
	)
	thermalCard := CardStyle.Width(cardWidth).Render(thermalContent)

	// Panel B: Fan Tachometers
	fanTitle := CardHeaderStyle.Render("🌀 FAN TACHOMETER")
	fan1Gauge := RenderFanGauge(status.Fan1RPM, 5500, gaugeInnerWidth)
	var fanRows []string
	fanRows = append(fanRows, fanTitle, "", fmt.Sprintf("%s %s", MetricLabelStyle.Render("CPU Fan:"), fan1Gauge))
	if status.Fan2RPM > 0 {
		fan2Gauge := RenderFanGauge(status.Fan2RPM, 5500, gaugeInnerWidth)
		fanRows = append(fanRows, fmt.Sprintf("%s %s", MetricLabelStyle.Render("GPU Fan:"), fan2Gauge))
	}
	fanCard := CardStyle.Width(cardWidth).Render(lipgloss.JoinVertical(lipgloss.Left, fanRows...))

	// Panel C: Battery & Power
	batTitle := CardHeaderSecondary.Render("⚡ POWER & BATTERY")
	batGauge := RenderBatteryGauge(status.BatteryPercent, status.BatteryLimit, status.OnAC, gaugeInnerWidth)
	batContent := lipgloss.JoinVertical(
		lipgloss.Left,
		batTitle,
		"",
		fmt.Sprintf("%s %s", MetricLabelStyle.Render("Charge Level:"), batGauge),
	)
	batCard := CardStyle.Width(cardWidth).Render(batContent)

	// Panel D: Thermal Profile & Governor Mode
	modeTitle := CardHeaderSecondary.Render("🎮 PROFILE & GOVERNOR")
	silentPill := ModePill(models.ModeSilent, status.ActiveMode)
	standardPill := ModePill(models.ModeStandard, status.ActiveMode)
	boostPill := ModePill(models.ModeBoost, status.ActiveMode)
	pillsRow := lipgloss.JoinHorizontal(lipgloss.Center, silentPill, "  ", standardPill, "  ", boostPill)

	var autoGovBadge string
	if status.AutoMode {
		autoGovBadge = AutoGovernorOn.Render("⚡ AUTO-CURVE GOVERNOR: ACTIVE")
	} else {
		autoGovBadge = AutoGovernorOff.Render("○ AUTO-CURVE GOVERNOR: DISABLED")
	}

	modeContent := lipgloss.JoinVertical(
		lipgloss.Left,
		modeTitle,
		"",
		pillsRow,
		"",
		autoGovBadge,
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
	kb := HelpKeyStyle.Render("[b]") + HelpDescStyle.Render(" Cycle Bat Limit")
	ka := HelpKeyStyle.Render("[a]") + HelpDescStyle.Render(" Toggle Auto")
	kq := HelpKeyStyle.Render("[q]") + HelpDescStyle.Render(" Quit")

	helpItems := []string{k1, k2, k3, kb, ka, kq}
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
