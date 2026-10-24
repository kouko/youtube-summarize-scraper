package tui

import (
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"
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

// REQ-6: the bridge notifies a callback after applying each event, so the TUI
// can redraw immediately instead of waiting for the next periodic tick
// (user-reported: the log area did not refresh promptly).
func TestBridgeNotifiesAfterApply(t *testing.T) {
	state := NewAppState()
	b := NewEventBridge(io.Discard, state, 100)
	defer b.Close()

	var n atomic.Int32
	b.SetNotify(func() { n.Add(1) })

	if _, err := b.Write([]byte(`time=1 level=INFO msg="streaming batch complete" success=1 skipped=0 partial=0 failed=0` + "\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	waitFor(t, func() bool { return state.Snapshot().Stats.Success == 1 })
	if n.Load() == 0 {
		t.Error("SetNotify callback not invoked after an applied event")
	}
}

// W1-01 (Acceptance 1, 2): when the queue overflows, the OLDEST queued line
// is evicted and the newest line is retained, so the panel ends with the
// contiguous newest suffix of the burst in write order — at any consumer
// speed — and every written line is applied or dropped exactly once.
func TestBridgeRetainsNewestWhenQueueFull(t *testing.T) {
	state := NewAppState()
	bridge := NewEventBridge(io.Discard, state, 100)
	defer bridge.Close()

	total := 110
	for i := range total {
		line := fmt.Sprintf("e-%03d\n", i)
		if _, err := bridge.Write([]byte(line)); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}

	waitFor(t, func() bool { return bridge.Applied()+bridge.Dropped() == uint64(total) })
	if got := bridge.Applied() + bridge.Dropped(); got != uint64(total) {
		t.Errorf("Applied+Dropped = %d, want %d (every line applied or dropped exactly once)", got, total)
	}

	// Overflow beyond the 100-line queue can drop at most the 10 oldest
	// lines, so the panel holds 100..110 lines: the contiguous suffix of
	// the burst ending at the final written line.
	snap := state.Snapshot()
	n := len(snap.RecentEvents)
	if n < 100 || n > total {
		t.Fatalf("RecentEvents = %d, want 100..%d (newest lines survive overflow)", n, total)
	}
	if first, last := snap.RecentEvents[0], snap.RecentEvents[n-1]; first != fmt.Sprintf("e-%03d", total-n) || last != "e-109" {
		t.Errorf("retained window = %q..%q, want %q..%q (contiguous newest suffix)", first, last, fmt.Sprintf("e-%03d", total-n), "e-109")
	}
	for i, line := range snap.RecentEvents {
		want := fmt.Sprintf("e-%03d", total-n+i)
		if line != want {
			t.Errorf("position %d = %q, want %q (order lost or window shifted)", i, line, want)
		}
	}
}

// W1-02 (Acceptance 1-4, deterministic guard): stall the consumer after it
// applies the first line so the burst is written while the consumer cannot
// drain — removing the timing dependence of TestBridgeRetainsNewestWhenQueueFull.
// Under the pre-W1-01 drop-new code this retained e-000..e-100 (the newest
// lines dropped on the full queue); the fixed drop-oldest bridge must retain
// exactly e-000 then e-010..e-109.
func TestBridgeStalledConsumerRetainsExactNewestWindow(t *testing.T) {
	state := NewAppState()
	bridge := NewEventBridge(io.Discard, state, 100)
	defer bridge.Close()

	// entered signals the consumer has applied e-000 and is now blocked
	// inside notify; gate holds it there until the burst is written.
	entered := make(chan struct{})
	gate := make(chan struct{})
	var once sync.Once
	bridge.SetNotify(func() {
		once.Do(func() { close(entered) })
		<-gate
	})

	if _, err := bridge.Write([]byte("e-000\n")); err != nil {
		t.Fatalf("write e-000: %v", err)
	}
	waitFor(t, func() bool {
		select {
		case <-entered:
			return true
		default:
			return false
		}
	})

	// Consumer is blocked; these writes must return immediately even as the
	// 100-slot queue overflows (that is the invariant under test).
	for i := 1; i <= 109; i++ {
		line := fmt.Sprintf("e-%03d\n", i)
		if _, err := bridge.Write([]byte(line)); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}
	close(gate)

	waitFor(t, func() bool { return bridge.Applied()+bridge.Dropped() == 110 })
	if got := bridge.Dropped(); got != 9 {
		t.Errorf("Dropped = %d, want 9 (e-001..e-009 evicted)", got)
	}
	if got := bridge.Applied(); got != 101 {
		t.Errorf("Applied = %d, want 101", got)
	}

	events := state.Snapshot().RecentEvents
	if len(events) != 101 {
		t.Fatalf("RecentEvents = %d, want 101", len(events))
	}
	for i, line := range events {
		want := "e-000"
		if i > 0 {
			want = fmt.Sprintf("e-%03d", i+9)
		}
		if line != want {
			t.Errorf("position %d = %q, want %q (retained window is not the exact newest suffix)", i, line, want)
		}
	}
}

// W1-01 (REQ-2): with the queue raised to MaxRecentEvents, a burst larger
// than both the queue and the state cap still fills the panel to its cap —
// the queue is no longer the tighter bottleneck. Dropping at the queue is
// allowed under load, so Dropped is not asserted; the queue capacity
// (MaxRecentEvents) guarantees at least MaxRecentEvents lines reach the
// state, enough to fill the panel to its cap.
func TestBridgeMaxRecentEventsCapacity(t *testing.T) {
	state := NewAppState()
	bridge := NewEventBridge(io.Discard, state, MaxRecentEvents)
	defer bridge.Close()

	total := MaxRecentEvents + 101
	for i := range total {
		line := "e-" + strconv.Itoa(i) + "\n"
		if _, err := bridge.Write([]byte(line)); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}

	waitFor(t, func() bool { return bridge.Applied()+bridge.Dropped() == uint64(total) })
	if got := len(state.Snapshot().RecentEvents); got != MaxRecentEvents {
		t.Errorf("RecentEvents = %d, want %d (state cap fed through the bridge)", got, MaxRecentEvents)
	}
}
