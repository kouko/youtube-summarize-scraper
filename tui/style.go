package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// TitleStyle returns a style for title bars.
// active: true for focused panel/popup, false otherwise.
func TitleStyle(active bool) lipgloss.Style {
	bg := TitleInactiveBG
	if active {
		bg = TitleActiveBG
	}
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(TitleFG).
		Background(bg).
		Padding(0, 1) // Left/right padding
}

// TitleWithMarker adds the focus marker prefix ("> ") and a black square (■) before the title.
// Format: [active? "> " : ""] + "■ " + title
func TitleWithMarker(active bool, title string) string {
	if active {
		return "> ■ " + title
	}
	return "■ " + title
}

func PanelBorderStyle(active bool) lipgloss.Style {
	borderColor := PanelBorderInactive
	if active {
		borderColor = PanelBorderActive
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor)
}

func PopupStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(PopupBorder).
		Background(PopupBG)
}

func PopupTitleStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(PopupTitleFG).
		Background(PopupTitleBG).
		Padding(0, 1)
}

// RenderPanelWithTitle renders a panel with a seamless title bar using half-block elements.
// The title bar uses ▐ (left) and ▌ (right) half-blocks that connect smoothly with
// the vertical │ borders below.
//
//	title   : title string (without the focus marker)
//	content : inner content string (may contain newlines)
//	w, h    : total width and height **including** the outer border
//	active  : true if the panel is focused
func RenderPanelWithTitle(title string, content string, w, h int, active bool) string {
	innerW := w - 2 // inner width (excluding left/right borders)
	innerH := h - 2 // inner height (excluding top/bottom borders)

	if innerW <= 0 || innerH <= 0 {
		return ""
	}

	/* ---------- Shared colors: title bar and border use the same palette ---------- */
	titleBG := TitleInactiveBG
	borderColor := PanelBorderInactive
	if active {
		titleBG = TitleActiveBG
		borderColor = PanelBorderActive
	}
	// The half-block glyphs (▐▌) are filled shapes: their FOREGROUND is the
	// fill color, so paint them with the title background color to make the
	// title bar look like one continuous strip.
	halfBlock := lipgloss.NewStyle().Foreground(titleBG)
	leftCap := halfBlock.Render("▐")
	rightCap := halfBlock.Render("▌")
	// Border glyphs (│ └─┘) share the panel border color for a uniform frame.
	bd := lipgloss.NewStyle().Foreground(borderColor)
	vBar := bd.Render("│")

	/* ---------- 1. Title row with half-block elements ---------- */
	titleStr := TitleWithMarker(active, title)
	// TitleStyle pads 1 column each side; lipgloss Width pads to innerW but
	// never truncates, so clip an overlong title first.
	if lipgloss.Width(titleStr)+2 > innerW {
		titleStr = truncateANSI(titleStr, innerW-2)
	}
	titleArea := TitleStyle(active).Width(innerW).Render(titleStr)

	var lines []string

	// Row 0: title bar with half-block elements. "▐" + innerW + "▌" is
	// exactly w columns: the half-blocks replace the top border and connect
	// seamlessly with the vertical │ borders below.
	lines = append(lines, leftCap+titleArea+rightCap)

	/* ---------- Content area (starts immediately after title row) ---------- */
	contentLines := strings.Split(content, "\n")
	// We have used 1 row for title; remaining rows = innerH - 1
	rows := innerH - 1
	for i := 0; i < rows; i++ {
		var line string
		if i < len(contentLines) {
			line = contentLines[i]
			// Every content row must be exactly innerW wide so the right
			// border stays in one column on every row: truncate overlong
			// rows and pad short ones, both display-width aware. Viewport
			// output already is innerW wide, so this is a no-op for
			// scrolling panels; plain-text panels (config card, status)
			// need the padding.
			line = truncateANSI(line, innerW)
			if pad := innerW - visibleWidth(line); pad > 0 {
				line += strings.Repeat(" ", pad)
			}
		} else {
			line = strings.Repeat(" ", innerW)
		}
		lines = append(lines, vBar+line+vBar)
	}

	// Bottom border line
	lines = append(lines, bd.Render("└"+strings.Repeat("─", innerW)+"┘"))

	return strings.Join(lines, "\n")
}

// RenderPopupWithTitle renders a popup whose title bar uses half-block elements.
// The left/right border lines start directly below the title bar (no top corners).
func RenderPopupWithTitle(title string, content string, w, h int) string {
	innerW := w - 2
	innerH := h - 2

	if innerW <= 0 || innerH <= 0 {
		return ""
	}

	/* ---------- Shared colors: title bar and border use the same palette ---------- */
	halfBlock := lipgloss.NewStyle().Foreground(PopupTitleBG)
	leftCap := halfBlock.Render("▐")
	rightCap := halfBlock.Render("▌")
	bd := lipgloss.NewStyle().Foreground(PopupBorder)
	vBar := bd.Render("│")

	titleArea := PopupTitleStyle().Width(innerW).Render(title)
	if lipgloss.Width(title)+2 > innerW {
		title = truncateANSI(title, innerW-2)
		titleArea = PopupTitleStyle().Width(innerW).Render(title)
	}

	var lines []string

	// Row 0: title bar with half-block elements.
	lines = append(lines, leftCap+titleArea+rightCap)

	/* ---------- Content area (starts immediately after title row) ---------- */
	contentLines := strings.Split(content, "\n")
	// We have used 1 row for title; remaining rows = innerH - 1
	rows := innerH - 1
	for i := 0; i < rows; i++ {
		var line string
		if i < len(contentLines) {
			line = contentLines[i]
			line = truncateANSI(line, innerW)
			if pad := innerW - visibleWidth(line); pad > 0 {
				line += strings.Repeat(" ", pad)
			}
		} else {
			line = strings.Repeat(" ", innerW)
		}
		lines = append(lines, vBar+line+vBar)
	}

	// Bottom border
	lines = append(lines, bd.Render("└"+strings.Repeat("─", innerW)+"┘"))

	return strings.Join(lines, "\n")
}

// truncateByWidth truncates a string to the specified visual width.
func truncateByWidth(s string, width int) string {
	if lipgloss.Width(s) <= width {
		return s
	}
	// Binary search for the longest prefix that fits within width
	low, high := 0, len(s)
	for low < high {
		mid := (low + high + 1) / 2
		if lipgloss.Width(s[:mid]) <= width {
			low = mid
		} else {
			high = mid - 1
		}
	}
	return s[:low]
}

// HintStyle returns a style for the hint line at the bottom.
func HintStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(HintFG)
}
