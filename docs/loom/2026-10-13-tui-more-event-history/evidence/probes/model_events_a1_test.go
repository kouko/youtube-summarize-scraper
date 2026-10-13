package probes

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kouko/youtube-summarize-scraper/tui"
)

// ATTACK: A1 — user scrolls up in events panel beyond 100 lines and sees
// older lines. With the old 100-line cap, lines 0-49 would be gone.
// This tests the FULL PIPELINE through the model's View().
// ATTACK: A1 — user scrolls up in events panel beyond 100 lines and sees
// older lines. With the old 100-line cap, lines 0-49 would be gone.
// This tests the FULL PIPELINE through the model's View().
func TestAdversaryEventsPanelScrollsBeyondOld100Limit(t *testing.T) {
	state := tui.NewAppState()
	m := newSizedEventsModelWithState(state)

	// Add 150 events — beyond the old 100 limit
	for i := range 150 {
		state.AddRecentEvent(fmt.Sprintf("event-%03d", i))
	}

	// Initial view should show the newest (bottom-pinned)
	viewContent(t, m)
	if !strings.Contains(m.View().Content, "event-149") {
		t.Error("initial view missing newest event-149")
	}

	// Helper to reset to bottom (newest pinned) for consistent starting point
	resetToBottom := func(m *tui.Model) {
		// Send enough down presses to clamp YOffset to max
		for i := 0; i < 200; i++ {
			m.Update(pressKey("down"))
		}
	}

	// Scroll up enough to reach the top (oldest line visible)
	const scrollsToTop = 200 // more than enough to clamp YOffset to 0
	for i := 0; i < scrollsToTop; i++ {
		m.Update(pressKey("up"))
	}
	// At top, viewport shows lines 0-15 (oldest 16)
	scrolled := m.View().Content
	// A1: event-0 (the oldest) should be visible when scrolled to top
	if !strings.Contains(scrolled, "event-000") {
		t.Errorf("A1 violated: scrolled to top but event-000 not visible")
	}
	// Additionally, verify we can see a line that would have been dropped
	// by the old 100-line cap: e.g., event-049 (the 50th line).
	// From top (YOffset=0), scroll down 49 times to see line 49.
	resetToBottom(m) // start at bottom
	// Go to top: many ups
	for i := 0; i < 200; i++ {
		m.Update(pressKey("up"))
	}
	// Now at top (YOffset=0). Scroll down 49 to see line 49.
	for i := 0; i < 49; i++ {
		m.Update(pressKey("down"))
	}
	if !strings.Contains(m.View().Content, "event-049") {
		t.Errorf("A1 assistance: scrolled down 49 from top but event-049 not visible")
	}
}

// ATTACK: A1 — with 1100 events (over the new 1000 cap), scroll to top
// and verify the oldest retained is ~100 lines back (event-100), not event-0.
// ATTACK: A1 — with 1100 events (over the new 1000 cap), scroll to top
// and verify the oldest retained is ~100 lines back (event-100), not event-0.
func TestAdventEventsPanelScrollsOverNew1000Cap(t *testing.T) {
	state := tui.NewAppState()
	m := newSizedEventsModelWithState(state)

	// Add 1100 events — 100 over the new cap
	for i := range 1100 {
		state.AddRecentEvent(fmt.Sprintf("event-%04d", i))
	}

	// Helper to reset to bottom (newest pinned) for consistent starting point
	resetToBottom := func(m *tui.Model) {
		// Send enough down presses to clamp YOffset to max
		for i := 0; i < 200; i++ {
			m.Update(pressKey("down"))
		}
	}

	// First, verify that lines 0-99 are dropped (not visible when scrolled to top)
	// Scroll up a lot to reach YOffset=0 (top)
	for i := 0; i < 200; i++ {
		m.Update(pressKey("up"))
	}
	scrolledTop := m.View().Content
	// Lines 0-15 should be absent (dropped by cap)
	for i := 0; i < 16; i++ {
		if strings.Contains(scrolledTop, fmt.Sprintf("event-%04d", i)) {
			t.Errorf("A2 violated: line event-%04d should have been dropped beyond cap but is visible at top", i)
		}
	}
	// Line 100 (oldest retained) should not be visible at top (only lines 0-15 visible)
	if strings.Contains(scrolledTop, "event-0100") {
		t.Errorf("A2 violated: event-0100 should not be visible at top (only 0-15 visible)")
	}

	// Now verify we can see the oldest retained line (event-0100) by scrolling down
	// from top to bring it into view.
	resetToBottom(m) // ensure we start at bottom for known baseline
	// Go to top: many ups
	for i := 0; i < 200; i++ {
		m.Update(pressKey("up"))
	}
	// Now at top (YOffset=0). Scroll down until we see event-0100 or we've tried too many times.
	found := false
	for step := 0; step < 2000; step++ {
		if strings.Contains(m.View().Content, "event-0100") {
			found = true
			break
		}
		m.Update(pressKey("down"))
	}
	if !found {
		t.Errorf("A2 violated: could not find event-0100 by scrolling down from top")
	}
	// Also verify that the newest event-1099 is still in buffer (should be visible when scrolled to bottom)
	resetToBottom(m)
	if !strings.Contains(m.View().Content, "event-1099") {
		t.Errorf("A2 violated: newest event-1099 not in buffer when scrolled to bottom")
	}
}

// ATTACK: A1 — verify that the state cap is actually enforced during rapid
// additions (no transient growth beyond cap).
func TestAdversaryEventsStateNeverExceedsCapDuringBurst(t *testing.T) {
	s := tui.NewAppState()
	// Simulate a rapid burst of 2000 events
	for i := range 2000 {
		s.AddRecentEvent(fmt.Sprintf("e-%d", i))
		// Check cap after EVERY addition
		snap := s.Snapshot()
		if got := len(snap.RecentEvents); got > tui.MaxRecentEvents {
			t.Errorf("State exceeded cap during burst: got %d at step %d, want <= %d", got, i, tui.MaxRecentEvents)
			break
		}
	}
}