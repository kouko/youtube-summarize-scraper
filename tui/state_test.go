package tui

import (
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
	// RecentEvents is a ring bounded at 100; 200 events leave the last 100.
	if got := len(snap.RecentEvents); got != 100 {
		t.Errorf("RecentEvents = %d, want 100 (ring bound)", got)
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

// RecentEvents ring bound: capacity stays at 100.
func TestRecentEventsRingBound(t *testing.T) {
	s := NewAppState()
	for range 250 {
		s.AddRecentEvent("e")
	}
	if got := len(s.Snapshot().RecentEvents); got != 100 {
		t.Errorf("RecentEvents = %d, want 100 (ring bound)", got)
	}
}
