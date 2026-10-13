package probes

import (
	"fmt"
	"testing"

	"github.com/kouko/youtube-summarize-scraper/tui"
)

// TestAdversaryNewAppStatePreallocatesCapacityCorrectly checks that
// NewAppState does not pre-fill the slice with empty strings (a common
// off-by-one mistake when confusing length and capacity).
func TestAdversaryNewAppStatePreallocatesCapacityCorrectly(t *testing.T) {
	s := tui.NewAppState()
	snap := s.Snapshot()

	// Length must be 0, capacity must be MaxRecentEvents
	if len(snap.RecentEvents) != 0 {
		t.Errorf("NewAppState RecentEvents length = %d, want 0 (should be empty, not pre-filled)", len(snap.RecentEvents))
	}
	if cap(snap.RecentEvents) != tui.MaxRecentEvents {
		t.Errorf("NewAppState RecentEvents capacity = %d, want %d", cap(snap.RecentEvents), tui.MaxRecentEvents)
	}
}

// TestAdversaryAddRecentEventFirstEventAtIndexZero checks that the first
// event added lands at index 0, catching any accidental pre-fill.
func TestAdversaryAddRecentEventFirstEventAtIndexZero(t *testing.T) {
	s := tui.NewAppState()
	s.AddRecentEvent("first-event")
	snap := s.Snapshot()

	if len(snap.RecentEvents) != 1 {
		t.Errorf("len = %d, want 1", len(snap.RecentEvents))
	}
	if snap.RecentEvents[0] != "first-event" {
		t.Errorf("first event at index 0 = %q, want 'first-event' (caught pre-filled empty strings)", snap.RecentEvents[0])
	}
}

// TestAdversaryStateCapUsesMaxRecentEventsConstant verifies that the
// state's AddRecentEvent logic uses the exported MaxRecentEvents constant
// as the single source of truth (not a magic number).
func TestAdversaryStateCapUsesMaxRecentEventsConstant(t *testing.T) {
	s := tui.NewAppState()

	// Fill to exactly MaxRecentEvents - 1
	for i := 0; i < tui.MaxRecentEvents-1; i++ {
		s.AddRecentEvent(fmt.Sprintf("e-%d", i))
	}
	snap := s.Snapshot()
	if len(snap.RecentEvents) != tui.MaxRecentEvents-1 {
		t.Errorf("before cap: len = %d, want %d", len(snap.RecentEvents), tui.MaxRecentEvents-1)
	}

	// Add one more to hit the cap exactly
	s.AddRecentEvent("at-cap")
	snap = s.Snapshot()
	if len(snap.RecentEvents) != tui.MaxRecentEvents {
		t.Errorf("at cap: len = %d, want %d", len(snap.RecentEvents), tui.MaxRecentEvents)
	}

	// Add one more — should drop the oldest
	s.AddRecentEvent("over-cap")
	snap = s.Snapshot()
	if len(snap.RecentEvents) != tui.MaxRecentEvents {
		t.Errorf("over cap: len = %d, want %d (capped)", len(snap.RecentEvents), tui.MaxRecentEvents)
	}
	// Oldest dropped should be e-0
	if snap.RecentEvents[0] != "e-1" {
		t.Errorf("over cap: oldest = %q, want e-1 (e-0 dropped)", snap.RecentEvents[0])
	}
}