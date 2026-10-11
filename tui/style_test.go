package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// Every rendered row (title bar, content, bottom border) must be exactly the
// panel width so all borders line up. User report: the config card and status
// panels' right borders drifted because short plain-text rows were rendered
// unpadded.
func TestRenderPanelRowsAllSameWidth(t *testing.T) {
	const w = 30
	out := RenderPanelWithTitle("T", "short\na somewhat longer line", w, 7, false, "")
	rows := strings.Split(out, "\n")
	if len(rows) != 7 {
		t.Fatalf("got %d rows, want 7", len(rows))
	}
	for i, line := range rows {
		if got := lipgloss.Width(line); got != w {
			t.Errorf("row %d width = %d, want %d: %q", i, got, w, line)
		}
	}
}

// CJK content rows must be padded by display width (CJK = 2 columns), not by
// rune or byte count, or the right border drifts on CJK-only rows.
func TestRenderPanelRowsCJKWidth(t *testing.T) {
	const w = 20
	out := RenderPanelWithTitle("面板", "漢字測試", w, 4, false, "")
	for i, line := range strings.Split(out, "\n") {
		if got := lipgloss.Width(line); got != w {
			t.Errorf("row %d width = %d, want %d: %q", i, got, w, line)
		}
	}
}

// An overlong content row must be truncated (display-width aware) so it can
// never push the right border out of the panel.
func TestRenderPanelRowsOverlongTruncated(t *testing.T) {
	const w = 20
	out := RenderPanelWithTitle("T", strings.Repeat("x", 100), w, 4, false, "")
	for i, line := range strings.Split(out, "\n") {
		if got := lipgloss.Width(line); got != w {
			t.Errorf("row %d width = %d, want %d: %q", i, got, w, line)
		}
	}
}

// A hint longer than the panel's text area wraps to additional hint lines
// instead of being truncated. Every rendered row still spans exactly the
// panel width (right border never drifts), the total row count stays h, and
// the content area shrinks by the wrapped lines.
func TestRenderPanelHintWraps(t *testing.T) {
	const w = 20
	longHint := strings.Repeat("hint ", 12) // ~60 cols, must wrap in a 16-col text area
	out := RenderPanelWithTitle("T", "content", w, 7, false, longHint)
	rows := strings.Split(out, "\n")
	if len(rows) != 7 { // exactly h rows
		t.Fatalf("got %d rows, want 7", len(rows))
	}
	sepIdx := -1
	for i, line := range rows {
		if got := lipgloss.Width(line); got != w {
			t.Errorf("row %d width = %d, want %d: %q", i, got, w, line)
		}
		if strings.Contains(line, "▔") {
			sepIdx = i
		}
	}
	if sepIdx < 3 {
		t.Errorf("separator row at %d, want >= 3 (hint should wrap to 2+ lines)", sepIdx)
	}
	if !strings.Contains(out, "content") {
		t.Error("content row missing after wrapped hint")
	}
}

// A short hint stays a single line: title row, hint row, then separator.
func TestRenderPanelHintShortSingleLine(t *testing.T) {
	const w = 20
	out := RenderPanelWithTitle("T", "content", w, 7, false, "[c] Switch")
	rows := strings.Split(out, "\n")
	if len(rows) != 7 {
		t.Fatalf("got %d rows, want 7", len(rows))
	}
	sepIdx := -1
	for i, line := range rows {
		if strings.Contains(line, "▔") {
			sepIdx = i
		}
	}
	if sepIdx != 2 {
		t.Errorf("separator row at %d, want 2 (single-line hint)", sepIdx)
	}
}

// A CJK hint wraps by display width too; every hint row must still be padded
// to the panel width or the right border drifts on CJK rows.
func TestRenderPanelHintWrapsCJK(t *testing.T) {
	const w = 20
	out := RenderPanelWithTitle("T", "content", w, 7, false, "導航  編輯  切換視圖測試")
	for i, line := range strings.Split(out, "\n") {
		if got := lipgloss.Width(line); got != w {
			t.Errorf("row %d width = %d, want %d: %q", i, got, w, line)
		}
	}
}

// layoutHintSegments places every word within the text area, wraps long
// segment runs to multiple lines, and leaves two-column gaps between
// segments.
func TestLayoutHintSegmentsWrapAndRanges(t *testing.T) {
	segs := []HintSegment{
		{Text: "[↑↓] Navigate"},
		{Text: "[Enter] Edit", Key: "enter"},
		{Text: "[c] Switch View", Key: "c"},
	}
	const area = 38
	lines := layoutHintSegments(segs, area, 5)
	if len(lines) == 0 {
		t.Fatal("no hint lines laid out")
	}
	segStart := map[int]int{}
	for _, line := range lines {
		prevSeg, prevEnd := -1, -1
		for _, pw := range line {
			if pw.startCol < 0 || pw.endCol > area || pw.startCol >= pw.endCol {
				t.Errorf("invalid word range %d..%d for %q", pw.startCol, pw.endCol, pw.text)
			}
			if prevSeg == pw.seg && pw.startCol != prevEnd+1 {
				t.Errorf("words of one segment must be adjacent: %q starts %d, prev end %d", pw.text, pw.startCol, prevEnd)
			}
			if prevSeg != -1 && prevSeg != pw.seg && pw.startCol-prevEnd != 2 {
				t.Errorf("segments must be separated by 2 columns, got gap %d", pw.startCol-prevEnd)
			}
			if s, seen := segStart[pw.seg]; !seen || s > pw.startCol {
				segStart[pw.seg] = pw.startCol
			}
			prevSeg, prevEnd = pw.seg, pw.endCol
		}
	}
}

// Clickable hint segments render in the brighter clickable color; plain
// RenderPanelWithTitle (informational hint) stays in the dim color. Every
// row keeps the panel width either way.
func TestRenderPanelWithSegmentsRowsAndColors(t *testing.T) {
	const w = 44
	segs := []HintSegment{
		{Text: "[↑↓] Navigate"},
		{Text: "[Enter] Edit", Key: "enter"},
	}
	out := RenderPanelWithSegments("Config", "content", w, 9, false, segs)
	for i, line := range strings.Split(out, "\n") {
		if got := lipgloss.Width(line); got != w {
			t.Errorf("row %d width = %d, want %d: %q", i, got, w, line)
		}
	}
	bright := lipgloss.NewStyle().Foreground(lipgloss.Color("#E0E0E0")).Render("Edit")
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("#AAAAAA")).Render("Navigate")
	if !strings.Contains(out, bright) {
		t.Error("clickable segment not rendered in the bright color")
	}
	if !strings.Contains(out, dim) {
		t.Error("informational segment not rendered in the dim color")
	}
}
