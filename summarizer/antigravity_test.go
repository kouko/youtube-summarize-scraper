package summarizer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeFakeAgy writes an executable shell script to a temp dir that stands in
// for the real `agy` binary, and returns its path. The script body receives
// the prompt on stdin and may inspect "$@" for the arguments agy was called with.
//
// These fakes exercise the summarizer's own contract (stdin wiring, arg
// construction, thinking-strip, quota mapping). They do NOT exercise real agy's
// flag parser — the `-p /dev/stdin` REQUIRES-a-value behavior and the
// --print-timeout flag name are verified by manual end-to-end runs against
// agy 1.0.3, not by this suite.
func writeFakeAgy(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "agy")
	script := "#!/bin/sh\n" + body
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake agy: %v", err)
	}
	return path
}

func TestAntigravityCLISummarize_StripsThinkingAndSetsProvider(t *testing.T) {
	// Fake agy emits a thinking block plus the real content; the summarizer
	// must strip the <think> block and report provider "antigravity-cli".
	fake := writeFakeAgy(t, "printf '<think>plan</think>\\n## Summary\\nbody\\n'\n")

	s := &AntigravityCLISummarizer{binaryPath: fake, timeout: time.Minute}
	res, err := s.Summarize("transcript text", SummarizeOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(res.Text, "<think>") || strings.Contains(res.Text, "plan") {
		t.Errorf("thinking block not stripped: %q", res.Text)
	}
	if !strings.Contains(res.Text, "## Summary") {
		t.Errorf("content missing: %q", res.Text)
	}
	if res.Provider != "antigravity-cli" {
		t.Errorf("provider = %q, want antigravity-cli", res.Provider)
	}
}

func TestAntigravityCLISummarize_PassesPromptViaStdin(t *testing.T) {
	// Fake agy echoes whatever it reads on stdin; the prompt must arrive there
	// (not as an argv flag) to avoid ARG_MAX limits on long transcripts.
	fake := writeFakeAgy(t, "cat\n")

	s := &AntigravityCLISummarizer{binaryPath: fake, timeout: time.Minute}
	res, err := s.Summarize("UNIQUE_TRANSCRIPT_MARKER", SummarizeOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(res.Text, "UNIQUE_TRANSCRIPT_MARKER") {
		t.Errorf("prompt not passed via stdin: %q", res.Text)
	}
}

func TestAntigravityCLISummarize_ArgsHaveNoModelFlag(t *testing.T) {
	// agy print mode has no per-call model flag; assert we send -p and
	// --print-timeout but never -m/--model.
	fake := writeFakeAgy(t, `printf '%s' "$*"` + "\n")

	s := &AntigravityCLISummarizer{binaryPath: fake, timeout: time.Minute}
	res, err := s.Summarize("x", SummarizeOptions{Model: "gemini-3-pro"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(res.Text, "-p") || !strings.Contains(res.Text, "--print-timeout") {
		t.Errorf("missing required flags in args: %q", res.Text)
	}
	if strings.Contains(res.Text, "-m") || strings.Contains(res.Text, "--model") {
		t.Errorf("model flag must not be sent to agy: %q", res.Text)
	}
}

func TestAntigravityCLISummarize_QuotaErrorMapped(t *testing.T) {
	// A non-zero exit whose output signals quota exhaustion must surface as a
	// QuotaError so the circuit breaker can skip to the next provider.
	fake := writeFakeAgy(t, "echo 'error: quota exceeded for this week' >&2\nexit 1\n")

	s := &AntigravityCLISummarizer{binaryPath: fake, timeout: time.Minute}
	_, err := s.Summarize("x", SummarizeOptions{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !IsQuotaError(err) {
		t.Errorf("expected QuotaError, got %T: %v", err, err)
	}
}

func TestAntigravityCLISummarize_NonQuotaErrorNotMisclassified(t *testing.T) {
	// A non-zero exit whose output does NOT signal quota must surface as a
	// plain error, not a QuotaError — otherwise the circuit breaker would
	// wrongly skip the provider on a transient failure.
	fake := writeFakeAgy(t, "echo 'fatal: model gateway unreachable' >&2\nexit 1\n")

	s := &AntigravityCLISummarizer{binaryPath: fake, timeout: time.Minute}
	_, err := s.Summarize("x", SummarizeOptions{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if IsQuotaError(err) {
		t.Errorf("non-quota failure must not be a QuotaError: %v", err)
	}
}
