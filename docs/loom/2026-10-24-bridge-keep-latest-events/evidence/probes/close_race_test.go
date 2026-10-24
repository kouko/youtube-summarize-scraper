package probes

import (
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/kouko/youtube-summarize-scraper/tui"
)

// concern: wrong order / two callers at once — guards the rewritten
// mutex+cond transport against Write racing Close from other goroutines:
// Write must never block or panic while Close runs, Close must stay
// idempotent under concurrent callers and keep waiting for a consumer
// blocked in its notify callback (intent acceptance 3), queued lines are
// abandoned uncounted (documented), and post-Close writes stay accepted.

// TestBridgeWrite_racingClose_neverBlocksOrPanics hammers Write from one
// goroutine while two others call Close concurrently mid-burst.
func TestBridgeWrite_racingClose_neverBlocksOrPanics(t *testing.T) {
	state := tui.NewAppState()
	bridge := tui.NewEventBridge(io.Discard, state, 50)

	const total = 5000
	start := time.Now()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < total; i++ {
			if _, err := bridge.Write([]byte(fmt.Sprintf("w-%04d\n", i))); err != nil {
				t.Errorf("write %d: %v", i, err)
				return
			}
		}
	}()

	var closeWG sync.WaitGroup
	closeWG.Add(2)
	go func() {
		defer closeWG.Done()
		time.Sleep(100 * time.Microsecond)
		bridge.Close()
	}()
	go func() {
		defer closeWG.Done()
		time.Sleep(150 * time.Microsecond)
		bridge.Close()
	}()
	closeWG.Wait()
	wg.Wait()

	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("%d writes racing two Closes took %v; neither may block the other", total, elapsed)
	}

	// Post-Close writes are still accepted: never blocking, never panicking.
	done := make(chan struct{})
	go func() {
		bridge.Write([]byte("after-close\n"))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("post-Close Write blocked")
	}
}

// TestBridgeClose_withStalledConsumer_waitsAndAbandonsQueuedLines pins the
// preserved Close semantics: Close waits for a consumer blocked in notify,
// returns idempotently, freezes Applied+Dropped with the still-queued lines
// abandoned, and leaves the bridge writable.
func TestBridgeClose_withStalledConsumer_waitsAndAbandonsQueuedLines(t *testing.T) {
	state := tui.NewAppState()
	bridge := tui.NewEventBridge(io.Discard, state, 10)
	g := newConsumerGate()
	bridge.SetNotify(g.notify)

	writeLine(t, bridge, "e-000\n")
	g.waitEntered(t)
	for i := 1; i <= 30; i++ {
		writeLine(t, bridge, fmt.Sprintf("e-%03d\n", i))
	}
	// At this point: 1 applied, 20 evicted, e-021..e-030 queued.

	go func() {
		time.Sleep(50 * time.Millisecond)
		close(g.gate) // unblock the consumer so Close's wait can finish
	}()

	closed := make(chan struct{})
	go func() { bridge.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not return while the consumer was blocked in notify")
	}
	bridge.Close() // idempotent second call

	if got := bridge.Applied() + bridge.Dropped(); got != 21 {
		t.Errorf("Applied+Dropped = %d, want 21 (the 10 queued lines at Close are abandoned, not counted)", got)
	}

	done := make(chan struct{})
	go func() {
		bridge.Write([]byte("after-close\n"))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("post-Close Write blocked")
	}
}
