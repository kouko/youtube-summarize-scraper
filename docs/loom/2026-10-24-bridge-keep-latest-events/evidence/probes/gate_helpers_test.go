package probes

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/kouko/youtube-summarize-scraper/tui"
)

// ---- shared helpers for the 2026-10-24 bridge keep-latest-event probes ----

// consumerGate stalls the bridge's consumer inside its notify callback after
// the first applied event until gate is closed, giving a probe a
// deterministic stalled-consumer phase without sleeps.
type consumerGate struct {
	entered chan struct{}
	gate    chan struct{}
	once    sync.Once
}

func newConsumerGate() *consumerGate {
	return &consumerGate{entered: make(chan struct{}), gate: make(chan struct{})}
}

// notify implements the bridge's notify contract; it closes entered once and
// then blocks until the probe closes gate.
func (g *consumerGate) notify() {
	g.once.Do(func() { close(g.entered) })
	<-g.gate
}

// waitEntered fails the test if the consumer does not reach its first apply
// within 5 seconds.
func (g *consumerGate) waitEntered(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-g.entered:
			return
		default:
			time.Sleep(2 * time.Millisecond)
		}
	}
	t.Fatal("consumer never applied the first event")
}

// writeLine writes one line, failing the test on error.
func writeLine(t *testing.T, b *tui.EventBridge, line string) {
	t.Helper()
	if _, err := b.Write([]byte(line)); err != nil {
		t.Fatalf("write %q: %v", line, err)
	}
}

// waitAccounting waits until Applied+Dropped reaches total, failing after 5s.
func waitAccounting(t *testing.T, b *tui.EventBridge, total uint64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if b.Applied()+b.Dropped() == total {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("applied+dropped = %d after 5s, want %d (consumer stalled or lines lost)",
		b.Applied()+b.Dropped(), total)
}

// assertSuffix checks that events ends with the contiguous ordered block
// e-<first>..e-<last> (inclusive). A window shift, lost order, or a
// resurrected evicted line all break one of these checks.
func assertSuffix(t *testing.T, events []string, first, last int) {
	t.Helper()
	n := last - first + 1
	if len(events) < n {
		t.Fatalf("RecentEvents = %d lines, want at least %d", len(events), n)
	}
	if got := events[len(events)-1]; got != fmt.Sprintf("e-%03d", last) {
		t.Fatalf("newest retained = %q, want %q", got, fmt.Sprintf("e-%03d", last))
	}
	for i := 0; i < n; i++ {
		want := fmt.Sprintf("e-%03d", first+i)
		if got := events[len(events)-n+i]; got != want {
			t.Errorf("position %d from end = %q, want %q (window shifted, order lost, or an evicted line resurrected)", i, got, want)
		}
	}
}
