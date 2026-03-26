package summarizer

import (
	"errors"
	"fmt"
	"strings"
)

// QuotaError indicates that an LLM provider rejected the request due to
// quota exhaustion or rate limiting. The circuit breaker uses this to
// decide when to skip a provider and try the next fallback.
type QuotaError struct {
	Provider string
	Err      error
}

func (e *QuotaError) Error() string {
	return fmt.Sprintf("%s: quota/rate limit exceeded: %v", e.Provider, e.Err)
}

func (e *QuotaError) Unwrap() error { return e.Err }

// IsQuotaError reports whether err (or any error in its chain) is a QuotaError.
func IsQuotaError(err error) bool {
	var qe *QuotaError
	return errors.As(err, &qe)
}

// quotaPatterns are substrings that indicate a quota or rate-limit error
// in LLM provider responses. Checked case-insensitively.
var quotaPatterns = []string{
	"resource_exhausted",
	"quota",
	"rate limit",
	"rate_limit",
	"429",
	"too many requests",
	"overloaded",
}

// isQuotaMessage checks whether a message string contains indicators of
// quota exhaustion or rate limiting.
func isQuotaMessage(msg string) bool {
	lower := strings.ToLower(msg)
	for _, pattern := range quotaPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}
