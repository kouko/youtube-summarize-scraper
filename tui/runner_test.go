package tui

import (
	"context"
	"os"
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
	err := runPipelineWithConfig(bad, state, context.Background())
	if err == nil {
		t.Fatal("runPipelineWithConfig with missing config returned nil, want error")
	}
	evs := state.Snapshot().RecentEvents
	if len(evs) == 0 {
		t.Fatal("RecentEvents empty, want error line")
	}
}

// I3: fetch events add to queue length; completions decrement; batch
// complete resets to zero.
func TestRunnerQueueAccounting(t *testing.T) {
	state := NewAppState()
	bridge := NewEventBridge(nil, state, 100)
	defer bridge.Close()

	lines := []string{
		`time=1 level=INFO msg="total filtered videos across tabs" count=3`,
		`time=1 level=INFO msg="fetched playlist videos" url="https://x" name="WL" total=2`,
	}
	for _, l := range lines {
		if _, err := bridge.Write([]byte(l + "\n")); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	// 3 (channel) + 2 (playlist) fetched = 5 queued.
	waitFor(t, func() bool { return state.Snapshot().QueueLen == 5 })

	done := `time=1 level=INFO msg="video processing complete" video_id=a`
	if _, err := bridge.Write([]byte(done + "\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	waitFor(t, func() bool { return state.Snapshot().QueueLen == 4 })

	batch := `time=1 level=INFO msg="streaming batch complete" success=1 skipped=0 partial=0 failed=0`
	if _, err := bridge.Write([]byte(batch + "\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	waitFor(t, func() bool { return state.Snapshot().QueueLen == 0 })
}

// I3: watch iteration line updates WatchIter via the bridge.
func TestRunnerWatchIter(t *testing.T) {
	state := NewAppState()
	bridge := NewEventBridge(nil, state, 100)
	defer bridge.Close()

	line := `time=1 level=INFO msg="watch: iteration 4 starting"`
	if _, err := bridge.Write([]byte(line + "\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	waitFor(t, func() bool { return state.Snapshot().WatchIter == 4 })
}

// I3: the runner logs a watch-iteration line per batch so WatchIter is
// driven, and a cancelled context stops the loop before the next iteration.
func TestRunnerWatchLoopCancels(t *testing.T) {
	state := NewAppState()
	cfg := writeMinimalConfig(t, "batch:\n  watch: true\n  watch_interval: 3600\n")
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- runPipelineWithConfig(cfg, state, ctx) }()

	// Let the first iteration log its watch line, then cancel.
	waitFor(t, func() bool { return state.Snapshot().WatchIter >= 1 })
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runPipelineWithConfig returned %v, want nil after cancel", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("runner did not stop after context cancel")
	}
}

func writeMinimalConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
