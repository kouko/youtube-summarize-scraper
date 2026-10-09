package tui

import (
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	// 5s to stay green under -race, where goroutines are much slower.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met before deadline")
}

func TestBridgeForwardsLinesToState(t *testing.T) {
	state := NewAppState()
	bridge := NewEventBridge(io.Discard, state, 100)
	logger := slog.New(slog.NewTextHandler(bridge, nil))

	logger.Info("streaming: processing channel video", "url", "https://x", "title", "T1")

	waitFor(t, func() bool {
		s := state.Snapshot()
		return len(s.RecentEvents) == 1 && s.CurrentVideo == "T1"
	})
	if got := bridge.Dropped(); got != 0 {
		t.Errorf("Dropped = %d, want 0", got)
	}
}

func TestBridgeMultiLineWrite(t *testing.T) {
	state := NewAppState()
	bridge := NewEventBridge(io.Discard, state, 100)

	// Two log lines in one Write call — slog never does this, but the
	// bridge must not lose either.
	bridge.Write([]byte(
		`time=1 level=INFO msg="a" nonce=1` + "\n" +
			`time=2 level=INFO msg="b" nonce=2` + "\n"))

	waitFor(t, func() bool { return len(state.Snapshot().RecentEvents) == 2 })
}

func TestBridgePartialLineBuffered(t *testing.T) {
	state := NewAppState()
	bridge := NewEventBridge(io.Discard, state, 100)

	// A line fragment without a trailing newline must not fire an event
	// until its newline arrives.
	bridge.Write([]byte(`time=1 level=INFO msg="half`))
	if got := len(state.Snapshot().RecentEvents); got != 0 {
		t.Errorf("events after partial write = %d, want 0", got)
	}
	bridge.Write([]byte(` line" nonce=1` + "\n"))
	waitFor(t, func() bool { return len(state.Snapshot().RecentEvents) == 1 })
}

func TestBridgeConcurrentWriters(t *testing.T) {
	state := NewAppState()
	// Big queue so the fast producers never overflow the bounded channel.
	bridge := NewEventBridge(io.Discard, state, 10000)
	logger := slog.New(slog.NewTextHandler(bridge, nil))

	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := range 50 {
				logger.Info("built video index", "goroutine", n, "seq", j)
			}
		}(i)
	}
	wg.Wait()

	waitFor(t, func() bool { return bridge.Applied() == 400 })
	if got := bridge.Dropped(); got != 0 {
		t.Errorf("Dropped = %d, want 0", got)
	}
}

func TestBridgeDropsWhenQueueFull(t *testing.T) {
	state := NewAppState()
	bridge := NewEventBridge(io.Discard, state, 1)
	// Stop the consumer so the queue never drains; then every line beyond
	// the one slot is dropped, and Write must still return without blocking.
	bridge.Close()

	if _, err := bridge.Write([]byte("a\n")); err != nil {
		t.Fatalf("first Write: %v", err)
	}
	for i := 0; i < 5; i++ {
		if _, err := bridge.Write([]byte("x\n")); err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
	}
	if got := bridge.Dropped(); got != 5 {
		t.Errorf("Dropped = %d, want 5", got)
	}
	if got := bridge.Applied(); got != 0 {
		t.Errorf("Applied = %d, want 0 (consumer stopped)", got)
	}
}

func TestBridgeDropsNeverBlocksPipeline(t *testing.T) {
	state := NewAppState()
	bridge := NewEventBridge(io.Discard, state, 1)
	bridge.Close() // consumer gone, queue is one slot

	start := time.Now()
	for i := 0; i < 1000; i++ {
		bridge.Write([]byte("x\n"))
	}
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Errorf("1000 writes took %v; non-blocking send should be far faster", elapsed)
	}
}
