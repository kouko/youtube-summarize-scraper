package summarizer

import (
	"fmt"
	"log/slog"
	"strings"
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
		if err == nil && strings.TrimSpace(result.Text) != "" {
			p.breaker.RecordSuccess()
			return result, nil // result already contains actual provider/model
		}
		if err == nil {
			// 2xx-but-empty: a "silent failure". Some backends return success
			// with no output when degraded — e.g. antigravity-cli exits 0 with
			// empty stdout/stderr when its quota is exhausted, giving no error
			// and no message to classify as a QuotaError. Treat the empty
			// response itself as this provider failing so the chain fails over
			// instead of returning a blank summary.
			// An empty response on a stage where empty can be legitimate
			// (opts.AllowEmpty — keywords/mermaid) still fails over, but must
			// not count toward opening the circuit.
			if !opts.AllowEmpty {
				p.breaker.RecordEmptyResponse()
			}
			slog.Warn("provider returned an empty response, trying fallback",
				"provider", p.name, "counted", !opts.AllowEmpty)
			lastErr = fmt.Errorf("%s: empty response", p.name)
			continue
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
