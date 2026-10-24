package tui

import (
	"bytes"
	"io"
	"sync"
	"sync/atomic"
)

// EventBridge tees each log line to an optional underlying writer and feeds
// it into a bounded, non-blocking queue consumed by a background goroutine
// that parses each line and applies it to AppState. A slow TUI can therefore
// never block the pipeline: when the queue is full, the OLDEST queued line is
// evicted and counted, so the most recent activity is what survives (spec:
// event-bridge layer, drop-oldest retention, non-blocking writes).
type EventBridge struct {
	out io.Writer

	// mu guards queue and closed, and serializes the detect-full/evict/append
	// decision so overflow always evicts exactly the queue front. cond wakes
	// the consumer when a line arrives or the bridge closes.
	mu       sync.Mutex
	cond     *sync.Cond
	queue    []string
	capacity int
	closed   bool

	once sync.Once
	wg   sync.WaitGroup

	buffer pending

	state   *AppState
	applied atomic.Uint64
	dropped atomic.Uint64

	// notify, when set, is called after each applied event so the TUI can
	// redraw immediately instead of waiting for the next periodic tick
	// (spec amend2 REQ-6). It must be non-blocking: it runs on the consumer
	// goroutine.
	notify func()
}

// NewEventBridge creates an EventBridge with a queue of the given capacity.
// If capacity <= 0 it defaults to 100 (spec capacity).
func NewEventBridge(out io.Writer, state *AppState, capacity int) *EventBridge {
	if capacity <= 0 {
		capacity = 100
	}
	b := &EventBridge{
		out:      out,
		queue:    make([]string, 0, capacity),
		capacity: capacity,
		state:    state,
	}
	b.cond = sync.NewCond(&b.mu)
	b.wg.Add(1)
	go b.consume()
	return b
}

// consume parses queued lines and applies them to AppState until Close. Lines
// still queued when Close runs are abandoned.
func (b *EventBridge) consume() {
	defer b.wg.Done()
	for {
		b.mu.Lock()
		for len(b.queue) == 0 && !b.closed {
			b.cond.Wait()
		}
		if b.closed {
			b.mu.Unlock()
			return
		}
		line := b.queue[0]
		b.queue = b.queue[1:]
		b.mu.Unlock()

		applyEvent(b.state, ParseLogLine(line))
		b.applied.Add(1)
		if b.notify != nil {
			b.notify()
		}
	}
}

// Write implements io.Writer. It writes p to the underlying writer and
// forwards complete lines to the queue; an incomplete trailing fragment is
// buffered until its newline arrives. It never blocks and never returns an
// error for queue overflow — overflow evicts the oldest queued line and is
// counted.
func (b *EventBridge) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if b.out != nil {
		if _, err := b.out.Write(p); err != nil {
			return 0, err
		}
	}
	b.buffer.Write(p)
	for {
		idx := bytes.IndexByte(b.buffer.bytes(), '\n')
		if idx < 0 {
			break
		}
		line := b.buffer.bytes()[:idx]
		if len(line) > 0 {
			b.enqueue(string(line))
		}
		b.buffer.next(idx + 1)
	}
	return len(p), nil
}

// Close stops the consumer goroutine and waits for it to exit, so state
// writes never race a later Write. Lines queued but not consumed are
// abandoned; later Writes still succeed, queuing (then evicting the oldest)
// without being consumed, so Applied + Dropped no longer tracks them. Close
// is idempotent.
func (b *EventBridge) Close() {
	b.once.Do(func() {
		b.mu.Lock()
		b.closed = true
		b.mu.Unlock()
		b.cond.Broadcast()
	})
	b.wg.Wait()
}

// Dropped returns the number of lines dropped because the queue was full.
func (b *EventBridge) Dropped() uint64 {
	return b.dropped.Load()
}

// SetNotify registers a callback invoked after each applied event (see the
// notify field). Pass nil to disable. Safe to call any time.
func (b *EventBridge) SetNotify(fn func()) {
	b.notify = fn
}

// Applied returns the number of lines consumed and applied to AppState.
func (b *EventBridge) Applied() uint64 {
	return b.applied.Load()
}

// enqueue appends a line without blocking; when the queue is full it evicts
// the oldest queued line and keeps the new one, so the newest lines survive
// overflow. The full check and the evict share the same lock hold, so the
// consumer cannot interleave between them.
func (b *EventBridge) enqueue(line string) {
	b.mu.Lock()
	b.queue = append(b.queue, line)
	if len(b.queue) > b.capacity {
		b.queue = b.queue[1:]
		b.dropped.Add(1)
	}
	b.mu.Unlock()
	b.cond.Signal()
}

// pending accumulates a partially written line between Write calls.
type pending struct{ buf []byte }

func (p *pending) Write(b []byte) { p.buf = append(p.buf, b...) }
func (p *pending) bytes() []byte  { return p.buf }
func (p *pending) next(n int)     { p.buf = p.buf[n:] }
