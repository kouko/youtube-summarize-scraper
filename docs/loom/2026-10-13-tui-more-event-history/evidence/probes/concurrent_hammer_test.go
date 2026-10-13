package probes

import (
	"fmt"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kouko/youtube-summarize-scraper/tui"
)

// TestAdversaryConcurrentHammer tests concurrent access to AppState
// from many goroutines (AddRecentEvent, Snapshot, and model render)
// to check for data races or panics under -race.
func TestAdversaryConcurrentHammer(t *testing.T) {
	// Single shared state
	s := tui.NewAppState()
	var wg sync.WaitGroup

	// Writers: hammer AddRecentEvent
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 500; j++ {
				s.AddRecentEvent(fmt.Sprintf("w%d-event-%d", id, j))
			}
		}(i)
	}

	// Snapshot readers: frequent snapshots while writers run
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				_ = s.Snapshot()
			}
		}(i)
	}

	// Model renderers: render the view (which does Snapshot internally)
	// No mutation, just exercises the render path concurrently.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			m := tui.NewModel(s)
			m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
			for j := 0; j < 200; j++ {
				// Force a render (does Snapshot internally)
				_ = m.View()
			}
		}(i)
	}

	wg.Wait()

	// Final sanity: state should be intact
	snap := s.Snapshot()
	if snap.RecentEvents == nil {
		t.Error("RecentEvents nil after concurrent hammer")
	}
	// We don't check exact counts due to non-determinism of goroutine ordering,
	// but we can check that the slice is sane: len <= cap.
	if got := len(snap.RecentEvents); got > tui.MaxRecentEvents*2 { // allow some slop due to bursts
		t.Errorf("RecentEvents length implausibly high: %d (cap=%d)", got, tui.MaxRecentEvents*2)
	}
	if cap := cap(snap.RecentEvents); cap < tui.MaxRecentEvents {
		t.Errorf("RecentEvents capacity too small: %d < %d", cap, tui.MaxRecentEvents)
	}
}