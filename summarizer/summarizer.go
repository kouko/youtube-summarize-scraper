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

// SummarizeResult holds the output of a summarization request along with
// metadata about which provider and model actually handled it.
type SummarizeResult struct {
	Text     string // the generated summary text
	Provider string // actual provider that generated the response (e.g., "qwen-code")
	Model    string // actual model used (e.g., "coder-model")
}

// Summarizer is the interface that all LLM backends must implement.
type Summarizer interface {
	Summarize(text string, opts SummarizeOptions) (SummarizeResult, error)
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
	case "antigravity-cli":
		antigravityTimeout := time.Duration(cfg.Antigravity.Timeout) * time.Second
		if antigravityTimeout == 0 {
			antigravityTimeout = 15 * time.Minute
		}
		return &AntigravityCLISummarizer{
			binaryPath: cfg.Antigravity.Path,
			timeout:    antigravityTimeout,
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

// agentArtifactRe matches fake tool-call XML blocks that CLI-based models
// (e.g., Gemini CLI, Qwen Code) sometimes emit in agent mode.
// Covers: <function_calls>, <invoke_tool_name>, <invoke>, <parameter>, <*> and variants.
var agentArtifactRe = regexp.MustCompile(`(?s)<(?:function_calls|invoke_tool_name|invoke|parameter|antml:\w+)[^>]*>.*?</(?:function_calls|invoke_tool_name|invoke|parameter|antml:\w+)>`)

// StripThinkingTags removes <think>...</think> blocks and agent artifacts from LLM responses.
// Some models (e.g., Qwen3.5) output thinking traces wrapped in these tags.
// CLI-based models in agent mode may also emit fake tool-call XML blocks.
func StripThinkingTags(response string) string {
	result := thinkingTagRe.ReplaceAllString(response, "")
	result = agentArtifactRe.ReplaceAllString(result, "")
	result = stripPreambleBeforeContent(result)
	return strings.TrimSpace(result)
}

// stripPreambleBeforeContent removes conversational preamble lines that appear
// before the actual structured content. CLI-based models sometimes prepend lines
// like "根據影片字幕內容，我為您生成完整的結構化摘要：" before the real output.
// This function detects the first markdown heading (##/###/####) or mermaid code
// fence and strips everything before it.
func stripPreambleBeforeContent(response string) string {
	trimmed := strings.TrimSpace(response)
	if trimmed == "" {
		return response
	}

	// If response already starts with markdown heading or code fence, no preamble.
	if trimmed[0] == '#' || strings.HasPrefix(trimmed, "```") {
		return response
	}

	// Find the first markdown heading (## or deeper).
	headingIdx := -1
	for _, prefix := range []string{"\n## ", "\n### ", "\n#### "} {
		if idx := strings.Index(response, prefix); idx >= 0 {
			if headingIdx < 0 || idx < headingIdx {
				headingIdx = idx
			}
		}
	}

	if headingIdx >= 0 {
		return response[headingIdx+1:] // +1 to skip the leading \n
	}

	return response
}
