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

// defaultMaxCooldown caps a server-advised Retry-After so a malformed or
// hostile value cannot block a provider for an unbounded time.
const defaultMaxCooldown = time.Hour

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
	// cooldown is the default/exhausted-kind cooldown set at construction.
	cooldown time.Duration
	// rateLimitCooldown is used for KindRateLimit failures with no Retry-After.
	// Defaults to cooldown so single-cooldown behavior is preserved unless a
	// caller (production config) sets a shorter value.
	rateLimitCooldown time.Duration
	// maxCooldown caps an honored Retry-After.
	maxCooldown time.Duration
	// activeCooldown is the cooldown chosen for the current open period.
	activeCooldown time.Duration
	provider       string
	nowFunc        func() time.Time // injectable clock for testing
}

func newCircuitBreaker(provider string, threshold int, cooldown time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:             stateClosed,
		threshold:         threshold,
		cooldown:          cooldown,
		rateLimitCooldown: cooldown, // overridden by production config when set
		maxCooldown:       defaultMaxCooldown,
		activeCooldown:    cooldown,
		provider:          provider,
		nowFunc:           time.Now,
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
		if cb.nowFunc().Sub(cb.lastFailure) >= cb.activeCooldown {
			cb.state = stateHalfOpen
			slog.Info("provider cooldown expired, probing recovery",
				"provider", cb.provider, "cooldown", cb.activeCooldown)
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

// RecordInconclusive signals that a probe request failed for a reason
// unrelated to quota (network blip, 5xx, timeout). It is not a quota failure,
// so it does not count toward the open threshold — but it must not leave a
// half-open circuit stranded. Without this, a non-quota error during the
// half-open probe would block the provider forever (Allow returns false in
// half-open and only RecordSuccess/RecordFailure transition out of it).
// An inconclusive probe re-arms the cooldown so the provider is probed again
// later instead of being permanently skipped.
func (cb *CircuitBreaker) RecordInconclusive() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == stateHalfOpen {
		cb.state = stateOpen
		cb.lastFailure = cb.nowFunc()
		slog.Info("provider probe inconclusive (non-quota error), re-arming cooldown",
			"provider", cb.provider, "cooldown", cb.activeCooldown)
	}
}

// RecordFailure signals that a request failed with a quota/rate-limit error,
// using the default (exhausted) cooldown. Retained for callers/tests that do
// not carry Retry-After/kind detail.
func (cb *CircuitBreaker) RecordFailure() {
	cb.RecordQuotaFailure(0, KindExhausted)
}

// RecordQuotaFailure signals a quota/rate-limit failure and, on opening,
// chooses the cooldown for this open period: the server-advised retryAfter
// (capped at maxCooldown) when known, otherwise a kind-based default
// (rateLimitCooldown for a transient throttle, cooldown for exhaustion).
func (cb *CircuitBreaker) RecordQuotaFailure(retryAfter time.Duration, kind QuotaErrorKind) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	if cb.failures >= cb.threshold {
		prev := cb.state
		cb.state = stateOpen
		cb.lastFailure = cb.nowFunc()
		cb.activeCooldown = cb.chooseCooldown(retryAfter, kind)
		if prev != stateOpen {
			slog.Warn("provider circuit opened",
				"provider", cb.provider,
				"failures", cb.failures,
				"kind", kind,
				"cooldown", cb.activeCooldown)
		}
	}
}

// chooseCooldown resolves the cooldown for an open period. Caller holds cb.mu.
func (cb *CircuitBreaker) chooseCooldown(retryAfter time.Duration, kind QuotaErrorKind) time.Duration {
	if retryAfter > 0 {
		if retryAfter > cb.maxCooldown {
			return cb.maxCooldown
		}
		return retryAfter
	}
	if kind == KindRateLimit {
		return cb.rateLimitCooldown
	}
	return cb.cooldown
}

// State returns the current circuit state (for testing/logging).
func (cb *CircuitBreaker) State() circuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}
