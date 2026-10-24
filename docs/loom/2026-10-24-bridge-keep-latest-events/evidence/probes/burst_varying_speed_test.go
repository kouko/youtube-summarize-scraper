package probes

import (
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/kouko/youtube-summarize-scraper/tui"
)

// concern: burst retention at any consumer speed — guards spec REQ-1/REQ-2
// (the newest capacity-suffix survives overflow) and REQ-3 (non-blocking
// writes, exact accounting) against a window shift at the eviction edge when
// the consumer switches between fully stalled and free-running inside one
// burst, and against an evicted line resurrecting.

// TestBridgeQueue_consumerVaryingSpeed_retainsNewestSuffix writes 350 lines
// into a 100-slot queue while the consumer is first fully stalled (blocked in
// its notify callback), then released mid-burst to drain at full speed. At
// any speed the last 100 written lines (e-250..e-349) must all be applied, in
// order, as the contiguous tail of the panel; every write must return without
// blocking; and Applied+Dropped must equal the 350 written lines.
func TestBridgeQueue_consumerVaryingSpeed_retainsNewestSuffix(t *testing.T) {
	state := tui.NewAppState()
	bridge := tui.NewEventBridge(io.Discard, state, 100)
	defer bridge.Close()

	g := newConsumerGate()
	bridge.SetNotify(g.notify)

	const total = 350
	writeLine(t, bridge, "e-000\n")
	g.waitEntered(t)

	start := time.Now()
	for i := 1; i <= 199; i++ {
		writeLine(t, bridge, fmt.Sprintf("e-%03d\n", i))
	}
	close(g.gate) // consumer released mid-burst; it now drains at full speed
	for i := 200; i < total; i++ {
		writeLine(t, bridge, fmt.Sprintf("e-%03d\n", i))
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("349 writes took %v; writes must never block on a stalled consumer", elapsed)
	}

	waitAccounting(t, bridge, total)

	// The last 100 written lines were never evictable (an eviction always
	// takes a front line written at index <= k-capacity), so they must all be
	// applied in write order regardless of consumer speed.
	snap := state.Snapshot()
	assertSuffix(t, snap.RecentEvents, 250, 349)
	if got := bridge.Applied(); got < 101 {
		t.Errorf("Applied = %d, want at least 101 (e-000 plus the last 100 lines)", got)
	}
}
