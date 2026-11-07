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
	// AllowEmpty marks a request where an empty response can be legitimate
	// (e.g. keywords/mermaid stages — not every summary needs a diagram). The
	// fallback chain still fails over on empty, but does NOT count it toward
	// opening the provider's circuit, so a minor-stage empty can't sideline a
	// provider whose main-summary stage is healthy. Defaults to false (empty is
	// a failure — correct for the main summary).
	AllowEmpty bool
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
	rateLimitCooldown := time.Duration(strategy.RateLimitCooldownSeconds) * time.Second
	if rateLimitCooldown == 0 {
		rateLimitCooldown = 60 * time.Second
	}
	emptyThreshold := strategy.EmptyResponseThreshold
	if emptyThreshold == 0 {
		emptyThreshold = defaultEmptyThreshold
	}

	makeBreaker := func(name string) *CircuitBreaker {
		cb := newCircuitBreaker(name, threshold, cooldown)
		cb.rateLimitCooldown = rateLimitCooldown
		cb.emptyThreshold = emptyThreshold
		return cb
	}

	entries := []providerEntry{{
		name:    cfg.Provider.Primary(),
		impl:    primary,
		breaker: makeBreaker(cfg.Provider.Primary()),
	}}

	for _, name := range fallbacks {
		fb, err := newSingleProvider(name, cfg)
		if err != nil {
			return nil, fmt.Errorf("fallback provider %q: %w", name, err)
		}
		entries = append(entries, providerEntry{
			name:    name,
			impl:    fb,
			breaker: makeBreaker(name),
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
	// openai-compat family: bare "openai-compat" targets instance "default";
	// "openai-compat:<instance>" targets a named instance in cfg.OpenAICompat.
	if name == "openai-compat" || strings.HasPrefix(name, "openai-compat:") {
		instance := "default"
		if idx := strings.IndexByte(name, ':'); idx >= 0 {
			instance = name[idx+1:]
		}
		oc, ok := cfg.OpenAICompat[instance]
		if !ok {
			return nil, fmt.Errorf("openai-compat: no instance %q configured", instance)
		}
		timeout := time.Duration(oc.Timeout) * time.Second
		if timeout == 0 {
			timeout = 15 * time.Minute
		}
		return &OpenAICompatSummarizer{
			endpoint: oc.Endpoint,
			model:    oc.Model,
			apiKey:   oc.APIKey,
			timeout:  timeout,
		}, nil
	}

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

// mermaidLimit returns the total diagram cap and additional range for a transcription length tier.
func mermaidLimit(transcriptionLength int) (total int, additional string) {
	switch {
	case transcriptionLength < 500:
		return 2, "0-1"
	case transcriptionLength < 3000:
		return 3, "1-2"
	case transcriptionLength < 10000:
		return 4, "2-3"
	default:
		return 5, "3-4"
	}
}

// mermaidLimitLang normalizes a language code into zh / ja / en.
func mermaidLimitLang(language string) string {
	switch {
	case strings.HasPrefix(language, "zh"):
		return "zh"
	case strings.HasPrefix(language, "ja"):
		return "ja"
	default:
		return "en"
	}
}

// MermaidLimitText returns the dynamic diagram count limit text based on
// transcription length and prompt language. It is prepended to the mermaid
// prompt at Stage 3 to enforce the count rules (total cap, one mandatory
// diagram for the leading Key Viewpoints section without a duplicated
// heading, and selective choice among chapter sections).
func MermaidLimitText(transcriptionLength int, language string) string {
	total, additional := mermaidLimit(transcriptionLength)
	lang := mermaidLimitLang(language)

	switch lang {
	case "zh":
		return fmt.Sprintf(`
【圖表數量限制（最高優先，必須遵守）】
本次最多輸出 %d 張圖表（總數上限，含主要觀點圖）。主要觀點 1 張（必要），其餘章節額外 %s 張。
- 第 1 張固定對應摘要最前面的「## 主要觀點」，作為全片架構總覽；此圖**不要**加 #### 標題，避免與「主要觀點」標題重複。
- 其餘圖表只從「###」章節中挑選，每章最多 1 張，依適合視覺化的程度擇優；不適合的章節一律跳過。
- 寧缺勿濫：圖表總數不得超過上限，絕對不要為每個章節都畫一張。

`, total, additional)
	case "ja":
		return fmt.Sprintf(`
【図表数の上限（最優先・必ず遵守）】
今回出力できる図表は合計 %d 枚まで（主要観点の図を含む）。主要観点 1 枚（必須）、その他の章節は追加 %s 枚。
- 1枚目は要約冒頭の「## 主要觀點」に対応させ、全体のアーキテクチャ概観とする。この図には #### 見出しを付けないこと（見出しの重複を避けるため）。
- 残りの図表は「###」セクションの中からのみ選び、各セクション最大 1 枚、視覚化に適したものを優先的に選ぶ。適さないセクションはスキップすること。
- 少なめを優先：図表の総数は上限を超えないこと。各章に1枚ずつ作成しないこと。

`, total, additional)
	default:
		return fmt.Sprintf(`
[Diagram count limit — highest priority, must comply]
Output at most %d diagrams in total (cap, including the Key Viewpoints diagram). Key Viewpoints 1 (mandatory), chapters add %s more.
- The first diagram must correspond to the "## Key Viewpoints" section at the top of the summary as the overall architecture overview; do NOT add a #### heading to it (it would duplicate the Key Viewpoints heading).
- The remaining diagrams must be selected only from "###" chapter sections, at most one per section, prioritizing sections best suited to visualization; skip sections that do not fit.
- Prefer fewer: the total must not exceed the cap; never produce one diagram per chapter.

`, total, additional)
	}
}
