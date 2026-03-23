package summarizer

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/kouko/youtube-summarize-scraper/config"
)

// SummarizeOptions holds options for a summarization request.
type SummarizeOptions struct {
	Prompt    string
	MaxTokens int
	Model     string
}

// Summarizer is the interface that all LLM backends must implement.
type Summarizer interface {
	Summarize(text string, opts SummarizeOptions) (string, error)
}

// NewSummarizer creates a Summarizer backend based on the provider in cfg.
func NewSummarizer(cfg config.LLMConfig) (Summarizer, error) {
	switch cfg.Provider {
	case "ollama":
		timeout := time.Duration(cfg.Ollama.Timeout) * time.Second
		if timeout == 0 {
			timeout = 15 * time.Minute
		}
		return &OllamaSummarizer{
			endpoint: cfg.Ollama.Endpoint,
			model:    cfg.Ollama.Model,
			think:    cfg.Ollama.Think,
			timeout:  timeout,
		}, nil
	case "llamacpp":
		return &LlamaCppSummarizer{
			endpoint: cfg.LlamaCpp.Endpoint,
		}, nil
	case "claude-api":
		return &ClaudeSummarizer{
			apiKey: cfg.ClaudeAPI.APIKey,
			model:  cfg.ClaudeAPI.Model,
		}, nil
	case "gemini-cli":
		return &GeminiCLISummarizer{
			model:      cfg.GeminiCLI.Model,
			binaryPath: cfg.GeminiCLI.Path,
		}, nil
	case "openai-compat":
		timeout := time.Duration(cfg.OpenAICompat.Timeout) * time.Second
		if timeout == 0 {
			timeout = 15 * time.Minute
		}
		return &OpenAICompatSummarizer{
			endpoint: cfg.OpenAICompat.Endpoint,
			model:    cfg.OpenAICompat.Model,
			apiKey:   cfg.OpenAICompat.APIKey,
			timeout:  timeout,
		}, nil
	default:
		return nil, fmt.Errorf("unknown LLM provider: %q", cfg.Provider)
	}
}

// resolvePrompt returns opts.Prompt if non-empty, otherwise falls back to text.
func resolvePrompt(text string, opts SummarizeOptions) string {
	if opts.Prompt != "" {
		return opts.Prompt
	}
	return text
}

// thinkingTagRe matches thinking-related XML blocks (including multiline).
// Covers: <think>, <thinking>, <reflection> and their closing tags.
var thinkingTagRe = regexp.MustCompile(`(?s)<(?:think|thinking|reflection)>.*?</(?:think|thinking|reflection)>`)

// StripThinkingTags removes <think>...</think> blocks from LLM responses.
// Some models (e.g., Qwen3.5) output thinking traces wrapped in these tags.
func StripThinkingTags(response string) string {
	result := thinkingTagRe.ReplaceAllString(response, "")
	return strings.TrimSpace(result)
}
