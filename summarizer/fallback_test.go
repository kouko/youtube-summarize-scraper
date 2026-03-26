package summarizer

import (
	"fmt"
	"testing"
	"time"
)

// mockSummarizer is a test double that returns pre-configured results.
type mockSummarizer struct {
	calls    int
	results  []mockResult
	lastOpts SummarizeOptions // captures the last opts received
}

type mockResult struct {
	text string
	err  error
}

func (m *mockSummarizer) Summarize(text string, opts SummarizeOptions) (string, error) {
	m.lastOpts = opts
	idx := m.calls
	m.calls++
	if idx < len(m.results) {
		return m.results[idx].text, m.results[idx].err
	}
	return "", fmt.Errorf("no more mock results")
}

func newMock(results ...mockResult) *mockSummarizer {
	return &mockSummarizer{results: results}
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
	primary := newMock(mockResult{text: "primary result"})
	fallback := newMock(mockResult{text: "fallback result"})

	f := newFallback(makeEntry("primary", primary), makeEntry("fallback", fallback))

	result, err := f.Summarize("test", SummarizeOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "primary result" {
		t.Errorf("got %q, want %q", result, "primary result")
	}
	if fallback.calls != 0 {
		t.Error("fallback should not have been called")
	}
}

func TestFallback_PrimaryQuotaError_UsesFallback(t *testing.T) {
	primary := newMock(mockResult{err: &QuotaError{Provider: "primary", Err: fmt.Errorf("429")}})
	fallback := newMock(mockResult{text: "fallback result"})

	f := newFallback(makeEntry("primary", primary), makeEntry("fallback", fallback))

	result, err := f.Summarize("test", SummarizeOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "fallback result" {
		t.Errorf("got %q, want %q", result, "fallback result")
	}
	if primary.calls != 1 {
		t.Error("primary should have been called once")
	}
	if fallback.calls != 1 {
		t.Error("fallback should have been called once")
	}
}

func TestFallback_AllProvidersQuotaError(t *testing.T) {
	primary := newMock(mockResult{err: &QuotaError{Provider: "primary", Err: fmt.Errorf("429")}})
	fallback := newMock(mockResult{err: &QuotaError{Provider: "fallback", Err: fmt.Errorf("429")}})

	f := newFallback(makeEntry("primary", primary), makeEntry("fallback", fallback))

	_, err := f.Summarize("test", SummarizeOptions{})
	if err == nil {
		t.Fatal("expected error when all providers fail")
	}
}

func TestFallback_NonQuotaError_TriesFallbackWithoutOpeningCircuit(t *testing.T) {
	primary := newMock(
		mockResult{err: fmt.Errorf("network timeout")},
		mockResult{text: "primary works now"},
	)
	fallback := newMock(mockResult{text: "fallback result"})

	f := newFallback(makeEntry("primary", primary), makeEntry("fallback", fallback))

	// First call: primary fails with non-quota error, fallback succeeds.
	result, err := f.Summarize("test", SummarizeOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "fallback result" {
		t.Errorf("got %q, want %q", result, "fallback result")
	}

	// Second call: primary should be tried again (circuit not opened).
	result, err = f.Summarize("test", SummarizeOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "primary works now" {
		t.Errorf("got %q, want %q", result, "primary works now")
	}
}

func TestFallback_PrimaryRecovery(t *testing.T) {
	now := time.Now()

	// Primary: first call quota error, second call succeeds.
	primary := newMock(
		mockResult{err: &QuotaError{Provider: "primary", Err: fmt.Errorf("429")}},
		mockResult{text: "primary recovered"},
	)
	fallback := newMock(
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
	if result != "fallback 1" {
		t.Errorf("call 1: got %q, want %q", result, "fallback 1")
	}
	if pEntry.breaker.State() != stateOpen {
		t.Error("primary circuit should be open")
	}

	// Call 2: primary circuit open, skip to fallback.
	result, err = f.Summarize("test", SummarizeOptions{})
	if err != nil {
		t.Fatalf("call 2: unexpected error: %v", err)
	}
	if result != "fallback 2" {
		t.Errorf("call 2: got %q, want %q", result, "fallback 2")
	}

	// Advance time past cooldown.
	pEntry.breaker.nowFunc = func() time.Time { return now.Add(6 * time.Minute) }

	// Call 3: cooldown expired, primary tried again (half-open) → succeeds.
	result, err = f.Summarize("test", SummarizeOptions{})
	if err != nil {
		t.Fatalf("call 3: unexpected error: %v", err)
	}
	if result != "primary recovered" {
		t.Errorf("call 3: got %q, want %q", result, "primary recovered")
	}
	if pEntry.breaker.State() != stateClosed {
		t.Error("primary circuit should be closed after recovery")
	}
}

func TestFallback_ModelNotPassedToFallbackProvider(t *testing.T) {
	primary := newMock(mockResult{err: &QuotaError{Provider: "primary", Err: fmt.Errorf("429")}})
	fallback := newMock(mockResult{text: "ok"})

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
