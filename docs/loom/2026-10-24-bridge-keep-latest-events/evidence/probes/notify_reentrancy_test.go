package probes

import (
	"fmt"
	"io"
	"sync/atomic"
	"testing"

	"github.com/kouko/youtube-summarize-scraper/tui"
)

// concern: preserved notify semantics — the callback must fire exactly once
// per applied event (spec REQ-4, intent acceptance 3), a reentrant Write back
// into the bridge from inside the callback must neither deadlock the
// mutex+cond consumer nor corrupt the applied+dropped accounting, and
// SetNotify(nil) must disable the callback.

// TestBridgeNotify_eachAppliedEvent_invokedExactlyOnce counts callbacks
// against applied lines.
func TestBridgeNotify_eachAppliedEvent_invokedExactlyOnce(t *testing.T) {
	state := tui.NewAppState()
	bridge := tui.NewEventBridge(io.Discard, state, 100)
	defer bridge.Close()

	var calls atomic.Int64
	bridge.SetNotify(func() { calls.Add(1) })

	for i := 0; i < 50; i++ {
		writeLine(t, bridge, fmt.Sprintf("n-%02d\n", i))
	}
	waitAccounting(t, bridge, 50)

	if got := calls.Load(); got != 50 {
		t.Errorf("notify calls = %d, want 50 (one per applied event)", got)
	}
	if got := bridge.Dropped(); got != 0 {
		t.Errorf("Dropped = %d, want 0", got)
	}
}

// TestBridgeNotify_setNil_disable disables the callback before any traffic.
func TestBridgeNotify_setNil_disable(t *testing.T) {
	state := tui.NewAppState()
	bridge := tui.NewEventBridge(io.Discard, state, 100)
	defer bridge.Close()

	var calls atomic.Int64
	bridge.SetNotify(func() { calls.Add(1) })
	bridge.SetNotify(nil)

	writeLine(t, bridge, "n-0\n")
	waitAccounting(t, bridge, 1)
	if got := calls.Load(); got != 0 {
		t.Errorf("notify calls = %d, want 0 after SetNotify(nil)", got)
	}
}
