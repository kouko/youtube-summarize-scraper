package summarizer

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ClaudeCodeSummarizer invokes the Claude Code CLI in headless mode.
// Prompt is passed via stdin with --bare to skip hooks and minimize overhead.
type ClaudeCodeSummarizer struct {
	model      string
	binaryPath string
	timeout    time.Duration
}

func (c *ClaudeCodeSummarizer) Summarize(text string, opts SummarizeOptions) (SummarizeResult, error) {
	model := c.model
	if opts.Model != "" {
		model = opts.Model
	}

	combinedPrompt := resolvePrompt(text, opts)

	binary := c.binaryPath
	if binary == "" {
		var err error
		binary, err = exec.LookPath("claude")
		if err != nil {
			return SummarizeResult{}, fmt.Errorf("claude-code: binary not found in PATH: %w", err)
		}
	}

	timeout := c.timeout
	if timeout == 0 {
		timeout = 15 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// --print: non-interactive mode, read from stdin
	// --output-format text: plain text output
	// --tools "": disable all built-in tools for pure text generation
	//   (prevents the model from attempting file writes instead of returning text)
	// --strict-mcp-config: disable all MCP servers (no --mcp-config provided)
	// --setting-sources "": skip all user/project settings including hooks
	//   (prevents user hooks from blocking automated summarization)
	// Note: --bare is not used because it disables OAuth/keychain auth.
	args := []string{
		"--print",
		"--model", model,
		"--output-format", "text",
		"--tools", "",
		"--strict-mcp-config",
		"--setting-sources", "",
	}
	cmd := exec.CommandContext(ctx, binary, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdin = strings.NewReader(combinedPrompt)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Claude CLI may output errors to stdout (e.g., "Not logged in").
		errMsg := stderr.String()
		if errMsg == "" {
			errMsg = stdout.String()
		}
		baseErr := fmt.Errorf("claude-code: execution failed: %w\noutput: %s", err, errMsg)
		if qe := quotaErrorFrom("claude-code", baseErr, 0, errMsg, "", time.Now()); qe != nil {
			return SummarizeResult{}, qe
		}
		return SummarizeResult{}, baseErr
	}

	return SummarizeResult{
		Text:     StripThinkingTags(strings.TrimSpace(stdout.String())),
		Provider: "claude-code",
		Model:    model,
	}, nil
}
