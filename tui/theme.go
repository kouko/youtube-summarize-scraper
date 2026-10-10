package tui

import "charm.land/lipgloss/v2"

// Theme holds all color definitions for the TUI.
// Using hex color strings for compatibility with lipgloss v2.
var (
	// Title bar colors
	TitleActiveBG   = lipgloss.Color("#4A90D9") // SteelBlue
	TitleInactiveBG = lipgloss.Color("#444444") // Dark gray
	TitleFG         = lipgloss.Color("#FFFFFF") // White text

	// Panel border colors
	PanelBorderActive   = lipgloss.Color("#4A90D9") // SteelBlue
	PanelBorderInactive = lipgloss.Color("#555555") // Medium gray

	// Popup colors
	PopupBG      = lipgloss.Color("#2B2B2B") // Almost black
	PopupBorder  = lipgloss.Color("#888888") // Gray border
	PopupTitleBG = lipgloss.Color("#FFA500") // Dark orange
	PopupTitleFG = lipgloss.Color("#FFFFFF") // White

	// General UI
	HintFG = lipgloss.Color("#AAAAAA") // Light gray
)
