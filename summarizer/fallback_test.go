package summarizer

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// mockSummarizer is a test double that returns pre-configured results.
type mockSummarizer struct {
	name     string // provider name for SummarizeResult
	calls    int
	results  []mockResult
	lastOpts SummarizeOptions // captures the last opts received
	lastText string           // captures the last input text received
}

type mockResult struct {
	text string
	err  error
}

func (m *mockSummarizer) Summarize(text string, opts SummarizeOptions) (SummarizeResult, error) {
	m.lastOpts = opts
	m.lastText = text
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

func TestFallback_QuotaError_ImmediatelySwitchesSameTask(t *testing.T) {
	const task = "the full transcript of the video to be summarized"
	opts := SummarizeOptions{Prompt: "PROMPT: summarize the transcript", MaxTokens: 1500}

	// Primary is out of quota; the fallback handles the request.
	primary := newMock("primary", mockResult{
		err: &QuotaError{Provider: "primary", Err: fmt.Errorf("429 You exceeded your current quota")},
	})
	fallback := newMock("fallback", mockResult{text: "summary from fallback"})

	f := newFallback(makeEntry("primary", primary), makeEntry("fallback", fallback))

	result, err := f.Summarize(task, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// (1) It switched to the next provider for THIS same request and returned
	//     that provider's result — not an error, not a wait.
	if result.Provider != "fallback" || result.Text != "summary from fallback" {
		t.Errorf("got provider=%q text=%q, want the fallback's result", result.Provider, result.Text)
	}

	// (2) The switch was immediate within the one call: primary was tried
	//     exactly once (no retry on the quota'd provider), fallback exactly once.
	if primary.calls != 1 {
		t.Errorf("primary calls = %d, want 1 (no same-provider retry)", primary.calls)
	}
	if fallback.calls != 1 {
		t.Errorf("fallback calls = %d, want 1", fallback.calls)
	}

	// (3) The SAME task was handed to the fallback — identical transcript and
	//     prompt/options (Model is intentionally cleared so each provider uses
	//     its own configured model).
	if fallback.lastText != task {
		t.Errorf("fallback received text %q, want the same task %q", fallback.lastText, task)
	}
	if fallback.lastOpts.Prompt != opts.Prompt {
		t.Errorf("fallback received prompt %q, want %q", fallback.lastOpts.Prompt, opts.Prompt)
	}
	if fallback.lastOpts.MaxTokens != opts.MaxTokens {
		t.Errorf("fallback received MaxTokens %d, want %d", fallback.lastOpts.MaxTokens, opts.MaxTokens)
	}
}

func TestFallback_RetryAfterDrivesCooldown(t *testing.T) {
	now := time.Now()

	// Primary fails with a quota error carrying a 120s Retry-After, then recovers.
	primary := newMock("primary",
		mockResult{err: &QuotaError{
			Provider:   "primary",
			Err:        fmt.Errorf("429"),
			Kind:       KindRateLimit,
			RetryAfter: 120 * time.Second,
		}},
		mockResult{text: "primary recovered"},
	)
	fallback := newMock("fallback",
		mockResult{text: "fallback 1"},
		mockResult{text: "fallback 2"},
	)

	pEntry := makeEntry("primary", primary) // default cooldown 5m
	pEntry.breaker.nowFunc = func() time.Time { return now }
	f := newFallback(pEntry, makeEntry("fallback", fallback))

	// Call 1: quota error with 120s advice → open with a 120s cooldown.
	if _, err := f.Summarize("t", SummarizeOptions{}); err != nil {
		t.Fatalf("call 1: %v", err)
	}

	// At 121s (> advised 120s, but < default 5m) the primary must be probed
	// again — proving the server-advised Retry-After overrode the 5m default.
	now = now.Add(121 * time.Second)
	result, err := f.Summarize("t", SummarizeOptions{})
	if err != nil {
		t.Fatalf("call 2: %v", err)
	}
	if result.Provider != "primary" {
		t.Errorf("provider got %q, want %q (Retry-After not honored)", result.Provider, "primary")
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

func TestFallback_HalfOpenEmptyResponse_DoesNotStrandProvider(t *testing.T) {
	now := time.Now()

	// Primary: quota error (opens circuit), then an EMPTY response during the
	// half-open probe, then recovers. An empty probe must re-arm the cooldown
	// (same as a non-quota error), not strand the circuit in half-open.
	primary := newMock("primary",
		mockResult{err: &QuotaError{Provider: "primary", Err: fmt.Errorf("429")}},
		mockResult{text: ""}, // half-open probe returns empty
		mockResult{text: "primary recovered"},
	)
	fallback := newMock("fallback",
		mockResult{text: "fallback 1"},
		mockResult{text: "fallback 2"},
		mockResult{text: "fallback 3"},
	)

	pEntry := makeEntry("primary", primary)
	pEntry.breaker.nowFunc = func() time.Time { return now }
	f := newFallback(pEntry, makeEntry("fallback", fallback))

	if _, err := f.Summarize("t", SummarizeOptions{}); err != nil {
		t.Fatalf("call 1: %v", err)
	}
	if pEntry.breaker.State() != stateOpen {
		t.Fatal("circuit should be open after quota error")
	}

	now = now.Add(6 * time.Minute)
	result, err := f.Summarize("t", SummarizeOptions{}) // half-open probe → empty
	if err != nil {
		t.Fatalf("call 2: %v", err)
	}
	if result.Provider != "fallback" {
		t.Errorf("call 2: provider got %q, want fallback", result.Provider)
	}

	now = now.Add(6 * time.Minute)
	result, err = f.Summarize("t", SummarizeOptions{}) // re-armed → probe primary
	if err != nil {
		t.Fatalf("call 3: %v", err)
	}
	if result.Provider != "primary" || result.Text != "primary recovered" {
		t.Errorf("call 3: got provider=%q text=%q, want primary/'primary recovered' (stranded?)", result.Provider, result.Text)
	}
}

func TestFallback_ConsecutiveEmpties_OpenCircuitAndSkipProvider(t *testing.T) {
	now := time.Now()

	// Primary always returns empty; fallback always works. After emptyThreshold
	// (default 3) consecutive empties, primary's circuit opens and it stops
	// being retried first on every request.
	primary := newMock("primary",
		mockResult{text: ""}, mockResult{text: ""}, mockResult{text: ""},
		mockResult{text: ""}, mockResult{text: ""},
	)
	fallback := newMock("fallback",
		mockResult{text: "ok1"}, mockResult{text: "ok2"}, mockResult{text: "ok3"},
		mockResult{text: "ok4"}, mockResult{text: "ok5"},
	)
	pEntry := makeEntry("primary", primary) // emptyThreshold defaults to 3
	pEntry.breaker.nowFunc = func() time.Time { return now }
	f := newFallback(pEntry, makeEntry("fallback", fallback))

	// 3 calls: each empty from primary → failover; the 3rd opens primary's circuit.
	for i := 0; i < 3; i++ {
		if _, err := f.Summarize("t", SummarizeOptions{}); err != nil {
			t.Fatalf("call %d: %v", i+1, err)
		}
	}
	if pEntry.breaker.State() != stateOpen {
		t.Fatalf("primary circuit should open after 3 consecutive empties, got %v", pEntry.breaker.State())
	}
	callsBeforeOpen := primary.calls // 3

	// 4th call: primary's circuit is open → it must be skipped, not called again.
	if _, err := f.Summarize("t", SummarizeOptions{}); err != nil {
		t.Fatalf("call 4: %v", err)
	}
	if primary.calls != callsBeforeOpen {
		t.Errorf("primary should be SKIPPED while its circuit is open, but it was called again (%d → %d)",
			callsBeforeOpen, primary.calls)
	}
}

func TestFallback_EmptyResponse_FailsOverToNextProvider(t *testing.T) {
	// A 2xx-but-empty response (e.g. antigravity-cli exits 0 with no output
	// when its quota is exhausted) must be treated as this provider failing,
	// so the chain fails over instead of returning a blank summary.
	primary := newMock("primary", mockResult{text: ""}) // empty, nil error
	fallback := newMock("fallback", mockResult{text: "real summary"})

	f := newFallback(makeEntry("primary", primary), makeEntry("fallback", fallback))

	result, err := f.Summarize("t", SummarizeOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Provider != "fallback" || result.Text != "real summary" {
		t.Errorf("expected fallback's result, got provider=%q text=%q", result.Provider, result.Text)
	}
	if primary.calls != 1 || fallback.calls != 1 {
		t.Errorf("calls: primary=%d fallback=%d, want 1/1", primary.calls, fallback.calls)
	}
}

func TestFallback_WhitespaceOnlyResponse_TreatedAsEmpty(t *testing.T) {
	primary := newMock("primary", mockResult{text: "   \n\t  "})
	fallback := newMock("fallback", mockResult{text: "real"})

	f := newFallback(makeEntry("primary", primary), makeEntry("fallback", fallback))

	result, err := f.Summarize("t", SummarizeOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "real" {
		t.Errorf("whitespace-only response should be treated as empty → failover, got %q", result.Text)
	}
}

func TestFallback_AllEmpty_ReturnsErrorNotBlank(t *testing.T) {
	primary := newMock("primary", mockResult{text: ""})
	fallback := newMock("fallback", mockResult{text: ""})

	f := newFallback(makeEntry("primary", primary), makeEntry("fallback", fallback))

	result, err := f.Summarize("t", SummarizeOptions{})
	if err == nil {
		t.Fatalf("all-empty chain should return an error, got blank success: %+v", result)
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error should mention the empty response, got: %v", err)
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
