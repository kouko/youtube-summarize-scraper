package summarizer

import (
	"testing"
	"time"
)

func TestCircuitBreaker_ClosedAllowsRequests(t *testing.T) {
	cb := newCircuitBreaker("test", 1, 5*time.Minute)
	if !cb.Allow() {
		t.Error("closed circuit should allow requests")
	}
}

func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	cb := newCircuitBreaker("test", 2, 5*time.Minute)

	cb.RecordFailure()
	if cb.State() != stateClosed {
		t.Error("should stay closed after 1 failure (threshold=2)")
	}
	if !cb.Allow() {
		t.Error("should allow requests while closed")
	}

	cb.RecordFailure()
	if cb.State() != stateOpen {
		t.Error("should open after 2 failures (threshold=2)")
	}
	if cb.Allow() {
		t.Error("should block requests while open")
	}
}

func TestCircuitBreaker_TransitionsToHalfOpenAfterCooldown(t *testing.T) {
	now := time.Now()
	cb := newCircuitBreaker("test", 1, 5*time.Minute)
	cb.nowFunc = func() time.Time { return now }

	cb.RecordFailure()
	if cb.State() != stateOpen {
		t.Fatal("should be open")
	}

	// Still within cooldown.
	cb.nowFunc = func() time.Time { return now.Add(4 * time.Minute) }
	if cb.Allow() {
		t.Error("should not allow during cooldown")
	}

	// Cooldown expired.
	cb.nowFunc = func() time.Time { return now.Add(5 * time.Minute) }
	if !cb.Allow() {
		t.Error("should allow after cooldown (half-open)")
	}
	if cb.State() != stateHalfOpen {
		t.Error("should be in half-open state")
	}

	// Second call in half-open should be blocked.
	if cb.Allow() {
		t.Error("should block second request in half-open")
	}
}

func TestCircuitBreaker_HalfOpenSuccess_ClosesCircuit(t *testing.T) {
	now := time.Now()
	cb := newCircuitBreaker("test", 1, 1*time.Minute)
	cb.nowFunc = func() time.Time { return now }

	cb.RecordFailure()

	// Advance past cooldown.
	cb.nowFunc = func() time.Time { return now.Add(2 * time.Minute) }
	cb.Allow() // transitions to half-open

	cb.RecordSuccess()
	if cb.State() != stateClosed {
		t.Error("should close after success in half-open")
	}
	if !cb.Allow() {
		t.Error("should allow requests after closing")
	}
}

func TestCircuitBreaker_HalfOpenFailure_ReOpens(t *testing.T) {
	now := time.Now()
	cb := newCircuitBreaker("test", 1, 1*time.Minute)
	cb.nowFunc = func() time.Time { return now }

	cb.RecordFailure()

	// Advance past cooldown.
	now = now.Add(2 * time.Minute)
	cb.nowFunc = func() time.Time { return now }
	cb.Allow() // transitions to half-open

	cb.RecordFailure()
	if cb.State() != stateOpen {
		t.Error("should re-open after failure in half-open")
	}
	if cb.Allow() {
		t.Error("should block requests after re-opening")
	}
}

func TestCircuitBreaker_HalfOpenInconclusive_ReArmsCooldown(t *testing.T) {
	now := time.Now()
	cb := newCircuitBreaker("test", 1, 1*time.Minute)
	cb.nowFunc = func() time.Time { return now }

	cb.RecordFailure() // open

	// Advance past cooldown → half-open probe.
	now = now.Add(2 * time.Minute)
	if !cb.Allow() {
		t.Fatal("should allow probe in half-open")
	}

	// Probe failed with a non-quota error → inconclusive.
	cb.RecordInconclusive()
	if cb.State() != stateOpen {
		t.Fatalf("inconclusive probe should re-open, got %v", cb.State())
	}

	// Must not be permanently stranded: after another cooldown, probe again.
	now = now.Add(2 * time.Minute)
	if !cb.Allow() {
		t.Error("should allow another probe after re-armed cooldown")
	}
}

func TestCircuitBreaker_InconclusiveWhenClosed_IsNoOp(t *testing.T) {
	cb := newCircuitBreaker("test", 1, time.Minute)

	cb.RecordInconclusive()
	if cb.State() != stateClosed {
		t.Errorf("inconclusive on a closed circuit should be a no-op, got %v", cb.State())
	}
	if !cb.Allow() {
		t.Error("closed circuit should still allow requests")
	}
}

func TestCircuitBreaker_RetryAfterHonored(t *testing.T) {
	now := time.Now()
	cb := newCircuitBreaker("test", 1, 5*time.Minute) // default cooldown 5m
	cb.nowFunc = func() time.Time { return now }

	// Server advised a 45s wait — the breaker must honor it, not the 5m default.
	cb.RecordQuotaFailure(45*time.Second, KindRateLimit)
	if cb.State() != stateOpen {
		t.Fatal("should open on quota failure")
	}

	now = now.Add(44 * time.Second)
	if cb.Allow() {
		t.Error("should still block 1s before the advised Retry-After")
	}
	now = now.Add(2 * time.Second) // 46s total
	if !cb.Allow() {
		t.Error("should allow probe once the advised Retry-After elapses")
	}
}

func TestCircuitBreaker_RateLimitVsExhaustedCooldown(t *testing.T) {
	now := time.Now()
	cb := newCircuitBreaker("test", 1, 300*time.Second) // exhausted/default cooldown
	cb.rateLimitCooldown = 60 * time.Second
	cb.nowFunc = func() time.Time { return now }

	// Rate-limit kind, no Retry-After → 60s cooldown.
	cb.RecordQuotaFailure(0, KindRateLimit)
	now = now.Add(61 * time.Second)
	if !cb.Allow() {
		t.Error("rate-limit should recover after rateLimitCooldown (60s)")
	}
	cb.RecordSuccess() // close again

	// Exhausted kind, no Retry-After → 300s cooldown.
	cb.RecordQuotaFailure(0, KindExhausted)
	now = now.Add(61 * time.Second)
	if cb.Allow() {
		t.Error("exhausted should NOT recover after only 61s")
	}
	now = now.Add(300 * time.Second)
	if !cb.Allow() {
		t.Error("exhausted should recover after the full cooldown (300s)")
	}
}

func TestCircuitBreaker_RetryAfterCappedAtMax(t *testing.T) {
	now := time.Now()
	cb := newCircuitBreaker("test", 1, 5*time.Minute)
	cb.maxCooldown = time.Hour
	cb.nowFunc = func() time.Time { return now }

	// An absurd 5h advice must be capped at maxCooldown (1h).
	cb.RecordQuotaFailure(5*time.Hour, KindExhausted)
	now = now.Add(61 * time.Minute)
	if !cb.Allow() {
		t.Error("Retry-After should be capped at maxCooldown, allowing a probe after 1h")
	}
}

func TestCircuitBreaker_OpensAfterConsecutiveEmpties(t *testing.T) {
	now := time.Now()
	cb := newCircuitBreaker("test", 1, 5*time.Minute)
	cb.emptyThreshold = 3
	cb.nowFunc = func() time.Time { return now }

	cb.RecordEmptyResponse()
	cb.RecordEmptyResponse()
	if cb.State() != stateClosed {
		t.Fatalf("should stay closed below empty threshold, got %v", cb.State())
	}
	cb.RecordEmptyResponse() // 3rd consecutive → open
	if cb.State() != stateOpen {
		t.Fatalf("should open after 3 consecutive empties, got %v", cb.State())
	}

	// Opens with the exhausted (default) cooldown.
	now = now.Add(4 * time.Minute)
	if cb.Allow() {
		t.Error("should still be cooling down before the exhausted cooldown elapses")
	}
	now = now.Add(2 * time.Minute) // 6m total
	if !cb.Allow() {
		t.Error("should probe once the cooldown elapses")
	}
}

func TestCircuitBreaker_SingleEmptyDoesNotOpen(t *testing.T) {
	cb := newCircuitBreaker("test", 1, time.Minute)
	cb.emptyThreshold = 3

	cb.RecordEmptyResponse()
	if cb.State() != stateClosed {
		t.Error("a single empty must not open the circuit (cannot be proven to be exhaustion)")
	}
	if !cb.Allow() {
		t.Error("provider should still be tried after a single empty")
	}
}

func TestCircuitBreaker_HalfOpenEmptyPastThreshold_ReOpens(t *testing.T) {
	now := time.Now()
	cb := newCircuitBreaker("test", 1, 5*time.Minute)
	cb.emptyThreshold = 3
	cb.nowFunc = func() time.Time { return now }

	// Drive 3 consecutive empties → open.
	cb.RecordEmptyResponse()
	cb.RecordEmptyResponse()
	cb.RecordEmptyResponse()
	if cb.State() != stateOpen {
		t.Fatal("should be open after 3 empties")
	}

	// Cooldown elapses → half-open probe is allowed.
	now = now.Add(6 * time.Minute)
	if !cb.Allow() {
		t.Fatal("should allow a half-open probe after cooldown")
	}
	if cb.State() != stateHalfOpen {
		t.Fatalf("should be half-open, got %v", cb.State())
	}

	// The probe itself comes back empty (still past threshold): must re-open
	// and re-arm, not strand or close.
	cb.RecordEmptyResponse()
	if cb.State() != stateOpen {
		t.Fatalf("an empty half-open probe past threshold must re-open, got %v", cb.State())
	}
	if cb.Allow() {
		t.Error("should be cooling down again immediately after the re-arm")
	}
}

func TestCircuitBreaker_EmptyThresholdDisabled_NeverOpens(t *testing.T) {
	// emptyThreshold <= 0 disables circuit-opening on empty entirely: empty
	// responses then only ever fail over (the pre-existing behavior), never
	// open the circuit, no matter how many in a row.
	cb := newCircuitBreaker("test", 1, time.Minute)
	cb.emptyThreshold = 0

	for i := 0; i < 10; i++ {
		cb.RecordEmptyResponse()
	}
	if cb.State() != stateClosed {
		t.Errorf("with emptyThreshold<=0 the circuit must never open on empty, got %v", cb.State())
	}
	if !cb.Allow() {
		t.Error("provider should still be tried (disabled = fail-over only)")
	}
}

func TestCircuitBreaker_EmptyStreakResetsOnSuccess(t *testing.T) {
	cb := newCircuitBreaker("test", 1, time.Minute)
	cb.emptyThreshold = 3

	cb.RecordEmptyResponse()
	cb.RecordEmptyResponse()
	cb.RecordSuccess() // a real (non-empty) success breaks the streak
	cb.RecordEmptyResponse()
	cb.RecordEmptyResponse()
	if cb.State() != stateClosed {
		t.Error("non-consecutive empties (broken by a success) must not open the circuit")
	}
}

func TestCircuitState_String(t *testing.T) {
	tests := []struct {
		state circuitState
		want  string
	}{
		{stateClosed, "closed"},
		{stateOpen, "open"},
		{stateHalfOpen, "half-open"},
		{circuitState(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("circuitState(%d).String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}

func TestCircuitBreaker_Allow_DefaultBranch(t *testing.T) {
	// Force an invalid state to exercise the default branch in Allow().
	cb := newCircuitBreaker("test", 1, time.Minute)
	cb.mu.Lock()
	cb.state = circuitState(99) // invalid state
	cb.mu.Unlock()

	if !cb.Allow() {
		t.Error("default branch in Allow should return true")
	}
}

func TestCircuitBreaker_SuccessResetsFailureCount(t *testing.T) {
	cb := newCircuitBreaker("test", 3, 5*time.Minute)

	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordSuccess()

	// After reset, need 3 more failures to open.
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != stateClosed {
		t.Error("should still be closed (failures reset)")
	}

	cb.RecordFailure()
	if cb.State() != stateOpen {
		t.Error("should open after 3 consecutive failures")
	}
}
