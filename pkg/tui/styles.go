package tui

import (
	"github.com/charmbracelet/lipgloss"

	"kuhlerprofil/pkg/models"
)

// Color Palette definitions
var (
	// Brand and Accent Colors
	ColorCyan    = lipgloss.Color("#00D7D7") // Electric Cyan
	ColorCyanDim = lipgloss.Color("#0891B2") // Deep Cyan
	ColorIndigo  = lipgloss.Color("#6366F1") // Indigo Accent
	ColorViolet  = lipgloss.Color("#8B5CF6") // Violet Gradient Accent

	// Status Colors
	ColorEmerald = lipgloss.Color("#10B981") // Cool / AC / Success
	ColorAmber   = lipgloss.Color("#F59E0B") // Warm / High / Warning
	ColorCrimson = lipgloss.Color("#EF4444") // Hot / Critical / Boost
	ColorMuted   = lipgloss.Color("#64748B") // Slate 500

	// Surfaces and Borders
	ColorBorder     = lipgloss.Color("#334155") // Slate 700
	ColorBorderDim  = lipgloss.Color("#1E293B") // Slate 800
	ColorCardBg     = lipgloss.Color("#0F172A") // Slate 900
	ColorTrackEmpty = lipgloss.Color("#1E293B") // Slate 800
	ColorTextLight  = lipgloss.Color("#F8FAFC") // Slate 50
	ColorTextMuted  = lipgloss.Color("#94A3B8") // Slate 400
)

// Lipgloss Styles
var (
	// Header & Banner Styles
	BannerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorCyan)

	SubtitleStyle = lipgloss.NewStyle().
			Foreground(ColorIndigo).
			Bold(false)

	DaemonBadgeOnline = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(ColorEmerald).
				Padding(0, 1)

	DaemonBadgeOffline = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(ColorCrimson).
				Padding(0, 1)

	// Card Styles
	CardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder).
			Padding(0, 1)

	CardHeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorCyan)

	CardHeaderSecondary = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorIndigo)

	// Gauge & Metric Styles
	MetricLabelStyle = lipgloss.NewStyle().
				Foreground(ColorTextMuted).
				Width(14)

	MetricValueStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorTextLight)

	// Mode Pill Badges
	PillSilentActive = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#0F172A")).
				Background(ColorEmerald).
				Padding(0, 1)

	PillStandardActive = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#0F172A")).
				Background(ColorCyan).
				Padding(0, 1)

	PillBoostActive = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(ColorCrimson).
			Padding(0, 1)

	PillInactive = lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Background(ColorBorderDim).
			Padding(0, 1)

	PillKeyBadge = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorCyan)

	// Governor & AC Indicators
	AutoGovernorOn = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#0F172A")).
			Background(ColorViolet).
			Padding(0, 1)

	AutoGovernorOff = lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Background(ColorBorderDim).
			Padding(0, 1)

	ACOnlineBadge = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#0F172A")).
			Background(ColorEmerald).
			Padding(0, 1)

	ACOfflineBadge = lipgloss.NewStyle().
			Foreground(ColorAmber).
			Background(ColorBorderDim).
			Padding(0, 1)

	// Toast & Notification
	ToastStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorCyan).
			Padding(0, 1)

	ErrorToastStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorCrimson).
			Padding(0, 1)

	// Help / Footer Keybindings
	HelpBarStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), true, false, false, false).
			BorderForeground(ColorBorderDim).
			Padding(0, 1)

	HelpKeyStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorCyan)

	HelpDescStyle = lipgloss.NewStyle().
			Foreground(ColorTextMuted)

	HelpDivider = lipgloss.NewStyle().
			Foreground(ColorBorder).
			SetString(" │ ")
)

// ModePill returns the styled pill representation for a given thermal mode based on active state.
func ModePill(mode models.ThermalMode, activeMode models.ThermalMode) string {
	isActive := mode == activeMode
	switch mode {
	case models.ModeSilent:
		if isActive {
			return PillSilentActive.Render("● 1: Silent")
		}
		return PillInactive.Render("○ 1: Silent")
	case models.ModeStandard:
		if isActive {
			return PillStandardActive.Render("● 2: Standard")
		}
		return PillInactive.Render("○ 2: Standard")
	case models.ModeBoost:
		if isActive {
			return PillBoostActive.Render("● 3: Boost")
		}
		return PillInactive.Render("○ 3: Boost")
	default:
		if isActive {
			return PillStandardActive.Render(string(mode))
		}
		return PillInactive.Render(string(mode))
	}
}

// Theme defines a color theme for the TUI dashboard header.
type Theme struct {
	Name  string
	Color lipgloss.Color
}

// AvailableThemes provides the color themes available for cycling.
var AvailableThemes = []Theme{
	{Name: "Electric Cyan", Color: ColorCyan},
	{Name: "Cyberpunk Violet", Color: ColorViolet},
	{Name: "Emerald Green", Color: ColorEmerald},
	{Name: "Crimson Red", Color: ColorCrimson},
	{Name: "Amber Gold", Color: ColorAmber},
}

// LogoBannerWithTheme returns the styled ASCII art banner for a specific theme index.
func LogoBannerWithTheme(compact bool, themeIndex int) string {
	if themeIndex < 0 || themeIndex >= len(AvailableThemes) {
		themeIndex = 0
	}
	style := lipgloss.NewStyle().Bold(true).Foreground(AvailableThemes[themeIndex].Color)

	if compact {
		return style.Render("⚡ KÜHLERPROFIL") + " " + SubtitleStyle.Render(":: ASUS Control Suite")
	}

	rawLogo := `█▄▀ █ █ █ █ █   █▀▀ █▀█   █▀█ █▀█ █▀█ █▀▀ ▀█▀ █     KÜHLERPROFIL
█ █ █▄█ █▀█ █▄▄ ██▄ █▀▄   █▀▀ █▀▄ █▄█ █▀  ▄█▄ █▄▄   ASUS Control`

	return style.Render(rawLogo)
}

// LogoBanner returns the ASCII art banner with default styling.
func LogoBanner(compact bool) string {
	return LogoBannerWithTheme(compact, 0)
}


