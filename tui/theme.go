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
	// Use the same colors as title backgrounds for a unified frame.
	PanelBorderActive   = TitleActiveBG   // SteelBlue
	PanelBorderInactive = TitleInactiveBG // Dark gray

	// Popup colors
	PopupBG      = lipgloss.Color("#2B2B2B") // Almost black
	PopupBorder  = PopupTitleBG              // Same as title background (dark orange)
	PopupTitleBG = lipgloss.Color("#FFA500") // Dark orange
	PopupTitleFG = lipgloss.Color("#FFFFFF") // White

	// General UI
	HintFG = lipgloss.Color("#AAAAAA") // Light gray
)
