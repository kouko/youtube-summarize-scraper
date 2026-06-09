package summarizer

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// QwenCodeSummarizer invokes the Qwen Code CLI in headless mode.
// Qwen Code is a Gemini CLI fork; prompt is passed via stdin.
type QwenCodeSummarizer struct {
	model      string
	binaryPath string
	timeout    time.Duration
}

func (q *QwenCodeSummarizer) Summarize(text string, opts SummarizeOptions) (SummarizeResult, error) {
	model := q.model
	if opts.Model != "" {
		model = opts.Model
	}

	combinedPrompt := resolvePrompt(text, opts)

	binary := q.binaryPath
	if binary == "" {
		var err error
		binary, err = exec.LookPath("qwen")
		if err != nil {
			return SummarizeResult{}, fmt.Errorf("qwen-code: binary not found in PATH: %w", err)
		}
	}

	timeout := q.timeout
	if timeout == 0 {
		timeout = 15 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// qwen reads from stdin when no positional prompt is provided.
	// -o text: plain text output format
	// --approval-mode default: normal mode (NOT "plan" which means "plan only"
	//   in Qwen Code and causes the model to discuss instead of execute)
	// --exclude-tools: disable all write-capable tools for safe text generation
	// --allowed-mcp-server-names __none__: disable MCP servers
	args := []string{
		"-m", model, "-o", "text",
		"--approval-mode", "default",
		"--exclude-tools", "write_file,edit,run_shell_command,save_memory,agent,skill,todo_write,exit_plan_mode",
		"--allowed-mcp-server-names", "__none__",
	}
	cmd := exec.CommandContext(ctx, binary, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdin = strings.NewReader(combinedPrompt)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		combined := stderr.String() + "\n" + stdout.String()
		baseErr := fmt.Errorf("qwen-code: execution failed: %w\nstderr: %s", err, stderr.String())
		if qe := quotaErrorFrom("qwen-code", baseErr, 0, combined, "", time.Now()); qe != nil {
			return SummarizeResult{}, qe
		}
		return SummarizeResult{}, baseErr
	}

	return SummarizeResult{
		Text:     StripThinkingTags(strings.TrimSpace(stdout.String())),
		Provider: "qwen-code",
		Model:    model,
	}, nil
}
