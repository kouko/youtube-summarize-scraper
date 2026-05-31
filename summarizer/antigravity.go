package summarizer

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// AntigravityCLISummarizer invokes the Google Antigravity CLI (`agy`) in
// headless print mode. Prompt is passed via stdin to avoid ARG_MAX limits on
// long transcriptions.
//
// Unlike the Gemini CLI, agy print mode has no per-call model flag: the model
// is chosen interactively via the /model slash command and persists across
// sessions. There is therefore no Model field here — config exposes only path
// and timeout. (Confirmed against agy 1.0.3 `--help` and antigravity.google docs.)
type AntigravityCLISummarizer struct {
	binaryPath string
	timeout    time.Duration
}

func (a *AntigravityCLISummarizer) Summarize(text string, opts SummarizeOptions) (SummarizeResult, error) {
	combinedPrompt := resolvePrompt(text, opts)

	binary := a.binaryPath
	if binary == "" {
		var err error
		binary, err = exec.LookPath("agy")
		if err != nil {
			return SummarizeResult{}, fmt.Errorf("antigravity-cli: binary not found in PATH: %w", err)
		}
	}

	timeout := a.timeout
	if timeout == 0 {
		timeout = 15 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Pass the prompt via stdin to avoid OS ARG_MAX limits on long transcripts.
	// agy's -p/--print is a string flag that REQUIRES a value, so we give it the
	// path "/dev/stdin" — agy then reads the prompt from fd 0, which exec wires to
	// cmd.Stdin below. (Unix-only path; we ship macOS/Linux. Verified on agy 1.0.3.)
	// --print-timeout bounds the headless wait (agy default is 5m).
	// No model flag exists in agy print mode (see type doc).
	args := []string{"-p", "/dev/stdin", "--print-timeout", timeout.String()}
	cmd := exec.CommandContext(ctx, binary, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdin = strings.NewReader(combinedPrompt)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		combined := stderr.String() + "\n" + stdout.String()
		baseErr := fmt.Errorf("antigravity-cli: execution failed: %w\nstderr: %s", err, stderr.String())
		if isQuotaMessage(combined) {
			return SummarizeResult{}, &QuotaError{Provider: "antigravity-cli", Err: baseErr}
		}
		return SummarizeResult{}, baseErr
	}

	return SummarizeResult{
		Text:     StripThinkingTags(strings.TrimSpace(stdout.String())),
		Provider: "antigravity-cli",
	}, nil
}
