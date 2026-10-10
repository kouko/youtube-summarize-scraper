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
	out := RenderPanelWithTitle("T", "short\na somewhat longer line", w, 7, false)
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
	out := RenderPanelWithTitle("面板", "漢字測試", w, 4, false)
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
	out := RenderPanelWithTitle("T", strings.Repeat("x", 100), w, 4, false)
	for i, line := range strings.Split(out, "\n") {
		if got := lipgloss.Width(line); got != w {
			t.Errorf("row %d width = %d, want %d: %q", i, got, w, line)
		}
	}
}
