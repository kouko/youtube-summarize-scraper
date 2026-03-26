package summarizer

import (
	"errors"
	"fmt"
	"testing"
)

func TestIsQuotaError(t *testing.T) {
	qe := &QuotaError{Provider: "test", Err: fmt.Errorf("quota hit")}
	if !IsQuotaError(qe) {
		t.Error("IsQuotaError should return true for *QuotaError")
	}

	wrapped := fmt.Errorf("outer: %w", qe)
	if !IsQuotaError(wrapped) {
		t.Error("IsQuotaError should return true for wrapped *QuotaError")
	}

	plain := fmt.Errorf("some other error")
	if IsQuotaError(plain) {
		t.Error("IsQuotaError should return false for plain error")
	}

	if IsQuotaError(nil) {
		t.Error("IsQuotaError should return false for nil")
	}
}

func TestQuotaError_Unwrap(t *testing.T) {
	inner := fmt.Errorf("inner error")
	qe := &QuotaError{Provider: "test", Err: inner}
	if !errors.Is(qe, inner) {
		t.Error("QuotaError.Unwrap should return the inner error")
	}
}

func TestQuotaError_Error(t *testing.T) {
	qe := &QuotaError{Provider: "gemini-cli", Err: fmt.Errorf("429 too many requests")}
	msg := qe.Error()
	if msg != "gemini-cli: quota/rate limit exceeded: 429 too many requests" {
		t.Errorf("unexpected error message: %q", msg)
	}
}

func TestIsQuotaMessage(t *testing.T) {
	positives := []string{
		"RESOURCE_EXHAUSTED",
		"You have exceeded your quota",
		"Rate limit exceeded",
		"rate_limit_error",
		"HTTP 429: too many requests",
		"Error 429",
		"Too Many Requests",
		"API is overloaded",
	}
	for _, msg := range positives {
		if !isQuotaMessage(msg) {
			t.Errorf("isQuotaMessage(%q) should be true", msg)
		}
	}

	negatives := []string{
		"connection refused",
		"invalid API key",
		"model not found",
		"context deadline exceeded",
		"empty response",
	}
	for _, msg := range negatives {
		if isQuotaMessage(msg) {
			t.Errorf("isQuotaMessage(%q) should be false", msg)
		}
	}
}
