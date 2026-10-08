package tui

import (
	"path/filepath"
	"testing"
	"time"
)

// W3-01 A4 positive: a batch-complete slog line updates Stats counts.
func TestRunnerBatchCompleteUpdatesStats(t *testing.T) {
	state := NewAppState()
	bridge := NewEventBridge(nil, state, 100)
	defer bridge.Close()

	line := `time=1 level=INFO msg="streaming batch complete" success=3 skipped=2 partial=1 failed=4`
	if _, err := bridge.Write([]byte(line + "\n")); err != nil {
		t.Fatalf("write: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s := state.Snapshot()
		if s.Stats.Success == 3 && s.Stats.Failed == 4 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	s := state.Snapshot()
	st := s.Stats
	if st.Success != 3 || st.Skipped != 2 || st.Partial != 1 || st.Failed != 4 {
		t.Errorf("Stats = %+v, want {3 2 1 4}", st)
	}
}

// W3-01 A4 positive: a video-start slog line updates CurrentVideo.
func TestRunnerVideoStartUpdatesCurrentVideo(t *testing.T) {
	state := NewAppState()
	bridge := NewEventBridge(nil, state, 100)
	defer bridge.Close()

	line := `time=1 level=INFO msg="streaming: processing channel video" url="https://x" title="My Video"`
	if _, err := bridge.Write([]byte(line + "\n")); err != nil {
		t.Fatalf("write: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if state.Snapshot().CurrentVideo == "My Video" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := state.Snapshot().CurrentVideo; got != "My Video" {
		t.Errorf("CurrentVideo = %q, want My Video", got)
	}
}

// W3-01 A4 positive: an ERROR slog line lands in RecentEvents.
func TestRunnerErrorLineInRecentEvents(t *testing.T) {
	state := NewAppState()
	bridge := NewEventBridge(nil, state, 100)
	defer bridge.Close()

	line := `time=1 level=ERROR msg="streaming: channel video processing failed" video_id=x error=boom`
	if _, err := bridge.Write([]byte(line + "\n")); err != nil {
		t.Fatalf("write: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		evs := state.Snapshot().RecentEvents
		if len(evs) > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	evs := state.Snapshot().RecentEvents
	if len(evs) == 0 {
		t.Fatal("RecentEvents empty, want the error line")
	}
	if evs[len(evs)-1] != line {
		t.Errorf("last event = %q, want %q", evs[len(evs)-1], line)
	}
}

// W3-01 boundary: runPipelineWithConfig reports a config-load failure as an
// error line in RecentEvents, not a panic.
func TestRunnerBadConfigReportsError(t *testing.T) {
	state := NewAppState()
	bad := filepath.Join(t.TempDir(), "nope.yaml")
	err := runPipelineWithConfig(bad, state)
	if err == nil {
		t.Fatal("runPipelineWithConfig with missing config returned nil, want error")
	}
	evs := state.Snapshot().RecentEvents
	if len(evs) == 0 {
		t.Fatal("RecentEvents empty, want error line")
	}
}
