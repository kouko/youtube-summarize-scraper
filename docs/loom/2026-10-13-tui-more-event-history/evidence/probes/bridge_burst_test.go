package probes

import (
	"io"
	"strconv"
	"testing"
	"time"

	"github.com/kouko/youtube-summarize-scraper/tui"
)

// ATTACK: queue sizing — with the queue at MaxRecentEvents, a burst larger
// than both queue and state cap must still deliver at least MaxRecentEvents
// lines to the state (the queue must never be the tighter bottleneck below
// the cap). More aggressive than the landed test: it also asserts the order
// of the delivered lines is preserved.
func TestAdversaryBridgeBurstDeliversInOrderUpToCap(t *testing.T) {
	state := tui.NewAppState()
	bridge := tui.NewEventBridge(io.Discard, state, tui.MaxRecentEvents)
	defer bridge.Close()

	total := tui.MaxRecentEvents + 500
	for i := range total {
		line := "e-" + strconv.Itoa(i) + "\n"
		if _, err := bridge.Write([]byte(line)); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if bridge.Applied()+bridge.Dropped() == uint64(total) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := bridge.Applied() + bridge.Dropped(); got != uint64(total) {
		t.Fatalf("applied+dropped = %d, want %d", got, total)
	}

	snap := state.Snapshot()
	if len(snap.RecentEvents) != tui.MaxRecentEvents {
		t.Fatalf("RecentEvents = %d, want %d (cap fed through bridge)", len(snap.RecentEvents), tui.MaxRecentEvents)
	}

	// Order preservation: because the channel is FIFO and the consumer is
	// single, applied lines arrive in order. The first applied line must be
	// e-0 and the state must hold a contiguous suffix of the written lines.
	first, _ := strconv.Atoi(snap.RecentEvents[0][2:])
	last, _ := strconv.Atoi(snap.RecentEvents[tui.MaxRecentEvents-1][2:])
	if last != total-1 {
		t.Errorf("newest retained = e-%d, want e-%d (the final written line)", last, total-1)
	}
	if first != total-tui.MaxRecentEvents {
		t.Errorf("oldest retained = e-%d, want e-%d (a contiguous suffix)", first, total-tui.MaxRecentEvents)
	}
	for i, line := range snap.RecentEvents {
		want := "e-" + strconv.Itoa(first+i)
		if line != want {
			t.Errorf("position %d = %q, want %q (order lost through the bridge)", i, line, want)
		}
	}
}

// ATTACK: single Write with a huge payload (1100 lines in one call) — the
// pending buffer must split all of them, dropping none before the cap.
func TestAdversaryBridgeHugeSingleWrite(t *testing.T) {
	state := tui.NewAppState()
	bridge := tui.NewEventBridge(io.Discard, state, tui.MaxRecentEvents)
	defer bridge.Close()

	var payload []byte
	total := tui.MaxRecentEvents + 100
	for i := range total {
		payload = append(payload, []byte("e-"+strconv.Itoa(i)+"\n")...)
	}

	if _, err := bridge.Write(payload); err != nil {
		t.Fatalf("huge write: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if bridge.Applied()+bridge.Dropped() == uint64(total) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := bridge.Applied() + bridge.Dropped(); got != uint64(total) {
		t.Fatalf("applied+dropped = %d, want %d (pending buffer lost lines)", got, total)
	}
	if got := len(state.Snapshot().RecentEvents); got != tui.MaxRecentEvents {
		t.Errorf("RecentEvents = %d, want %d", got, tui.MaxRecentEvents)
	}
}

// ATTACK: queue capacity must equal the state cap when constructed through
// the cmd path — cmd/tui.go passes MaxRecentEvents. This guards against a
// regression where the call site forgets to match the constant (single
// source of truth, spec REQ-2).
func TestAdversaryBridgeQueueMatchesStateCap(t *testing.T) {
	state := tui.NewAppState()
	bridge := tui.NewEventBridge(io.Discard, state, tui.MaxRecentEvents)

	// Fill the queue with the consumer stopped; exactly MaxRecentEvents
	// lines must fit without a single drop.
	bridge.Close() // consumer gone; queue is the only sink

	dropped := 0
	for i := range tui.MaxRecentEvents {
		if _, err := bridge.Write([]byte("x\n")); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
		if bridge.Dropped() != uint64(dropped) {
			t.Fatalf("unexpected drop at line %d: Dropped=%d want %d", i, bridge.Dropped(), dropped)
		}
	}
	// The next line must be dropped (queue full).
	bridge.Write([]byte("y\n"))
	if bridge.Dropped() != 1 {
		t.Errorf("queue not full after %d lines: Dropped=%d, want 1", tui.MaxRecentEvents, bridge.Dropped())
	}
}