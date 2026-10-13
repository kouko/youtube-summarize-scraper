package probes

import (
	"fmt"
	"testing"

	"github.com/kouko/youtube-summarize-scraper/tui"
)

// TestAdversaryRecentEventsExactlyAtCapAndOneOver tests the boundary at exactly
// MaxRecentEvents and one over.
func TestAdversaryRecentEventsExactlyAtCapAndOneOver(t *testing.T) {
	s := tui.NewAppState()

	// Fill exactly to MaxRecentEvents (1000)
	for i := range tui.MaxRecentEvents {
		s.AddRecentEvent(fmt.Sprintf("event-%04d", i))
	}
	snap := s.Snapshot()
	if got := len(snap.RecentEvents); got != tui.MaxRecentEvents {
		t.Errorf("At exactly cap: RecentEvents = %d, want %d", got, tui.MaxRecentEvents)
	}
	if snap.RecentEvents[0] != "event-0000" {
		t.Errorf("At exactly cap: oldest should be event-0000, got %q", snap.RecentEvents[0])
	}
	if snap.RecentEvents[tui.MaxRecentEvents-1] != fmt.Sprintf("event-%04d", tui.MaxRecentEvents-1) {
		t.Errorf("At exactly cap: newest should be event-0999, got %q", snap.RecentEvents[tui.MaxRecentEvents-1])
	}

	// Add ONE more - should drop the oldest
	s.AddRecentEvent("event-1000")
	snap = s.Snapshot()
	if got := len(snap.RecentEvents); got != tui.MaxRecentEvents {
		t.Errorf("One over cap: RecentEvents = %d, want %d (capped)", got, tui.MaxRecentEvents)
	}
	if snap.RecentEvents[0] != "event-0001" {
		t.Errorf("One over cap: oldest should be event-0001 (event-0000 dropped), got %q", snap.RecentEvents[0])
	}
	if snap.RecentEvents[tui.MaxRecentEvents-1] != "event-1000" {
		t.Errorf("One over cap: newest should be event-1000, got %q", snap.RecentEvents[tui.MaxRecentEvents-1])
	}
}

// TestAdversaryRecentEventsJustUnderCap tests just under the cap.
func TestAdversaryRecentEventsJustUnderCap(t *testing.T) {
	s := tui.NewAppState()

	for i := range tui.MaxRecentEvents - 1 {
		s.AddRecentEvent(fmt.Sprintf("e-%d", i))
	}
	snap := s.Snapshot()
	if got := len(snap.RecentEvents); got != tui.MaxRecentEvents-1 {
		t.Errorf("Just under cap: RecentEvents = %d, want %d", got, tui.MaxRecentEvents-1)
	}
	if snap.RecentEvents[0] != "e-0" {
		t.Errorf("Just under cap: oldest dropped unexpectedly, got %q", snap.RecentEvents[0])
	}
}

// TestAdversaryRecentEventsMassivelyOverCap tests massively over the cap.
func TestAdversaryRecentEventsMassivelyOverCap(t *testing.T) {
	s := tui.NewAppState()

	for i := range 10000 {
		s.AddRecentEvent(fmt.Sprintf("e-%d", i))
	}
	snap := s.Snapshot()
	if got := len(snap.RecentEvents); got != tui.MaxRecentEvents {
		t.Errorf("Massively over cap: RecentEvents = %d, want %d", got, tui.MaxRecentEvents)
	}
	if snap.RecentEvents[0] != "e-9000" {
		t.Errorf("Massively over cap: oldest should be e-9000 (10000-1000), got %q", snap.RecentEvents[0])
	}
	if snap.RecentEvents[tui.MaxRecentEvents-1] != "e-9999" {
		t.Errorf("Massively over cap: newest should be e-9999, got %q", snap.RecentEvents[tui.MaxRecentEvents-1])
	}
}