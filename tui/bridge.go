package tui

import (
	"bytes"
	"io"
	"strconv"
	"sync"
	"sync/atomic"
)

// EventBridge tees each log line to an optional underlying writer and feeds
// it into a bounded, non-blocking queue consumed by a background goroutine
// that parses each line and applies it to AppState. A slow TUI can therefore
// never block the pipeline: when the queue is full, lines are dropped and
// counted (spec: event-bridge layer, bounded channel, non-blocking send).
type EventBridge struct {
	out  io.Writer
	ch   chan string
	done chan struct{}
	once sync.Once

	buffer pending

	state   *AppState
	applied atomic.Uint64
	dropped atomic.Uint64
}

// NewEventBridge creates an EventBridge with a queue of the given capacity.
// If capacity <= 0 it defaults to 100 (spec capacity).
func NewEventBridge(out io.Writer, state *AppState, capacity int) *EventBridge {
	if capacity <= 0 {
		capacity = 100
	}
	b := &EventBridge{
		out:   out,
		ch:    make(chan string, capacity),
		done:  make(chan struct{}),
		state: state,
	}
	go b.consume()
	return b
}

// consume parses queued lines and applies them to AppState until Close.
func (b *EventBridge) consume() {
	for {
		select {
		case <-b.done:
			return
		case line := <-b.ch:
			applyEvent(b.state, ParseLogLine(line))
			b.applied.Add(1)
		}
	}
}

// Write implements io.Writer. It writes p to the underlying writer and
// forwards complete lines to the queue; an incomplete trailing fragment is
// buffered until its newline arrives. It never blocks and never returns an
// error for queue overflow — overflow is counted and dropped.
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

// Close stops the consumer goroutine. Lines queued but not yet consumed are
// abandoned; later Writes are still accepted and dropped (counted) once the
// queue is full. Close is idempotent.
func (b *EventBridge) Close() {
	b.once.Do(func() { close(b.done) })
}

// Dropped returns the number of lines dropped because the queue was full.
func (b *EventBridge) Dropped() uint64 {
	return b.dropped.Load()
}

// Applied returns the number of lines consumed and applied to AppState.
func (b *EventBridge) Applied() uint64 {
	return b.applied.Load()
}

// String reports queue drop counts for the status panel.
func (b *EventBridge) String() string {
	return "dropped " + strconv.FormatUint(b.dropped.Load(), 10)
}

// enqueue sends a line without blocking; a full queue drops the line.
func (b *EventBridge) enqueue(line string) {
	select {
	case b.ch <- line:
	default:
		b.dropped.Add(1)
	}
}

// pending accumulates a partially written line between Write calls.
type pending struct{ buf []byte }

func (p *pending) Write(b []byte) { p.buf = append(p.buf, b...) }
func (p *pending) bytes() []byte  { return p.buf }
func (p *pending) next(n int)     { p.buf = p.buf[n:] }
