package tui

// ClickTarget represents a clickable interactive element on the TUI dashboard.
type ClickTarget string

const (
	TargetNone     ClickTarget = ""
	TargetLogo     ClickTarget = "logo"
	TargetSilent   ClickTarget = "silent"
	TargetStandard ClickTarget = "standard"
	TargetBoost    ClickTarget = "boost"
	TargetGovernor ClickTarget = "governor"
	TargetCooldown ClickTarget = "cooldown"
	TargetBattery  ClickTarget = "battery"
	TargetRefresh  ClickTarget = "refresh"
)

// Rect represents a rectangular bounding box in screen cell coordinates.
type Rect struct {
	MinX, MinY, MaxX, MaxY int
}

// Contains returns true if (x, y) falls inside the rectangle bounds.
func (r Rect) Contains(x, y int) bool {
	return x >= r.MinX && x <= r.MaxX && y >= r.MinY && y <= r.MaxY
}

// DetectClickTarget maps terminal cell coordinates (x, y) to a ClickTarget based on dashboard layout geometry.
func DetectClickTarget(x, y int, width, height int, hasDualFan bool) ClickTarget {
	if x < 0 || y < 0 || width <= 0 || height <= 0 {
		return TargetNone
	}

	isCompact := width < 75

	if !isCompact {
		// Non-compact (side-by-side) layout
		// 1. Header: Rows 0..2 (Logo banner is rows 0..1, subtitle row 2)
		if y >= 0 && y <= 2 && x <= 65 {
			return TargetLogo
		}

		cardWidth := (width - 6) / 2
		if cardWidth < 34 {
			cardWidth = 34
		}
		cardTotalWidth := cardWidth + 2 // card border (1 left, 1 right)

		headerH := 4
		row1H := 5
		if hasDualFan {
			row1H = 6
		}

		// Row 1: Thermal & Fan Cards
		if y >= headerH && y < headerH+row1H {
			if x >= 0 && x < width {
				return TargetRefresh
			}
		}

		// Row 2: Battery & Profile Cards
		row2Y := headerH + row1H + 1
		row2H := 7
		if y >= row2Y && y < row2Y+row2H {
			// Left Card: Battery
			if x >= 0 && x < cardTotalWidth {
				return TargetBattery
			}

			// Right Card: Mode & Governor
			if x >= cardTotalWidth && x < width {
				relX := x - cardTotalWidth
				// Pills Row: Y == row2Y + 3
				if y == row2Y+3 {
					if relX < cardTotalWidth/3 {
						return TargetSilent
					} else if relX < (cardTotalWidth*2)/3 {
						return TargetStandard
					} else {
						return TargetBoost
					}
				}
				// Badges Row: Y == row2Y + 5
				if y == row2Y+5 {
					if relX < cardTotalWidth/2 {
						return TargetGovernor
					} else {
						return TargetCooldown
					}
				}
			}
		}

		return TargetNone
	}

	// Compact (stacked) layout
	// 1. Header: Rows 0..1
	if y >= 0 && y <= 1 && x <= width {
		return TargetLogo
	}

	headerH := 3
	currY := headerH

	// Card 1: Thermal (height: 5)
	if y >= currY && y < currY+5 {
		return TargetRefresh
	}
	currY += 5

	// Card 2: Fan (height: 5 or 6)
	fanH := 5
	if hasDualFan {
		fanH = 6
	}
	if y >= currY && y < currY+fanH {
		return TargetRefresh
	}
	currY += fanH

	// Card 3: Battery (height: 5)
	if y >= currY && y < currY+5 {
		return TargetBattery
	}
	currY += 5

	// Card 4: Mode & Governor (height: 7)
	modeH := 7
	if y >= currY && y < currY+modeH {
		if y == currY+3 {
			if x < width/3 {
				return TargetSilent
			} else if x < (width*2)/3 {
				return TargetStandard
			} else {
				return TargetBoost
			}
		}
		if y == currY+5 {
			if x < width/2 {
				return TargetGovernor
			} else {
				return TargetCooldown
			}
		}
	}

	return TargetNone
}
