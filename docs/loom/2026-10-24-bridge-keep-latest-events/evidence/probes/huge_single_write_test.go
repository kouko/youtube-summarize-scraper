package probes

import (
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/kouko/youtube-summarize-scraper/tui"
)

// concern: huge single Write under overflow — guards spec REQ-2 (exact newest
// window retained) and REQ-3 (non-blocking call, exact accounting) when ONE
// Write call carries 1099 numbered lines plus a 1 MiB line against a fully
// stalled consumer: the old drop-new transport lost exactly these newest
// lines while reporting the same drop count, so only the retained window
// separates the two behaviours.

// TestBridgeQueue_singleHugeWriteStalled_retainsExactWindow blocks the
// consumer after the first applied line, then delivers a 1 MiB line followed
// by e-001..e-1099 in a single Write at queue capacity 100. The panel must end
// with exactly the 1 MiB line plus e-1000..e-1099, with 999 dropped, 101
// applied, and the Write itself non-blocking.
func TestBridgeQueue_singleHugeWriteStalled_retainsExactWindow(t *testing.T) {
	state := tui.NewAppState()
	bridge := tui.NewEventBridge(io.Discard, state, 100)
	defer bridge.Close()

	g := newConsumerGate()
	bridge.SetNotify(g.notify)

	const total = 1100 // the 1 MiB line plus e-001..e-1099
	huge := strings.Repeat("x", 1<<20)
	writeLine(t, bridge, huge+"\n")
	g.waitEntered(t)

	var payload []byte
	for i := 1; i <= 1099; i++ {
		payload = append(payload, []byte(fmt.Sprintf("e-%03d\n", i))...)
	}

	start := time.Now()
	if _, err := bridge.Write(payload); err != nil {
		t.Fatalf("huge single write: %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("single Write carrying 1099 lines took %v; must not block on a full queue", elapsed)
	}
	close(g.gate)

	waitAccounting(t, bridge, total)
	if got := bridge.Dropped(); got != 999 {
		t.Errorf("Dropped = %d, want 999 (only the oldest queued lines may be evicted)", got)
	}
	if got := bridge.Applied(); got != 101 {
		t.Errorf("Applied = %d, want 101 (the 1 MiB line plus e-1000..e-1099)", got)
	}

	events := state.Snapshot().RecentEvents
	if len(events) != 101 {
		t.Fatalf("RecentEvents = %d lines, want 101", len(events))
	}
	if events[0] != huge {
		t.Errorf("first retained = %d-byte line, want the 1 MiB line (an applied line was lost)", len(events[0]))
	}
	assertSuffix(t, events[1:], 1000, 1099)
}
