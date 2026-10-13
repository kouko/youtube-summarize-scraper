package tui

import (
	"fmt"
	"sync"
	"testing"

	"github.com/kouko/youtube-summarize-scraper/pipeline"
)

// W1-03 A3 positive: concurrent writers and snapshot readers under -race.
func TestAppStateConcurrentAccess(t *testing.T) {
	s := NewAppState()
	var wg sync.WaitGroup

	for i := range 4 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := range 100 {
				switch n % 2 {
				case 0:
					// Increment-only writers.
					s.IncrementSuccess()
					s.IncrementSkipped()
					s.IncrementFailed()
					s.IncrementPartial()
				case 1:
					// Video/queue/event writers + readers.
					s.SetCurrentVideo("vid", "stage")
					s.SetStage("stage2")
					s.AddRecentEvent("line")
					s.SetWatchIter(j)
					s.SetQueueLen(j)
					_ = s.Snapshot()
				}
			}
		}(i)
	}
	wg.Wait()

	snap := s.Snapshot()
	if snap.Stats.Success != 200 || snap.Stats.Skipped != 200 ||
		snap.Stats.Failed != 200 || snap.Stats.Partial != 200 {
		t.Errorf("Stats = %+v, want 200 each (2 increment goroutines)", snap.Stats)
	}
	// 200 events are below the cap: every event is retained.
	if got := len(snap.RecentEvents); got != 200 {
		t.Errorf("RecentEvents = %d, want 200 (no events lost below the cap)", got)
	}
	if snap.CurrentVideo != "vid" || snap.CurrentStage != "stage2" {
		t.Errorf("video = %q stage = %q, want vid/stage2", snap.CurrentVideo, snap.CurrentStage)
	}
}

// UpdateStats replaces the whole Stats struct.
func TestAppStateUpdateStatsReplaces(t *testing.T) {
	s := NewAppState()
	s.IncrementSuccess()
	s.UpdateStats(pipeline.Stats{Success: 5, Failed: 2})
	snap := s.Snapshot()
	if snap.Stats.Success != 5 || snap.Stats.Failed != 2 || snap.Stats.Skipped != 0 {
		t.Errorf("Stats = %+v, want replaced {5 0 2 0}", snap.Stats)
	}
}

// W1-03 negative: Snapshot returns a copy; mutating it leaves state unchanged.
func TestSnapshotIsCopy(t *testing.T) {
	s := NewAppState()
	s.SetCurrentVideo("v", "st")
	s.AddRecentEvent("one")

	snap := s.Snapshot()
	snap.CurrentVideo = "mutated"
	snap.RecentEvents[0] = "mutated"
	snap.Stats.Success = 99

	fresh := s.Snapshot()
	if fresh.CurrentVideo != "v" {
		t.Errorf("CurrentVideo = %q after mutating snapshot, want v", fresh.CurrentVideo)
	}
	if fresh.RecentEvents[0] != "one" {
		t.Errorf("RecentEvents[0] = %q after mutating snapshot, want one", fresh.RecentEvents[0])
	}
	if fresh.Stats.Success != 0 {
		t.Errorf("Stats.Success = %d after mutating snapshot, want 0", fresh.Stats.Success)
	}
}

// A1 (REQ-1): history beyond the old 100-line limit is retained, with no
// drop before the cap.
func TestRecentEventsRetainBeyondOldLimit(t *testing.T) {
	s := NewAppState()
	for i := range 150 {
		s.AddRecentEvent(fmt.Sprintf("e-%d", i))
	}
	snap := s.Snapshot()
	if got := len(snap.RecentEvents); got != 150 {
		t.Errorf("RecentEvents = %d, want 150 (beyond the old 100 limit)", got)
	}
	if snap.RecentEvents[0] != "e-0" {
		t.Errorf("RecentEvents[0] = %q, want e-0 (no drop before the cap)", snap.RecentEvents[0])
	}
}

// A2 (REQ-2): retention caps at 1000 (MaxRecentEvents); the oldest lines are
// dropped beyond the cap.
func TestRecentEventsCapDropsOldest(t *testing.T) {
	s := NewAppState()
	for i := range 1000 {
		s.AddRecentEvent(fmt.Sprintf("e-%d", i))
	}
	snap := s.Snapshot()
	if got := len(snap.RecentEvents); got != 1000 {
		t.Errorf("RecentEvents = %d, want 1000 (at cap, nothing dropped yet)", got)
	}
	if snap.RecentEvents[0] != "e-0" {
		t.Errorf("RecentEvents[0] = %q, want e-0 (no drop at exactly the cap)", snap.RecentEvents[0])
	}

	for i := 1000; i < 1101; i++ {
		s.AddRecentEvent(fmt.Sprintf("e-%d", i))
	}
	snap = s.Snapshot()
	if got := len(snap.RecentEvents); got != 1000 {
		t.Errorf("RecentEvents = %d, want 1000 (capped after 1101 lines)", got)
	} else if snap.RecentEvents[0] != "e-101" {
		t.Errorf("RecentEvents[0] = %q, want e-101 (oldest dropped beyond cap)", snap.RecentEvents[0])
	} else if last := snap.RecentEvents[999]; last != "e-1100" {
		t.Errorf("RecentEvents[999] = %q, want e-1100 (newest retained)", last)
	}
}
