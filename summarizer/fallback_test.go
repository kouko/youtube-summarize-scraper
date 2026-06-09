package summarizer

import (
	"fmt"
	"testing"
	"time"
)

// mockSummarizer is a test double that returns pre-configured results.
type mockSummarizer struct {
	name     string // provider name for SummarizeResult
	calls    int
	results  []mockResult
	lastOpts SummarizeOptions // captures the last opts received
}

type mockResult struct {
	text string
	err  error
}

func (m *mockSummarizer) Summarize(text string, opts SummarizeOptions) (SummarizeResult, error) {
	m.lastOpts = opts
	idx := m.calls
	m.calls++
	if idx < len(m.results) {
		if m.results[idx].err != nil {
			return SummarizeResult{}, m.results[idx].err
		}
		return SummarizeResult{
			Text:     m.results[idx].text,
			Provider: m.name,
			Model:    "mock-model",
		}, nil
	}
	return SummarizeResult{}, fmt.Errorf("no more mock results")
}

func newMock(name string, results ...mockResult) *mockSummarizer {
	return &mockSummarizer{name: name, results: results}
}

func newFallback(entries ...providerEntry) *FallbackSummarizer {
	return &FallbackSummarizer{providers: entries}
}

func makeEntry(name string, mock *mockSummarizer) providerEntry {
	return providerEntry{
		name:    name,
		impl:    mock,
		breaker: newCircuitBreaker(name, 1, 5*time.Minute),
	}
}

func TestFallback_PrimarySucceeds(t *testing.T) {
	primary := newMock("primary", mockResult{text: "primary result"})
	fallback := newMock("fallback", mockResult{text: "fallback result"})

	f := newFallback(makeEntry("primary", primary), makeEntry("fallback", fallback))

	result, err := f.Summarize("test", SummarizeOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "primary result" {
		t.Errorf("got %q, want %q", result.Text, "primary result")
	}
	if result.Provider != "primary" {
		t.Errorf("provider: got %q, want %q", result.Provider, "primary")
	}
	if fallback.calls != 0 {
		t.Error("fallback should not have been called")
	}
}

func TestFallback_PrimaryQuotaError_UsesFallback(t *testing.T) {
	primary := newMock("primary", mockResult{err: &QuotaError{Provider: "primary", Err: fmt.Errorf("429")}})
	fallback := newMock("fallback", mockResult{text: "fallback result"})

	f := newFallback(makeEntry("primary", primary), makeEntry("fallback", fallback))

	result, err := f.Summarize("test", SummarizeOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "fallback result" {
		t.Errorf("got %q, want %q", result.Text, "fallback result")
	}
	if result.Provider != "fallback" {
		t.Errorf("provider: got %q, want %q", result.Provider, "fallback")
	}
	if primary.calls != 1 {
		t.Error("primary should have been called once")
	}
	if fallback.calls != 1 {
		t.Error("fallback should have been called once")
	}
}

func TestFallback_AllProvidersQuotaError(t *testing.T) {
	primary := newMock("primary", mockResult{err: &QuotaError{Provider: "primary", Err: fmt.Errorf("429")}})
	fallback := newMock("fallback", mockResult{err: &QuotaError{Provider: "fallback", Err: fmt.Errorf("429")}})

	f := newFallback(makeEntry("primary", primary), makeEntry("fallback", fallback))

	_, err := f.Summarize("test", SummarizeOptions{})
	if err == nil {
		t.Fatal("expected error when all providers fail")
	}
}

func TestFallback_NonQuotaError_TriesFallbackWithoutOpeningCircuit(t *testing.T) {
	primary := newMock("primary",
		mockResult{err: fmt.Errorf("network timeout")},
		mockResult{text: "primary works now"},
	)
	fallback := newMock("fallback", mockResult{text: "fallback result"})

	f := newFallback(makeEntry("primary", primary), makeEntry("fallback", fallback))

	// First call: primary fails with non-quota error, fallback succeeds.
	result, err := f.Summarize("test", SummarizeOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "fallback result" {
		t.Errorf("got %q, want %q", result.Text, "fallback result")
	}

	// Second call: primary should be tried again (circuit not opened).
	result, err = f.Summarize("test", SummarizeOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "primary works now" {
		t.Errorf("got %q, want %q", result.Text, "primary works now")
	}
}

func TestFallback_PrimaryRecovery(t *testing.T) {
	now := time.Now()

	// Primary: first call quota error, second call succeeds.
	primary := newMock("primary",
		mockResult{err: &QuotaError{Provider: "primary", Err: fmt.Errorf("429")}},
		mockResult{text: "primary recovered"},
	)
	fallback := newMock("fallback",
		mockResult{text: "fallback 1"},
		mockResult{text: "fallback 2"},
	)

	pEntry := makeEntry("primary", primary)
	pEntry.breaker.nowFunc = func() time.Time { return now }
	fEntry := makeEntry("fallback", fallback)

	f := newFallback(pEntry, fEntry)

	// Call 1: primary quota error → use fallback.
	result, err := f.Summarize("test", SummarizeOptions{})
	if err != nil {
		t.Fatalf("call 1: unexpected error: %v", err)
	}
	if result.Text != "fallback 1" {
		t.Errorf("call 1: got %q, want %q", result.Text, "fallback 1")
	}
	if pEntry.breaker.State() != stateOpen {
		t.Error("primary circuit should be open")
	}

	// Call 2: primary circuit open, skip to fallback.
	result, err = f.Summarize("test", SummarizeOptions{})
	if err != nil {
		t.Fatalf("call 2: unexpected error: %v", err)
	}
	if result.Text != "fallback 2" {
		t.Errorf("call 2: got %q, want %q", result.Text, "fallback 2")
	}

	// Advance time past cooldown.
	pEntry.breaker.nowFunc = func() time.Time { return now.Add(6 * time.Minute) }

	// Call 3: cooldown expired, primary tried again (half-open) → succeeds.
	result, err = f.Summarize("test", SummarizeOptions{})
	if err != nil {
		t.Fatalf("call 3: unexpected error: %v", err)
	}
	if result.Text != "primary recovered" {
		t.Errorf("call 3: got %q, want %q", result.Text, "primary recovered")
	}
	if result.Provider != "primary" {
		t.Errorf("call 3: provider got %q, want %q", result.Provider, "primary")
	}
	if pEntry.breaker.State() != stateClosed {
		t.Error("primary circuit should be closed after recovery")
	}
}

func TestFallback_HalfOpenNonQuotaError_DoesNotStrandProvider(t *testing.T) {
	now := time.Now()

	// Primary: quota error (opens circuit), then a NON-quota error during the
	// half-open probe, then recovers.
	primary := newMock("primary",
		mockResult{err: &QuotaError{Provider: "primary", Err: fmt.Errorf("429")}},
		mockResult{err: fmt.Errorf("network timeout")},
		mockResult{text: "primary recovered"},
	)
	fallback := newMock("fallback",
		mockResult{text: "fallback 1"},
		mockResult{text: "fallback 2"},
		mockResult{text: "fallback 3"},
	)

	pEntry := makeEntry("primary", primary)
	pEntry.breaker.nowFunc = func() time.Time { return now }
	fEntry := makeEntry("fallback", fallback)

	f := newFallback(pEntry, fEntry)

	// Call 1: primary quota error → open circuit, use fallback.
	if _, err := f.Summarize("t", SummarizeOptions{}); err != nil {
		t.Fatalf("call 1: unexpected error: %v", err)
	}
	if pEntry.breaker.State() != stateOpen {
		t.Fatal("circuit should be open after quota error")
	}

	// Advance past cooldown → next call probes primary in half-open.
	now = now.Add(6 * time.Minute)

	// Call 2: half-open probe; primary returns a NON-quota error.
	// This must NOT strand the circuit in half-open forever.
	result, err := f.Summarize("t", SummarizeOptions{})
	if err != nil {
		t.Fatalf("call 2: unexpected error: %v", err)
	}
	if result.Provider != "fallback" {
		t.Errorf("call 2: provider got %q, want %q", result.Provider, "fallback")
	}

	// Advance past cooldown again so the re-armed circuit can probe.
	now = now.Add(6 * time.Minute)

	// Call 3: primary has recovered and MUST be retried, not skipped.
	result, err = f.Summarize("t", SummarizeOptions{})
	if err != nil {
		t.Fatalf("call 3: unexpected error: %v", err)
	}
	if result.Provider != "primary" {
		t.Errorf("call 3: provider got %q, want %q (circuit stranded in half-open)", result.Provider, "primary")
	}
	if result.Text != "primary recovered" {
		t.Errorf("call 3: text got %q, want %q", result.Text, "primary recovered")
	}
}

func TestFallback_ModelNotPassedToFallbackProvider(t *testing.T) {
	primary := newMock("primary", mockResult{err: &QuotaError{Provider: "primary", Err: fmt.Errorf("429")}})
	fallback := newMock("fallback", mockResult{text: "ok"})

	f := newFallback(makeEntry("primary", primary), makeEntry("fallback", fallback))

	// Call with a model set (simulating pipeline passing primary's model).
	_, err := f.Summarize("test", SummarizeOptions{Model: "auto", MaxTokens: 2000})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The fallback provider should receive empty Model so it uses its own.
	if fallback.lastOpts.Model != "" {
		t.Errorf("fallback received Model=%q, want empty (should use its own)", fallback.lastOpts.Model)
	}
	// Other opts fields should be preserved.
	if fallback.lastOpts.MaxTokens != 2000 {
		t.Errorf("fallback MaxTokens=%d, want 2000", fallback.lastOpts.MaxTokens)
	}
}

func TestFallback_NoProviders(t *testing.T) {
	f := newFallback()
	_, err := f.Summarize("test", SummarizeOptions{})
	if err == nil {
		t.Fatal("expected error with no providers")
	}
}

func TestFallback_ProviderInfoPropagated(t *testing.T) {
	// Verify that when fallback handles the request, the result carries
	// the actual provider's name, not the primary's.
	primary := newMock("gemini-cli", mockResult{err: &QuotaError{Provider: "gemini-cli", Err: fmt.Errorf("429")}})
	fallback := newMock("qwen-code", mockResult{text: "summary from qwen"})

	f := newFallback(makeEntry("gemini-cli", primary), makeEntry("qwen-code", fallback))

	result, err := f.Summarize("test", SummarizeOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Provider != "qwen-code" {
		t.Errorf("provider: got %q, want %q", result.Provider, "qwen-code")
	}
	if result.Text != "summary from qwen" {
		t.Errorf("text: got %q, want %q", result.Text, "summary from qwen")
	}
}
