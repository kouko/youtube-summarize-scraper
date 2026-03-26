package summarizer

import (
	"log/slog"
	"sync"
	"time"
)

type circuitState int

const (
	stateClosed   circuitState = iota // Normal operation; requests are allowed.
	stateOpen                         // Provider failed; requests are blocked until cooldown expires.
	stateHalfOpen                     // Cooldown expired; one trial request is allowed to probe recovery.
)

func (s circuitState) String() string {
	switch s {
	case stateClosed:
		return "closed"
	case stateOpen:
		return "open"
	case stateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreaker tracks provider health and prevents repeated calls to a
// provider that is known to be unavailable (e.g., quota exhausted).
//
// State transitions:
//
//	Closed  ──(quota error)──→  Open  ──(cooldown)──→  HalfOpen
//	  ↑                                                    │
//	  └────────────(success)───────────────────────────────┘
//	  Open  ←──────(quota error)──────────────────────────┘
type CircuitBreaker struct {
	mu          sync.Mutex
	state       circuitState
	failures    int
	threshold   int
	lastFailure time.Time
	cooldown    time.Duration
	provider    string
	nowFunc     func() time.Time // injectable clock for testing
}

func newCircuitBreaker(provider string, threshold int, cooldown time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:     stateClosed,
		threshold: threshold,
		cooldown:  cooldown,
		provider:  provider,
		nowFunc:   time.Now,
	}
}

// Allow reports whether a request to this provider should be attempted.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case stateClosed:
		return true
	case stateOpen:
		if cb.nowFunc().Sub(cb.lastFailure) >= cb.cooldown {
			cb.state = stateHalfOpen
			slog.Info("provider cooldown expired, probing recovery",
				"provider", cb.provider, "cooldown", cb.cooldown)
			return true
		}
		return false
	case stateHalfOpen:
		// Only one trial allowed in half-open state.
		// The trial is already in progress (Allow was called once to transition).
		return false
	default:
		return true
	}
}

// RecordSuccess signals that a request succeeded. Resets the circuit to Closed.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == stateHalfOpen {
		slog.Info("provider recovered, resuming as primary", "provider", cb.provider)
	}
	cb.state = stateClosed
	cb.failures = 0
}

// RecordFailure signals that a request failed with a quota/rate-limit error.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	if cb.failures >= cb.threshold {
		prev := cb.state
		cb.state = stateOpen
		cb.lastFailure = cb.nowFunc()
		if prev != stateOpen {
			slog.Warn("provider circuit opened",
				"provider", cb.provider,
				"failures", cb.failures,
				"cooldown", cb.cooldown)
		}
	}
}

// State returns the current circuit state (for testing/logging).
func (cb *CircuitBreaker) State() circuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}
