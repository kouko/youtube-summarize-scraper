package tui

import (
	"strings"
	"testing"
)

// concern: events panel incorrectly highlights INFO lines containing the word "ERROR"
// REQ-4: Show recent log events → Acceptance #4
//
//	WHEN the pipeline emits log lines, the TUI shall display the last N log lines (default 10) in a scrolling area, with ERROR‑level lines highlighted in red.
//
// This test verifies that an INFO-level line containing the substring "ERROR" in its message
// is NOT highlighted as an error, because only ERROR‑level lines should be highlighted.
//
// The defect: the current implementation highlights any line containing the substring "ERROR"
// (case-sensitive), regardless of log level. For example, an INFO line like
//
//	time=1 level=INFO msg="ERROR: disk full"
//
// would be incorrectly highlighted.
//
// To reproduce the defect, run this test against the current change; it should fail.
// After fixing the highlight logic to check only for level=ERROR, the test will pass.
func TestEventsInfoLineWithERRORwordNotHighlighted(t *testing.T) {
	// Arrange: create a model and state snapshot with an INFO line that contains the word "ERROR" in the message.
	state := NewAppState()
	state.AddRecentEvent(`time=1 level=INFO msg="ERROR: disk full"`)
	snap := state.Snapshot()

	m := NewModel(state)
	// Set dummy size for rendering
	m.width = 80
	m.height = 24

	// Act: render the events panel
	out := m.renderEvents(snap, m.width, m.height)

	// Assert: the line should appear without error highlight (i.e., no "[ERROR] " prefix and no ErrorHighlight styling)
	if strings.Contains(out, `[ERROR] time=1 level=INFO msg="ERROR: disk full"`) {
		t.Errorf("events panel incorrectly highlighted INFO line containing the word 'ERROR' in the message\\noutput: %q", out)
	}
	// Also ensure the line itself is present
	if !strings.Contains(out, `time=1 level=INFO msg="ERROR: disk full"`) {
		t.Errorf("events panel did not render the expected line\\noutput: %q", out)
	}
}
