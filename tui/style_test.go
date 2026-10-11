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
	if len(rows) != 6 {
		t.Fatalf("got %d rows, want 6", len(rows))
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
// panel width (right border never drifts), the total row count stays h-1,
// and the content area shrinks by the wrapped lines.
func TestRenderPanelHintWraps(t *testing.T) {
	const w = 20
	longHint := strings.Repeat("hint ", 12) // ~60 cols, must wrap in a 16-col text area
	out := RenderPanelWithTitle("T", "content", w, 7, false, longHint)
	rows := strings.Split(out, "\n")
	if len(rows) != 6 { // h-1 rows
		t.Fatalf("got %d rows, want 6", len(rows))
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
	if len(rows) != 6 {
		t.Fatalf("got %d rows, want 6", len(rows))
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
