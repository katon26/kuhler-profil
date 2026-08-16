package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// RenderProgressBar builds an ANSI-styled bar with optional ceiling marker.
func RenderProgressBar(fraction float64, barWidth int, filledColor lipgloss.Color, emptyColor lipgloss.Color, markerFraction float64, markerChar string) string {
	if barWidth <= 0 {
		return ""
	}

	if fraction < 0.0 {
		fraction = 0.0
	} else if fraction > 1.0 {
		fraction = 1.0
	}

	filledLen := int(fraction * float64(barWidth))
	if filledLen > barWidth {
		filledLen = barWidth
	}

	markerPos := -1
	if markerFraction > 0.0 && markerFraction <= 1.0 && len(markerChar) > 0 {
		markerPos = int(markerFraction * float64(barWidth))
		if markerPos >= barWidth {
			markerPos = barWidth - 1
		}
	}

	filledStyle := lipgloss.NewStyle().Foreground(filledColor)
	emptyStyle := lipgloss.NewStyle().Foreground(emptyColor)
	markerStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorAmber)

	var sb strings.Builder
	sb.WriteString(lipgloss.NewStyle().Foreground(ColorBorder).Render("["))

	for i := 0; i < barWidth; i++ {
		if i == markerPos {
			sb.WriteString(markerStyle.Render(markerChar))
		} else if i < filledLen {
			sb.WriteString(filledStyle.Render("█"))
		} else {
			sb.WriteString(emptyStyle.Render("░"))
		}
	}

	sb.WriteString(lipgloss.NewStyle().Foreground(ColorBorder).Render("]"))
	return sb.String()
}

// RenderTempGauge renders a temperature progress bar with dynamic color grading.
func RenderTempGauge(temp float64, width int) string {
	if temp < 0.0 {
		temp = 0.0
	}

	// Dynamic color thresholds
	var tempColor lipgloss.Color
	var stateTag string

	switch {
	case temp < 50.0:
		tempColor = ColorEmerald
		stateTag = "COOL"
	case temp < 70.0:
		tempColor = ColorCyan
		stateTag = "NORM"
	case temp < 85.0:
		tempColor = ColorAmber
		stateTag = "WARM"
	default:
		tempColor = ColorCrimson
		stateTag = "HOT!"
	}

	// Bar width allocation
	numStr := fmt.Sprintf("%5.1f°C", temp)
	tagStr := fmt.Sprintf("[%s]", stateTag)

	// Available width for the bar itself
	overhead := len(numStr) + len(tagStr) + 4
	barWidth := width - overhead
	if barWidth < 6 {
		barWidth = 6
	}

	fraction := temp / 100.0
	bar := RenderProgressBar(fraction, barWidth, tempColor, ColorTrackEmpty, 0.0, "")

	valueStyled := lipgloss.NewStyle().Bold(true).Foreground(tempColor).Render(numStr)
	tagStyled := lipgloss.NewStyle().Foreground(tempColor).Render(tagStr)

	return fmt.Sprintf("%s %s %s", bar, valueStyled, tagStyled)
}

// RenderFanGauge renders a tachometer RPM gauge.
func RenderFanGauge(rpm int32, maxRPM int32, width int) string {
	if maxRPM <= 0 {
		maxRPM = 5500
	}
	if rpm < 0 {
		rpm = 0
	}

	pct := float64(rpm) / float64(maxRPM)
	displayPct := int(pct * 100.0)

	var fanColor lipgloss.Color
	switch {
	case pct < 0.35:
		fanColor = ColorEmerald
	case pct < 0.70:
		fanColor = ColorCyan
	case pct < 0.88:
		fanColor = ColorAmber
	default:
		fanColor = ColorCrimson
	}

	numStr := fmt.Sprintf("%4d RPM", rpm)
	pctStr := fmt.Sprintf("(%d%%)", displayPct)

	overhead := len(numStr) + len(pctStr) + 4
	barWidth := width - overhead
	if barWidth < 6 {
		barWidth = 6
	}

	bar := RenderProgressBar(pct, barWidth, fanColor, ColorTrackEmpty, 0.0, "")
	valueStyled := lipgloss.NewStyle().Bold(true).Foreground(ColorTextLight).Render(numStr)
	pctStyled := lipgloss.NewStyle().Foreground(fanColor).Render(pctStr)

	return fmt.Sprintf("%s %s %s", bar, valueStyled, pctStyled)
}

// RenderBatteryGauge renders battery charge level with limit ceiling marker and AC status.
func RenderBatteryGauge(percent int32, limit int32, onAC bool, width int) string {
	if percent < 0 {
		percent = 0
	} else if percent > 100 {
		percent = 100
	}
	if limit <= 0 || limit > 100 {
		limit = 100
	}

	fraction := float64(percent) / 100.0
	markerFraction := float64(limit) / 100.0

	var batColor lipgloss.Color
	switch {
	case percent <= 20:
		batColor = ColorCrimson
	case percent <= 50:
		batColor = ColorAmber
	default:
		batColor = ColorEmerald
	}

	acBadge := "🔋 BAT"
	if onAC {
		acBadge = "⚡ AC"
	}

	numStr := fmt.Sprintf("%3d%%", percent)
	limitStr := fmt.Sprintf("[Cap: %d%%]", limit)

	overhead := len(numStr) + len(acBadge) + len(limitStr) + 5
	barWidth := width - overhead
	if barWidth < 6 {
		barWidth = 6
	}

	markerChar := ""
	if limit < 100 {
		markerChar = "│"
	}

	bar := RenderProgressBar(fraction, barWidth, batColor, ColorTrackEmpty, markerFraction, markerChar)

	valueStyled := lipgloss.NewStyle().Bold(true).Foreground(batColor).Render(numStr)
	acStyled := ACOnlineBadge.Render(acBadge)
	if !onAC {
		acStyled = ACOfflineBadge.Render(acBadge)
	}
	limitStyled := lipgloss.NewStyle().Foreground(ColorTextMuted).Render(limitStr)

	return fmt.Sprintf("%s %s %s %s", bar, valueStyled, acStyled, limitStyled)
}
