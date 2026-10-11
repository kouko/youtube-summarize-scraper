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

// HintSegment is one piece of a panel hint row: the text shown and the
// keyboard key a mouse click on it triggers ("" = informational, not
// clickable).
type HintSegment struct {
	Text string
	Key  string
}

// placedHintWord is one word of a hint segment placed on a wrapped hint line.
// Columns are 0-based relative to the hint text area (after the border and
// left padding).
type placedHintWord struct {
	seg      int
	text     string
	startCol int
	endCol   int
}

// layoutHintSegments word-wraps hint segments into lines of at most
// textAreaW columns (display-width aware) and caps the result at maxLines
// (at least one). Words are separated by one column, segments by two. The
// placed-word geometry is shared by rendering and by mouse hit tests, so a
// click position maps back to the exact segment under the cursor.
func layoutHintSegments(segs []HintSegment, textAreaW, maxLines int) [][]placedHintWord {
	if maxLines < 1 {
		maxLines = 1
	}
	if textAreaW < 1 {
		textAreaW = 1
	}
	var lines [][]placedHintWord
	var cur []placedHintWord
	curCol := 0
	flush := func() {
		if len(cur) > 0 {
			lines = append(lines, cur)
		}
		cur = nil
		curCol = 0
	}
	for si, seg := range segs {
		for wi, word := range strings.Fields(seg.Text) {
			// One column between words of a segment, two between segments;
			// no gap when a word starts a fresh line.
			gap := 1
			if wi == 0 && si > 0 && curCol > 0 {
				gap = 2
			}
			if lipgloss.Width(word) > textAreaW {
				word = truncateANSI(word, textAreaW)
			}
			w := lipgloss.Width(word)
			need := w
			if curCol > 0 {
				need += gap
			}
			if len(cur) > 0 && curCol+need > textAreaW {
				flush()
			}
			start := 0
			if curCol > 0 {
				start = curCol + gap
			}
			cur = append(cur, placedHintWord{seg: si, text: word, startCol: start, endCol: start + w})
			curCol = start + w
		}
	}
	flush()
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	return lines
}

// renderPlacedHintLine builds one styled hint row from placed words: words of
// clickable segments are bright, informational ones dim, and the row is
// padded to exactly innerW columns so the right border never drifts.
func renderPlacedHintLine(words []placedHintWord, segs []HintSegment, innerW int) string {
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("#AAAAAA"))
	bright := lipgloss.NewStyle().Foreground(lipgloss.Color("#E0E0E0"))
	var b strings.Builder
	col := 0
	for _, pw := range words {
		if pw.startCol > col {
			b.WriteString(strings.Repeat(" ", pw.startCol-col))
		}
		st := dim
		if segs[pw.seg].Key != "" {
			st = bright
		}
		b.WriteString(st.Render(pw.text))
		col = pw.endCol
	}
	line := b.String()
	if pad := innerW - lipgloss.Width(line); pad > 0 {
		line += strings.Repeat(" ", pad)
	}
	return line
}

// RenderPanelWithTitle renders a panel with a single informational hint
// string (not clickable). See RenderPanelWithSegments.
func RenderPanelWithTitle(title string, content string, w, h int, active bool, hint string) string {
	var segs []HintSegment
	if hint != "" {
		segs = []HintSegment{{Text: hint}}
	}
	return RenderPanelWithSegments(title, content, w, h, active, segs)
}

// RenderPanelWithSegments renders a panel with a seamless title bar using
// half-block elements. The title bar uses ▐ (left) and ▌ (right) half-blocks
// that connect smoothly with the vertical │ borders below.
//
//	title   : title string (without the focus marker)
//	content : inner content string (may contain newlines)
//	w, h    : total width and height **including** the outer border
//	active  : true if the panel is focused
//	segs    : hint segments shown below the title bar; segments with a Key
//	          render bright and are clickable (see layoutHintSegments)
//
// Long hints word-wrap to extra lines, every hint row is padded to exactly
// the inner width so borders line up, and the content area shrinks by the
// wrapped line count. The title bar stays truncated.
func RenderPanelWithSegments(title string, content string, w, h int, active bool, segs []HintSegment) string {
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

	// Hint rows: a hint longer than the text area wraps to additional lines;
	// every hint row still spans exactly innerW so the right border stays in
	// one column, and the content area shrinks by the wrapped line count.
	hintRows := 0
	if len(segs) > 0 {
		hintWords := layoutHintSegments(segs, innerW-2, innerH-3)
		for _, words := range hintWords {
			lines = append(lines, vBar+renderPlacedHintLine(words, segs, innerW)+vBar)
			hintRows++
		}

		// Separator row: a thin line of upper half blocks (▔) with very subtle color
		sepStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#2A2A2A")) // even more subtle than border
		sepLine := sepStyle.Render(strings.Repeat("▔", innerW))
		lines = append(lines, vBar+sepLine+vBar)
		hintRows++
	}

	/* ---------- Content area (starts after title and hint rows) ---------- */
	contentLines := strings.Split(content, "\n")
	// Rows used so far: 1 (title) + hintRows (hint + separator); the bottom
	// border takes one more, so the content gets innerH - hintRows rows and
	// the panel totals exactly h rows (title + inner + border).
	rows := innerH - hintRows
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
// Optional hint string is rendered below the title bar, wrapped to multiple lines
// if needed, followed by a subtle separator line. Content area shrinks accordingly.
// popupHintLineCount returns how many rows the hint occupies inside a popup
// of the given outer size when rendered by RenderPopupWithTitle (0 when the
// hint is empty). The separator row is not counted.
func popupHintLineCount(hint string, w, h int) int {
	if hint == "" {
		return 0
	}
	innerW, innerH := w-2, h-2
	if innerW <= 0 || innerH <= 0 {
		return 0
	}
	// Same segment/width/height arguments as RenderPopupWithTitle, so the
	// count always matches the rendered rows.
	return len(layoutHintSegments([]HintSegment{{Text: hint}}, innerW-2, innerH-3))
}

func RenderPopupWithTitle(title string, content string, w, h int, hint string) string {
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

	// Hint rows (optional)
	hintRows := 0
	if hint != "" {
		segs := []HintSegment{{Text: hint}}
		hintWords := layoutHintSegments(segs, innerW-2, innerH-3)
		for _, words := range hintWords {
			lines = append(lines, vBar+renderPlacedHintLine(words, segs, innerW)+vBar)
			hintRows++
		}
		// Separator row
		sepStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#2A2A2A"))
		sepLine := sepStyle.Render(strings.Repeat("▔", innerW))
		lines = append(lines, vBar+sepLine+vBar)
		hintRows++
	}

	/* ---------- Content area (starts after title and hint rows) ---------- */
	contentLines := strings.Split(content, "\n")
	// Rows used so far: 1 (title) + hintRows (hint lines + separator, when
	// present); the bottom border takes one more, so the content gets
	// innerH - hintRows rows and the popup totals exactly h rows.
	rows := innerH - hintRows
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
