package probes

import (
	"fmt"
	"io"
	"testing"

	"github.com/kouko/youtube-summarize-scraper/tui"
)

// concern: boundary and hostile input — guards the drop-oldest eviction at the
// smallest capacities (1 and 2), the default capacity of 100 for capacity <= 0
// (spec REQ-4 / NewEventBridge contract), preserved partial-line buffering at
// the burst boundary (REQ-4), and inert handling of empty/newline-only/blank
// writes plus intact non-ASCII and CRLF content — each against off-by-one
// eviction, lost fragments, or spurious events that a drop-new transport or a
// sloppy splitter would produce at the same drop count.

// TestBridgeQueue_capacity1_retainsExactWindow: with one queue slot and a
// stalled consumer, the second overflow line must evict the queued first
// overflow line, not itself.
func TestBridgeQueue_capacity1_retainsExactWindow(t *testing.T) {
	state := tui.NewAppState()
	bridge := tui.NewEventBridge(io.Discard, state, 1)
	defer bridge.Close()

	g := newConsumerGate()
	bridge.SetNotify(g.notify)

	writeLine(t, bridge, "e-000\n")
	g.waitEntered(t)
	writeLine(t, bridge, "e-001\n") // fills the single slot
	writeLine(t, bridge, "e-002\n") // must evict e-001 (drop-oldest)
	close(g.gate)

	waitAccounting(t, bridge, 3)
	if got := bridge.Dropped(); got != 1 {
		t.Errorf("Dropped = %d, want 1 (e-001 evicted)", got)
	}
	events := state.Snapshot().RecentEvents
	if len(events) != 2 || events[0] != "e-000" || events[1] != "e-002" {
		t.Errorf("RecentEvents = %v, want [e-000 e-002] (drop-oldest at capacity 1)", events)
	}
}

// TestBridgeQueue_capacity2_retainsExactWindow: with two queue slots, a
// three-line overflow must retain the two newest overflow lines after the
// first applied line.
func TestBridgeQueue_capacity2_retainsExactWindow(t *testing.T) {
	state := tui.NewAppState()
	bridge := tui.NewEventBridge(io.Discard, state, 2)
	defer bridge.Close()

	g := newConsumerGate()
	bridge.SetNotify(g.notify)

	writeLine(t, bridge, "e-000\n")
	g.waitEntered(t)
	writeLine(t, bridge, "e-001\n")
	writeLine(t, bridge, "e-002\n") // queue now full
	writeLine(t, bridge, "e-003\n") // must evict e-001
	close(g.gate)

	waitAccounting(t, bridge, 4)
	if got := bridge.Dropped(); got != 1 {
		t.Errorf("Dropped = %d, want 1 (e-001 evicted)", got)
	}
	events := state.Snapshot().RecentEvents
	if len(events) != 3 || events[0] != "e-000" || events[1] != "e-002" || events[2] != "e-003" {
		t.Errorf("RecentEvents = %v, want [e-000 e-002 e-003] (drop-oldest at capacity 2)", events)
	}
}

// TestBridgeQueue_defaultCapacity0_is100_retainsExactWindow: capacity 0 must
// default to 100 slots — a stalled-consumer burst of 110 lines drops exactly
// the 10 oldest queued lines and retains e-011..e-110.
func TestBridgeQueue_defaultCapacity0_is100_retainsExactWindow(t *testing.T) {
	state := tui.NewAppState()
	bridge := tui.NewEventBridge(io.Discard, state, 0)
	defer bridge.Close()

	g := newConsumerGate()
	bridge.SetNotify(g.notify)

	writeLine(t, bridge, "e-000\n")
	g.waitEntered(t)
	for i := 1; i <= 110; i++ {
		writeLine(t, bridge, fmt.Sprintf("e-%03d\n", i))
	}
	close(g.gate)

	waitAccounting(t, bridge, 111)
	if got := bridge.Dropped(); got != 10 {
		t.Errorf("Dropped = %d, want 10 (default capacity is 100, so e-001..e-010 evicted)", got)
	}
	if got := bridge.Applied(); got != 101 {
		t.Errorf("Applied = %d, want 101", got)
	}
	events := state.Snapshot().RecentEvents
	if len(events) != 101 {
		t.Fatalf("RecentEvents = %d lines, want 101", len(events))
	}
	if events[0] != "e-000" {
		t.Errorf("first retained = %q, want \"e-000\"", events[0])
	}
	assertSuffix(t, events[1:], 11, 110)
}

// TestBridgeWrite_fragmentSurvivesBurst_completesInOrder: with the consumer
// stalled, a fragment is buffered, then completed, then a burst fills the
// queue to exactly capacity. Every event must arrive in order: e-000, the
// completed fragment line, then the burst — with no drop at the boundary.
func TestBridgeWrite_fragmentSurvivesBurst_completesInOrder(t *testing.T) {
	state := tui.NewAppState()
	bridge := tui.NewEventBridge(io.Discard, state, 100)
	defer bridge.Close()

	g := newConsumerGate()
	bridge.SetNotify(g.notify)

	writeLine(t, bridge, "e-000\n")
	g.waitEntered(t)

	// The fragment alone must not fire an event while the consumer is
	// stalled: only the already-applied e-000 is visible.
	writeLine(t, bridge, "frag-A")
	if got := len(state.Snapshot().RecentEvents); got != 1 {
		t.Errorf("RecentEvents = %d after a partial write, want 1 (fragment must wait for its newline)", got)
	}

	// Completing the fragment yields exactly one line, in FIFO position.
	writeLine(t, bridge, "ry-B\n")

	// Fill the queue to exactly capacity: 1 completed fragment + 99 burst
	// lines = 100 slots. Nothing may be dropped at the boundary.
	var payload []byte
	for i := 0; i < 99; i++ {
		payload = append(payload, []byte("burst\n")...)
	}
	if _, err := bridge.Write(payload); err != nil {
		t.Fatalf("burst write: %v", err)
	}
	close(g.gate)

	waitAccounting(t, bridge, 101) // e-000 + completed fragment + 99 burst lines
	if got := bridge.Dropped(); got != 0 {
		t.Errorf("Dropped = %d, want 0 (queue reached exactly capacity, not overflow)", got)
	}

	events := state.Snapshot().RecentEvents
	if len(events) != 101 {
		t.Fatalf("RecentEvents = %d lines, want 101", len(events))
	}
	if events[0] != "e-000" || events[1] != "frag-Ary-B" {
		t.Errorf("leading events = %q, %q; want \"e-000\", \"frag-Ary-B\" (fragment lost or reordered)", events[0], events[1])
	}
	for i := 2; i < len(events); i++ {
		if events[i] != "burst" {
			t.Errorf("position %d = %q, want \"burst\" (order lost or line corrupted)", i, events[i])
		}
	}
}

// TestBridgeWrite_hostileInputs_inertAndCounted: empty writes, newline-only
// writes and blank lines never enqueue a line, so they never perturb the
// retained window or the accounting.
func TestBridgeWrite_hostileInputs_inertAndCounted(t *testing.T) {
	state := tui.NewAppState()
	bridge := tui.NewEventBridge(io.Discard, state, 100)
	defer bridge.Close()

	if n, err := bridge.Write(nil); n != 0 || err != nil {
		t.Errorf("Write(nil) = (%d, %v), want (0, nil)", n, err)
	}
	if n, err := bridge.Write([]byte{}); n != 0 || err != nil {
		t.Errorf("Write(empty) = (%d, %v), want (0, nil)", n, err)
	}
	writeLine(t, bridge, "\n\n\n") // blank lines: no events
	writeLine(t, bridge, "   \n")  // whitespace-only line IS a line

	waitAccounting(t, bridge, 1)
	if got := bridge.Dropped(); got != 0 {
		t.Errorf("Dropped = %d, want 0 (blank lines are not events)", got)
	}
	events := state.Snapshot().RecentEvents
	if len(events) != 1 || events[0] != "   " {
		t.Errorf("RecentEvents = %q, want [\"   \"]", events)
	}
}

// TestBridgeWrite_crlfAndNonAscii_linesIntact: CRLF line endings and
// multi-byte content survive the split and the queue intact.
func TestBridgeWrite_crlfAndNonAscii_linesIntact(t *testing.T) {
	state := tui.NewAppState()
	bridge := tui.NewEventBridge(io.Discard, state, 100)
	defer bridge.Close()

	_, _ = bridge.Write([]byte("動画その1\r\n動画その2\r\n"))
	waitAccounting(t, bridge, 2)

	events := state.Snapshot().RecentEvents
	if len(events) != 2 {
		t.Fatalf("RecentEvents = %d lines, want 2", len(events))
	}
	if events[0] != "動画その1\r" || events[1] != "動画その2\r" {
		t.Errorf("CRLF lines = %q, %q; \\r must survive as content, lines must not be lost", events[0], events[1])
	}
}
