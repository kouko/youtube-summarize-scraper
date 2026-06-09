package summarizer

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// QuotaErrorKind distinguishes a short-lived throttle from hard exhaustion.
// The two recover on very different timescales, so the circuit breaker uses
// the kind to pick a cooldown when the server gives no explicit Retry-After.
type QuotaErrorKind int

const (
	// KindRateLimit is a transient per-second/per-minute throttle; recovers in
	// seconds (HTTP 429/529, "too many requests", "overloaded").
	KindRateLimit QuotaErrorKind = iota
	// KindExhausted is hard quota/balance exhaustion; recovers in hours or not
	// until a billing/quota reset ("exceeded your quota", "billing hard limit").
	KindExhausted
)

func (k QuotaErrorKind) String() string {
	switch k {
	case KindRateLimit:
		return "rate-limit"
	case KindExhausted:
		return "exhausted"
	default:
		return "unknown"
	}
}

// QuotaError indicates that an LLM provider rejected the request due to
// quota exhaustion or rate limiting. The circuit breaker uses this to
// decide when to skip a provider and try the next fallback, and how long
// to keep it skipped.
type QuotaError struct {
	Provider string
	Err      error
	// Kind classifies the throttle so the breaker can pick a cooldown when no
	// RetryAfter is known. Zero value (KindRateLimit) is the safe default.
	Kind QuotaErrorKind
	// RetryAfter is the server-advised wait before retrying; 0 if unknown.
	RetryAfter time.Duration
}

func (e *QuotaError) Error() string {
	return fmt.Sprintf("%s: quota/rate limit exceeded: %v", e.Provider, e.Err)
}

func (e *QuotaError) Unwrap() error { return e.Err }

// IsQuotaError reports whether err (or any error in its chain) is a QuotaError.
func IsQuotaError(err error) bool {
	return asQuotaError(err) != nil
}

// asQuotaError returns the *QuotaError in err's chain, or nil if none.
func asQuotaError(err error) *QuotaError {
	var qe *QuotaError
	if errors.As(err, &qe) {
		return qe
	}
	return nil
}

// rateLimitPatterns indicate a transient throttle (short cooldown).
var rateLimitPatterns = []string{
	"rate limit",
	"rate_limit",
	"429",
	"529",
	"too many requests",
	"overloaded",
	"resource_exhausted", // Gemini returns this for per-minute limits too
	"slow down",
}

// exhaustedPatterns indicate hard quota/balance exhaustion (long cooldown).
// Kept to bounded forms grounded in real provider messages — bare tokens like
// "insufficient" / "balance" / "billing" over-match unrelated errors
// ("insufficient permissions", "load balancer", "billing address").
var exhaustedPatterns = []string{
	"quota",         // OpenAI "exceeded your current quota", Gemini quota
	"exceeded your", // "exceeded your quota/limit"
	"hard limit",    // "billing hard limit reached"
	"daily limit",   // "daily limit reached"
	"out of credit", // CLI credit exhaustion
	"insufficient balance",
	"credit balance", // Anthropic "credit balance is too low"
	"usage limit",    // claude-code "Claude AI usage limit reached"
	"spend limit",    // claude-code "hit your extra usage spend limit"
}

// isQuotaMessage checks whether a message string contains indicators of
// quota exhaustion or rate limiting.
func isQuotaMessage(msg string) bool {
	lower := strings.ToLower(msg)
	for _, pattern := range rateLimitPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	for _, pattern := range exhaustedPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

// classifyQuotaKind decides whether a quota message is a transient rate limit
// or hard exhaustion. Exhaustion signals win when both are present, because a
// long cooldown is the safer choice for a provider that is genuinely out of
// quota (an over-long wait merely delays recovery; an over-short one wastes
// repeated probes all day).
func classifyQuotaKind(msg string) QuotaErrorKind {
	lower := strings.ToLower(msg)
	for _, pattern := range exhaustedPatterns {
		if strings.Contains(lower, pattern) {
			return KindExhausted
		}
	}
	return KindRateLimit
}

// retryDelayTextRe matches embedded retry hints CLI providers print, e.g.
// `"retryDelay": "57s"`, "Please retry in 42s", "retry after 30 seconds".
var retryDelayTextRe = regexp.MustCompile(`(?i)(?:retrydelay"?\s*:?\s*"?|retry\s+(?:in|after)\s+)(\d+(?:\.\d+)?)\s*(?:s|sec|secs|second|seconds)`)

// parseRetryAfterFromText extracts a best-effort retry delay from free-form
// provider output (CLI providers have no Retry-After header). Returns 0 if no
// hint is found.
func parseRetryAfterFromText(msg string) time.Duration {
	m := retryDelayTextRe.FindStringSubmatch(msg)
	if m == nil {
		return 0
	}
	secs, err := strconv.ParseFloat(m[1], 64)
	if err != nil || secs <= 0 {
		return 0
	}
	return time.Duration(secs * float64(time.Second))
}

// parseRetryAfter parses an HTTP Retry-After header value, which is either a
// number of seconds or an HTTP-date. Returns 0 when absent, unparseable, or
// in the past (clamped, never negative). now is injectable for testing.
func parseRetryAfter(header string, now time.Time) time.Duration {
	header = strings.TrimSpace(header)
	if header == "" {
		return 0
	}
	if secs, err := strconv.Atoi(header); err == nil {
		if secs <= 0 {
			return 0
		}
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(header); err == nil {
		if d := t.Sub(now); d > 0 {
			return d
		}
	}
	return 0
}

// quotaErrorFrom builds a *QuotaError from a provider failure if the status
// code or message indicate quota/rate limiting, else nil. It centralizes the
// detection that was previously duplicated across every provider.
//
//   - statusCode: HTTP status (0 for CLI providers with no HTTP response).
//   - msg: the response body / combined stdout+stderr to scan.
//   - retryAfterHeader: the HTTP Retry-After header value ("" if none).
//
// RetryAfter is taken from the header first, then a best-effort text scan.
func quotaErrorFrom(provider string, baseErr error, statusCode int, msg, retryAfterHeader string, now time.Time) *QuotaError {
	byStatus := statusCode == http.StatusTooManyRequests || statusCode == 529
	if !byStatus && !isQuotaMessage(msg) {
		return nil
	}

	// classifyQuotaKind already defaults to KindRateLimit when no exhaustion
	// keyword is present, which is exactly right for a bare 429/529.
	kind := classifyQuotaKind(msg)

	retryAfter := parseRetryAfter(retryAfterHeader, now)
	if retryAfter == 0 {
		retryAfter = parseRetryAfterFromText(msg)
	}

	return &QuotaError{
		Provider:   provider,
		Err:        baseErr,
		Kind:       kind,
		RetryAfter: retryAfter,
	}
}
