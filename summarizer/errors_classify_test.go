package summarizer

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestClassifyQuotaKind(t *testing.T) {
	exhausted := []string{
		"You have exceeded your quota",
		"billing hard limit reached",
		"insufficient_quota",
		"insufficient balance",
		"you are out of credit",
		"daily limit reached",
	}
	for _, msg := range exhausted {
		if got := classifyQuotaKind(msg); got != KindExhausted {
			t.Errorf("classifyQuotaKind(%q) = %v, want KindExhausted", msg, got)
		}
	}

	rateLimited := []string{
		"Rate limit exceeded",
		"rate_limit_error",
		"Too Many Requests",
		"HTTP 429",
		"API is overloaded",
		"RESOURCE_EXHAUSTED", // Gemini per-minute throttle; transient by default
		"please slow down",
	}
	for _, msg := range rateLimited {
		if got := classifyQuotaKind(msg); got != KindRateLimit {
			t.Errorf("classifyQuotaKind(%q) = %v, want KindRateLimit", msg, got)
		}
	}
}

func TestParseRetryAfter_Seconds(t *testing.T) {
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	if got := parseRetryAfter("60", now); got != 60*time.Second {
		t.Errorf("parseRetryAfter(\"60\") = %v, want 60s", got)
	}
	if got := parseRetryAfter("", now); got != 0 {
		t.Errorf("parseRetryAfter(\"\") = %v, want 0", got)
	}
	if got := parseRetryAfter("not-a-number", now); got != 0 {
		t.Errorf("parseRetryAfter(garbage) = %v, want 0", got)
	}
}

func TestParseRetryAfter_HTTPDate(t *testing.T) {
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	future := now.Add(45 * time.Second)
	header := future.Format(http.TimeFormat)
	got := parseRetryAfter(header, now)
	// Allow 1s slack for second-granularity formatting.
	if got < 44*time.Second || got > 46*time.Second {
		t.Errorf("parseRetryAfter(date +45s) = %v, want ~45s", got)
	}

	// A past date must clamp to 0, not a negative duration.
	past := now.Add(-30 * time.Second).Format(http.TimeFormat)
	if got := parseRetryAfter(past, now); got != 0 {
		t.Errorf("parseRetryAfter(past date) = %v, want 0", got)
	}
}

func TestParseRetryAfterFromText(t *testing.T) {
	cases := map[string]time.Duration{
		`{"retryDelay": "57s"}`:  57 * time.Second,
		"Please retry in 42s":    42 * time.Second,
		"retry after 30 seconds": 30 * time.Second,
		"no hint here":           0,
		"connection refused":     0,
	}
	for msg, want := range cases {
		if got := parseRetryAfterFromText(msg); got != want {
			t.Errorf("parseRetryAfterFromText(%q) = %v, want %v", msg, got, want)
		}
	}
}

func TestQuotaErrorFrom(t *testing.T) {
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	base := fmt.Errorf("boom")

	// HTTP 429 with a Retry-After header → rate-limit kind, honored wait.
	qe := quotaErrorFrom("openai-compat", base, http.StatusTooManyRequests, "too many requests", "30", now)
	if qe == nil {
		t.Fatal("429 should produce a QuotaError")
	}
	if qe.Kind != KindRateLimit {
		t.Errorf("429 kind = %v, want KindRateLimit", qe.Kind)
	}
	if qe.RetryAfter != 30*time.Second {
		t.Errorf("429 RetryAfter = %v, want 30s", qe.RetryAfter)
	}
	if qe.Provider != "openai-compat" {
		t.Errorf("provider = %q, want openai-compat", qe.Provider)
	}

	// HTTP 529 overloaded, no header → rate-limit, unknown wait.
	if qe := quotaErrorFrom("claude-api", base, 529, "overloaded", "", now); qe == nil || qe.Kind != KindRateLimit || qe.RetryAfter != 0 {
		t.Errorf("529 should be rate-limit with 0 RetryAfter, got %+v", qe)
	}

	// CLI exhaustion message, no status/header → exhausted kind.
	if qe := quotaErrorFrom("gemini-cli", base, 0, "You have exceeded your quota", "", now); qe == nil || qe.Kind != KindExhausted {
		t.Errorf("quota message should be exhausted, got %+v", qe)
	}

	// CLI message with an embedded retryDelay → rate-limit wait from text.
	qe = quotaErrorFrom("gemini-cli", base, 0, `RESOURCE_EXHAUSTED {"retryDelay": "57s"}`, "", now)
	if qe == nil || qe.RetryAfter != 57*time.Second {
		t.Errorf("text retryDelay should be parsed, got %+v", qe)
	}

	// Non-quota error → nil.
	if qe := quotaErrorFrom("ollama", base, 404, "model not found", "", now); qe != nil {
		t.Errorf("non-quota error should return nil, got %+v", qe)
	}
}

func TestQuotaDetection_DoesNotOverMatch(t *testing.T) {
	// Broadened patterns must stay grounded: these non-quota messages share a
	// substring with a quota keyword ("insufficient", "balance", "billing")
	// but are not quota/rate-limit errors and must not trigger a cooldown.
	nonQuota := []string{
		"insufficient permissions",
		"insufficient context provided",
		"load balancer error", // contains "balance"
		"billing address invalid",
	}
	for _, msg := range nonQuota {
		if isQuotaMessage(msg) {
			t.Errorf("isQuotaMessage(%q) should be false (over-match)", msg)
		}
		if qe := quotaErrorFrom("x", fmt.Errorf("e"), 0, msg, "", time.Now()); qe != nil {
			t.Errorf("quotaErrorFrom(%q) should be nil (over-match), got %+v", msg, qe)
		}
	}
}

func TestQuotaDetection_ClaudeCodeUsageLimit(t *testing.T) {
	now := time.Now()
	// Real claude-code CLI subscription-limit messages (Max/Pro plan). These
	// are the most common claude-code limits and carry no HTTP status, so they
	// must be detected from text alone and classified as exhaustion.
	msgs := []string{
		"Claude AI usage limit reached, please try again after 5pm",
		"You've hit your extra usage spend limit",
	}
	for _, msg := range msgs {
		qe := quotaErrorFrom("claude-code", fmt.Errorf("e"), 0, msg, "", now)
		if qe == nil {
			t.Errorf("quotaErrorFrom(%q) should detect a quota error", msg)
			continue
		}
		if qe.Kind != KindExhausted {
			t.Errorf("quotaErrorFrom(%q) kind = %v, want KindExhausted", msg, qe.Kind)
		}
	}

	// The transient claude-code throttle stays a rate limit, not exhaustion.
	if qe := quotaErrorFrom("claude-code", fmt.Errorf("e"), 0, "API Error: Rate limit reached", "", now); qe == nil || qe.Kind != KindRateLimit {
		t.Errorf("rate-limit message should be KindRateLimit, got %+v", qe)
	}
}

func TestIsQuotaMessage_BroadenedPatterns(t *testing.T) {
	positives := []string{
		"billing hard limit reached",
		"insufficient_quota",
		"insufficient balance",
		"you are out of credit",
		"please slow down",
	}
	for _, msg := range positives {
		if !isQuotaMessage(msg) {
			t.Errorf("isQuotaMessage(%q) should be true after broadening", msg)
		}
	}
}
