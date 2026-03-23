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

func (c *ClaudeCodeSummarizer) Summarize(text string, opts SummarizeOptions) (string, error) {
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
			return "", fmt.Errorf("claude-code: binary not found in PATH: %w", err)
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
	// --strict-mcp-config: disable all MCP servers (no --mcp-config provided)
	// Note: --bare is not used because it disables OAuth/keychain auth.
	args := []string{
		"--print",
		"--model", model,
		"--output-format", "text",
		"--strict-mcp-config",
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
		return "", fmt.Errorf("claude-code: execution failed: %w\noutput: %s", err, errMsg)
	}

	return StripThinkingTags(strings.TrimSpace(stdout.String())), nil
}
