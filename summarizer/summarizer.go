package summarizer

import (
	"fmt"
	"log/slog"
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

// NewSummarizer creates a Summarizer backend based on the provider config.
// When fallback providers are configured, it returns a FallbackSummarizer
// that tries providers in order with circuit breaker auto-recovery.
func NewSummarizer(cfg config.LLMConfig) (Summarizer, error) {
	primary, err := newSingleProvider(cfg.Provider.Primary(), cfg)
	if err != nil {
		return nil, err
	}

	fallbacks := cfg.Provider.Fallbacks()
	if len(fallbacks) == 0 {
		return primary, nil
	}

	// Build fallback chain with circuit breakers.
	strategy := cfg.ProviderFallbackStrategy
	cooldown := time.Duration(strategy.CooldownSeconds) * time.Second
	if cooldown == 0 {
		cooldown = 5 * time.Minute
	}
	threshold := strategy.FailureThreshold
	if threshold <= 0 {
		threshold = 1
	}

	entries := []providerEntry{{
		name:    cfg.Provider.Primary(),
		impl:    primary,
		breaker: newCircuitBreaker(cfg.Provider.Primary(), threshold, cooldown),
	}}

	for _, name := range fallbacks {
		fb, err := newSingleProvider(name, cfg)
		if err != nil {
			return nil, fmt.Errorf("fallback provider %q: %w", name, err)
		}
		entries = append(entries, providerEntry{
			name:    name,
			impl:    fb,
			breaker: newCircuitBreaker(name, threshold, cooldown),
		})
	}

	slog.Info("fallback summarizer initialized",
		"primary", cfg.Provider.Primary(),
		"fallbacks", fallbacks,
		"cooldown", cooldown,
	)

	return &FallbackSummarizer{providers: entries}, nil
}

// newSingleProvider creates a single Summarizer for the named provider.
func newSingleProvider(name string, cfg config.LLMConfig) (Summarizer, error) {
	switch name {
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
	case "claude-code":
		claudeCodeTimeout := time.Duration(cfg.ClaudeCode.Timeout) * time.Second
		if claudeCodeTimeout == 0 {
			claudeCodeTimeout = 15 * time.Minute
		}
		return &ClaudeCodeSummarizer{
			model:      cfg.ClaudeCode.Model,
			binaryPath: cfg.ClaudeCode.Path,
			timeout:    claudeCodeTimeout,
		}, nil
	case "gemini-cli":
		geminiTimeout := time.Duration(cfg.GeminiCLI.Timeout) * time.Second
		if geminiTimeout == 0 {
			geminiTimeout = 15 * time.Minute
		}
		return &GeminiCLISummarizer{
			model:      cfg.GeminiCLI.Model,
			binaryPath: cfg.GeminiCLI.Path,
			timeout:    geminiTimeout,
		}, nil
	case "qwen-code":
		qwenTimeout := time.Duration(cfg.QwenCode.Timeout) * time.Second
		if qwenTimeout == 0 {
			qwenTimeout = 15 * time.Minute
		}
		return &QwenCodeSummarizer{
			model:      cfg.QwenCode.Model,
			binaryPath: cfg.QwenCode.Path,
			timeout:    qwenTimeout,
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
		return nil, fmt.Errorf("unknown LLM provider: %q", name)
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
