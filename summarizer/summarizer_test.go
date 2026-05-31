package summarizer

import (
	"testing"

	"github.com/kouko/youtube-summarize-scraper/config"
)

func TestNewSingleProvider_AllProviders(t *testing.T) {
	cfg := config.LLMConfig{
		Ollama: config.OllamaConfig{
			Model:    "llama3",
			Endpoint: "http://localhost:11434",
			Timeout:  900,
		},
		LlamaCpp: config.LlamaCppConfig{
			Endpoint: "http://localhost:8080",
		},
		ClaudeAPI: config.ClaudeAPIConfig{
			APIKey: "test-key",
			Model:  "claude-sonnet-4-20250514",
		},
		ClaudeCode: config.ClaudeCodeConfig{
			Model:   "haiku",
			Timeout: 900,
		},
		GeminiCLI: config.GeminiCLIConfig{
			Model:   "auto",
			Timeout: 900,
		},
		QwenCode: config.QwenCodeConfig{
			Model:   "coder-model",
			Timeout: 900,
		},
		OpenAICompat: config.OpenAICompatConfig{
			Endpoint: "http://localhost:8000/v1",
			Model:    "test-model",
			Timeout:  900,
		},
	}

	providers := []string{"ollama", "llamacpp", "claude-api", "claude-code", "gemini-cli", "qwen-code", "openai-compat"}
	for _, name := range providers {
		s, err := newSingleProvider(name, cfg)
		if err != nil {
			t.Errorf("newSingleProvider(%q): unexpected error: %v", name, err)
		}
		if s == nil {
			t.Errorf("newSingleProvider(%q): returned nil", name)
		}
	}
}

func TestNewSingleProvider_Antigravity(t *testing.T) {
	cfg := config.LLMConfig{
		Antigravity: config.AntigravityCLIConfig{Timeout: 900},
	}
	s, err := newSingleProvider("antigravity-cli", cfg)
	if err != nil {
		t.Fatalf("newSingleProvider(antigravity-cli): %v", err)
	}
	if _, ok := s.(*AntigravityCLISummarizer); !ok {
		t.Fatalf("expected *AntigravityCLISummarizer, got %T", s)
	}
}

func TestNewSingleProvider_Unknown(t *testing.T) {
	_, err := newSingleProvider("nonexistent", config.LLMConfig{})
	if err == nil {
		t.Error("expected error for unknown provider")
	}
}

func TestNewSingleProvider_DefaultTimeouts(t *testing.T) {
	// Timeout = 0 should default to 15 min internally.
	cfg := config.LLMConfig{
		Ollama:       config.OllamaConfig{Model: "m", Endpoint: "http://x"},
		ClaudeCode:   config.ClaudeCodeConfig{Model: "m"},
		GeminiCLI:    config.GeminiCLIConfig{Model: "m"},
		QwenCode:     config.QwenCodeConfig{Model: "m"},
		OpenAICompat: config.OpenAICompatConfig{Endpoint: "http://x", Model: "m"},
	}
	for _, name := range []string{"ollama", "claude-code", "gemini-cli", "qwen-code", "openai-compat"} {
		s, err := newSingleProvider(name, cfg)
		if err != nil {
			t.Fatalf("newSingleProvider(%q): %v", name, err)
		}
		if s == nil {
			t.Fatalf("newSingleProvider(%q): nil", name)
		}
	}
}

func TestNewSummarizer_SingleProvider(t *testing.T) {
	cfg := config.LLMConfig{
		Provider: config.ProviderList{"ollama"},
		Ollama: config.OllamaConfig{
			Model:    "llama3",
			Endpoint: "http://localhost:11434",
			Timeout:  60,
		},
	}
	s, err := NewSummarizer(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Single provider should NOT be wrapped in FallbackSummarizer.
	if _, ok := s.(*FallbackSummarizer); ok {
		t.Error("single provider should not be FallbackSummarizer")
	}
}

func TestNewSummarizer_WithFallbacks(t *testing.T) {
	cfg := config.LLMConfig{
		Provider: config.ProviderList{"gemini-cli", "claude-code", "ollama"},
		ProviderFallbackStrategy: config.FallbackStrategyConfig{
			CooldownSeconds:  60,
			FailureThreshold: 2,
		},
		GeminiCLI:  config.GeminiCLIConfig{Model: "auto", Timeout: 60},
		ClaudeCode: config.ClaudeCodeConfig{Model: "haiku", Timeout: 60},
		Ollama: config.OllamaConfig{
			Model:    "llama3",
			Endpoint: "http://localhost:11434",
			Timeout:  60,
		},
	}
	s, err := NewSummarizer(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fb, ok := s.(*FallbackSummarizer)
	if !ok {
		t.Fatal("expected FallbackSummarizer")
	}
	if len(fb.providers) != 3 {
		t.Errorf("providers: got %d, want 3", len(fb.providers))
	}
	if fb.providers[0].name != "gemini-cli" {
		t.Errorf("primary: got %q, want %q", fb.providers[0].name, "gemini-cli")
	}
	if fb.providers[1].name != "claude-code" {
		t.Errorf("fallback 1: got %q, want %q", fb.providers[1].name, "claude-code")
	}
	if fb.providers[2].name != "ollama" {
		t.Errorf("fallback 2: got %q, want %q", fb.providers[2].name, "ollama")
	}
}

func TestNewSummarizer_FallbackDefaultStrategy(t *testing.T) {
	// CooldownSeconds=0 and FailureThreshold=0 should use defaults.
	cfg := config.LLMConfig{
		Provider:                 config.ProviderList{"ollama", "llamacpp"},
		ProviderFallbackStrategy: config.FallbackStrategyConfig{},
		Ollama: config.OllamaConfig{
			Model:    "llama3",
			Endpoint: "http://localhost:11434",
			Timeout:  60,
		},
		LlamaCpp: config.LlamaCppConfig{Endpoint: "http://localhost:8080"},
	}
	s, err := NewSummarizer(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := s.(*FallbackSummarizer); !ok {
		t.Error("expected FallbackSummarizer")
	}
}

func TestNewSummarizer_UnknownPrimary(t *testing.T) {
	cfg := config.LLMConfig{
		Provider: config.ProviderList{"nonexistent"},
	}
	_, err := NewSummarizer(cfg)
	if err == nil {
		t.Error("expected error for unknown primary provider")
	}
}

func TestNewSummarizer_UnknownFallback(t *testing.T) {
	cfg := config.LLMConfig{
		Provider: config.ProviderList{"ollama", "nonexistent"},
		Ollama: config.OllamaConfig{
			Model:    "llama3",
			Endpoint: "http://localhost:11434",
			Timeout:  60,
		},
	}
	_, err := NewSummarizer(cfg)
	if err == nil {
		t.Error("expected error for unknown fallback provider")
	}
}

func TestResolvePrompt(t *testing.T) {
	// opts.Prompt takes priority.
	got := resolvePrompt("fallback text", SummarizeOptions{Prompt: "explicit prompt"})
	if got != "explicit prompt" {
		t.Errorf("got %q, want %q", got, "explicit prompt")
	}

	// Falls back to text when Prompt is empty.
	got = resolvePrompt("fallback text", SummarizeOptions{})
	if got != "fallback text" {
		t.Errorf("got %q, want %q", got, "fallback text")
	}
}

func TestStripThinkingTags(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"no tags here", "no tags here"},
		{"before <think>internal</think> after", "before  after"},
		{"<thinking>deep thought</thinking> result", "result"},
		{"<reflection>hmm</reflection> ok", "ok"},
		{"multi\n<think>line\nthought</think>\nresult", "multi\n\nresult"},
		{"", ""},
	}
	for _, tt := range tests {
		got := StripThinkingTags(tt.input)
		if got != tt.want {
			t.Errorf("StripThinkingTags(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestStripAgentArtifacts(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "fake function_calls block",
			input: "前言\n<function_calls>\n<invoke_tool_name>Read</invoke_tool_name>\n<parameter name=\"path\">/some/path</parameter>\n</function_calls>\n\n### 概述\n摘要內容",
			want:  "### 概述\n摘要內容",
		},
		{
			name:  "preamble before heading",
			input: "根據影片字幕內容，我為您生成完整的結構化摘要：\n\n### 概述\n摘要內容",
			want:  "### 概述\n摘要內容",
		},
		{
			name:  "no preamble - starts with heading",
			input: "### 概述\n摘要內容",
			want:  "### 概述\n摘要內容",
		},
		{
			name:  "no heading - keywords output preserved",
			input: "人工智慧\n機器學習\n深度學習",
			want:  "人工智慧\n機器學習\n深度學習",
		},
		{
			name:  "mermaid with preamble",
			input: "我來幫您生成流程圖：\n\n#### 章節標題\n```mermaid\ngraph LR\nA-->B\n```",
			want:  "#### 章節標題\n```mermaid\ngraph LR\nA-->B\n```",
		},
		{
			name:  "combined thinking + agent artifacts + preamble",
			input: "<think>planning</think>\n我來查看記憶。\n<function_calls>\n<invoke_tool_name>Glob</invoke_tool_name>\n</function_calls>\n根據內容：\n\n### 概述\n結果",
			want:  "### 概述\n結果",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripThinkingTags(tt.input)
			if got != tt.want {
				t.Errorf("StripThinkingTags() =\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}
