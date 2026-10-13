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

	// Scroll up enough to reach the oldest events
	// With 150 events and viewport height ~24 rows, need ~150 up-presses
	for i := 0; i < 160; i++ {
		m.Update(pressKey("up"))
	}

	scrolled := m.View().Content
	// A1: event-0 (the oldest) should be visible when scrolled to top
	if !strings.Contains(scrolled, "event-000") {
		t.Errorf("A1 violated: scrolled to top but event-000 not visible; content=%q", scrolled[:200])
	}
	// Also verify event-49 (the 50th line, which was beyond old 100 limit) is visible
	if !strings.Contains(scrolled, "event-049") {
		t.Errorf("A1 violated: scrolled to top but event-049 (beyond old 100) not visible; content=%q", scrolled[:200])
	}
}

// ATTACK: A1 — with 1100 events (over the new 1000 cap), scroll to top
// and verify the oldest retained is ~100 lines back (event-100), not event-0.
func TestAdversaryEventsPanelScrollsOverNew1000Cap(t *testing.T) {
	state := tui.NewAppState()
	m := newSizedEventsModelWithState(state)

	// Add 1100 events — 100 over the new cap
	for i := range 1100 {
		state.AddRecentEvent(fmt.Sprintf("event-%04d", i))
	}

	// Scroll all the way to the top
	for i := 0; i < 1200; i++ {
		m.Update(pressKey("up"))
	}

	scrolled := m.View().Content
	// A2: oldest visible should be event-0100 (1100 - 1000 = 100 dropped)
	if !strings.Contains(scrolled, "event-0100") {
		t.Errorf("A2 violated: scrolled to top but event-0100 (oldest retained) not visible; content=%q", scrolled[:200])
	}
	// event-0000 through event-0099 should be GONE
	if strings.Contains(scrolled, "event-0000") {
		t.Errorf("A2 violated: event-0000 should have been dropped beyond cap, but is visible; content=%q", scrolled[:200])
	}
	if strings.Contains(scrolled, "event-0050") {
		t.Errorf("A2 violated: event-0050 should have been dropped beyond cap, but is visible; content=%q", scrolled[:200])
	}
	if strings.Contains(scrolled, "event-0099") {
		t.Errorf("A2 violated: event-0099 should have been dropped beyond cap, but is visible; content=%q", scrolled[:200])
	}
	// Newest event-1099 should still be in the buffer
	if !strings.Contains(scrolled, "event-1099") {
		t.Errorf("A2 violated: newest event-1099 not in buffer; content=%q", scrolled[:200])
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