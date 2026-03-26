package summarizer

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// GeminiCLISummarizer invokes the Gemini CLI tool in headless mode.
// Prompt is passed via stdin to avoid ARG_MAX limits on long transcriptions.
type GeminiCLISummarizer struct {
	model      string
	binaryPath string
	timeout    time.Duration
}

func (g *GeminiCLISummarizer) Summarize(text string, opts SummarizeOptions) (string, error) {
	model := g.model
	if opts.Model != "" {
		model = opts.Model
	}

	combinedPrompt := resolvePrompt(text, opts)

	binary := g.binaryPath
	if binary == "" {
		var err error
		binary, err = exec.LookPath("gemini")
		if err != nil {
			return "", fmt.Errorf("gemini-cli: binary not found in PATH: %w", err)
		}
	}

	timeout := g.timeout
	if timeout == 0 {
		timeout = 15 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Use stdin pipe for prompt content to avoid OS ARG_MAX limits.
	// gemini reads from stdin when no -p flag is provided in pipe mode.
	// -o text: plain text output format
	// --approval-mode plan: read-only mode, prevents tool write operations
	//   (gemini-cli has no --tools "" equivalent to disable tools entirely)
	// --allowed-mcp-server-names __none__: disable MCP servers
	args := []string{"-m", model, "-o", "text", "--approval-mode", "plan", "--allowed-mcp-server-names", "__none__"}
	cmd := exec.CommandContext(ctx, binary, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdin = strings.NewReader(combinedPrompt)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		combined := stderr.String() + "\n" + stdout.String()
		baseErr := fmt.Errorf("gemini-cli: execution failed: %w\nstderr: %s", err, stderr.String())
		if isQuotaMessage(combined) {
			return "", &QuotaError{Provider: "gemini-cli", Err: baseErr}
		}
		return "", baseErr
	}

	return StripThinkingTags(strings.TrimSpace(stdout.String())), nil
}
