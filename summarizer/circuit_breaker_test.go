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
