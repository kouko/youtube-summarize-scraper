package summarizer

import (
	"fmt"
	"log/slog"
)

// providerEntry pairs a Summarizer implementation with its circuit breaker.
type providerEntry struct {
	name    string
	impl    Summarizer
	breaker *CircuitBreaker
}

// FallbackSummarizer wraps multiple Summarizer backends with circuit breakers.
// It tries providers in priority order, skipping those with open circuits.
// Only QuotaErrors trigger the circuit breaker; other errors try the next
// provider without penalizing the current one.
type FallbackSummarizer struct {
	providers []providerEntry
}

func (f *FallbackSummarizer) Summarize(text string, opts SummarizeOptions) (SummarizeResult, error) {
	var lastErr error

	for _, p := range f.providers {
		if !p.breaker.Allow() {
			slog.Debug("provider circuit open, skipping", "provider", p.name)
			continue
		}

		// Clear Model so each provider uses its own configured model,
		// not the primary provider's model (e.g., gemini's "auto" would
		// fail on claude-code which expects "haiku"/"sonnet"/etc.).
		providerOpts := opts
		providerOpts.Model = ""
		result, err := p.impl.Summarize(text, providerOpts)
		if err == nil {
			p.breaker.RecordSuccess()
			return result, nil // result already contains actual provider/model
		}

		if qe := asQuotaError(err); qe != nil {
			p.breaker.RecordQuotaFailure(qe.RetryAfter, qe.Kind)
			slog.Warn("provider quota exceeded, trying fallback",
				"provider", p.name, "kind", qe.Kind, "retry_after", qe.RetryAfter, "error", err)
			lastErr = err
			continue
		}

		// Non-quota error: try next provider for this request,
		// but don't open the circuit (provider itself is healthy).
		// Still inform the breaker so a half-open probe isn't stranded —
		// otherwise the provider stays half-open and is skipped forever.
		p.breaker.RecordInconclusive()
		slog.Warn("provider error (non-quota), trying fallback",
			"provider", p.name, "error", err)
		lastErr = err
		continue
	}

	if lastErr != nil {
		return SummarizeResult{}, fmt.Errorf("all providers failed: %w", lastErr)
	}
	return SummarizeResult{}, fmt.Errorf("no providers configured")
}
