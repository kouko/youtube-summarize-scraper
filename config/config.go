package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	OutputDir          string          `yaml:"output_dir"`
	PreferredLanguages []string        `yaml:"preferred_languages"`
	DefaultCount       int             `yaml:"default_count"`
	Whisper            WhisperConfig   `yaml:"whisper"`
	Cookie             CookieConfig    `yaml:"cookie"`
	LLM                LLMConfig       `yaml:"llm"`
	Summary            SummaryConfig   `yaml:"summary"`
	Filter             FilterConfig    `yaml:"filter"`
	Batch              BatchConfig     `yaml:"batch"`
	Obsidian           ObsidianConfig  `yaml:"obsidian"`
	Playlists          []PlaylistConfig `yaml:"playlists"`
	Channels           []ChannelConfig  `yaml:"channels"`
}

type BatchConfig struct {
	RandomOrder bool `yaml:"random_order"` // Shuffle channel processing order
	DelayMin    int  `yaml:"delay_min"`    // Min seconds delay between channels
	DelayMax    int  `yaml:"delay_max"`    // Max seconds delay between channels
}

type WhisperConfig struct {
	ModelDir       string            `yaml:"model_dir"`
	DefaultModel   string            `yaml:"default_model"`
	LanguageModels map[string]string `yaml:"language_models"`
	ModelSources   map[string]string `yaml:"model_sources"`
}

type CookieConfig struct {
	File          string `yaml:"file"`
	Browser       string `yaml:"browser"`
	ChromeProfile string `yaml:"chrome_profile"`
}

type LLMConfig struct {
	Provider  string          `yaml:"provider"`
	Ollama    OllamaConfig    `yaml:"ollama"`
	LlamaCpp  LlamaCppConfig  `yaml:"llamacpp"`
	ClaudeAPI ClaudeAPIConfig `yaml:"claude_api"`
	GeminiCLI GeminiCLIConfig `yaml:"gemini_cli"`
}

type OllamaConfig struct {
	Model    string `yaml:"model"`
	Endpoint string `yaml:"endpoint"`
	Think    *bool  `yaml:"think,omitempty"`
	Timeout  int    `yaml:"timeout"` // Seconds per LLM request (default: 900)
}

type LlamaCppConfig struct {
	Endpoint string `yaml:"endpoint"`
}

type ClaudeAPIConfig struct {
	APIKey string `yaml:"api_key"`
	Model  string `yaml:"model"`
}

type GeminiCLIConfig struct {
	Model string `yaml:"model"`
	Path  string `yaml:"path"`
}

type SummaryConfig struct {
	Language        string         `yaml:"language"`
	Prompt          string         `yaml:"prompt"`
	SummaryPromptFile string       `yaml:"summary_prompt_file"`
	MaxTokens       int            `yaml:"max_tokens"`
	Keywords        KeywordsConfig `yaml:"keywords"`
	Mermaid         MermaidConfig  `yaml:"mermaid"`
}

type KeywordsConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Language string `yaml:"language"`
	Count    int    `yaml:"count"`
}

type MermaidConfig struct {
	Enabled bool `yaml:"enabled"`
}

type FilterConfig struct {
	Types       []string `yaml:"types"`
	MinDuration int      `yaml:"min_duration"`
	MaxDuration int      `yaml:"max_duration"`
}

type ObsidianConfig struct {
	Enabled     bool     `yaml:"enabled"`
	AutoTags    []string `yaml:"auto_tags"`
	GenerateMOC bool     `yaml:"generate_moc"`
	Wikilinks   bool     `yaml:"wikilinks"`
}

type CopyToConfig struct {
	Path      string   `yaml:"path"`
	Files     []string `yaml:"files"`     // "summary", "transcription", "subtitle"
	Filename  string   `yaml:"filename"`  // template with variables
	Overwrite bool     `yaml:"overwrite"`
}

type ChannelConfig struct {
	URL              string        `yaml:"url"`
	Count            int           `yaml:"count"`
	SummaryPromptFile string       `yaml:"summary_prompt_file"`
	Filter           *FilterConfig `yaml:"filter"`
	Cookie           *CookieConfig `yaml:"cookie"`
	CopyTo           *CopyToConfig `yaml:"copy_to"`
}

type PlaylistConfig struct {
	URL              string        `yaml:"url"`
	Name             string        `yaml:"name"`
	Count            int           `yaml:"count"`
	SummaryPromptFile string       `yaml:"summary_prompt_file"`
	Cookie           *CookieConfig `yaml:"cookie"`
	CopyTo           *CopyToConfig `yaml:"copy_to"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	cfg.expandEnvVars()
	return cfg, nil
}

func DefaultConfig() *Config {
	return &Config{
		OutputDir:    "./ytss-output",
		DefaultCount: 5,
		Whisper: WhisperConfig{
			ModelDir:     "~/.ytss/models",
			DefaultModel: "medium",
			LanguageModels: map[string]string{
				"ja": "kotoba-ja",
				"zh": "belle-zh",
				"en": "medium",
			},
			ModelSources: map[string]string{
				"tiny":            "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-tiny.bin",
				"base":            "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.bin",
				"small":           "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small.bin",
				"medium":          "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-medium.bin",
				"large-v3":        "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-large-v3.bin",
				"large-v3-turbo":  "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-large-v3-turbo.bin",
				"belle-zh":        "https://huggingface.co/BELLE-2/Belle-whisper-large-v3-turbo-zh-ggml/resolve/main/ggml-model.bin",
				"kotoba-ja":       "https://huggingface.co/kotoba-tech/kotoba-whisper-v2.0-ggml/resolve/main/ggml-model.bin",
				"kotoba-ja-q5":    "https://huggingface.co/kotoba-tech/kotoba-whisper-v2.0-ggml/resolve/main/ggml-model-q5.bin",
			},
		},
		LLM: LLMConfig{
			Provider: "ollama",
			Ollama: OllamaConfig{
				Model:    "llama3",
				Endpoint: "http://localhost:11434",
				Think:    ptrBool(false),
				Timeout:  900,
			},
			LlamaCpp: LlamaCppConfig{
				Endpoint: "http://localhost:8080",
			},
			ClaudeAPI: ClaudeAPIConfig{
				Model: "claude-sonnet-4-20250514",
			},
			GeminiCLI: GeminiCLIConfig{
				Model: "gemini-2.5-pro",
			},
		},
		Summary: SummaryConfig{
			Language:  "en",
			MaxTokens: 2000,
			Keywords: KeywordsConfig{
				Enabled:  true,
				Language: "en",
				Count:    10,
			},
			Mermaid: MermaidConfig{
				Enabled: true,
			},
		},
		Filter: FilterConfig{
			Types: []string{"video", "live", "short"},
		},
		Batch: BatchConfig{
			RandomOrder: true,
		},
	}
}

func (c *Config) expandEnvVars() {
	c.LLM.ClaudeAPI.APIKey = os.ExpandEnv(c.LLM.ClaudeAPI.APIKey)
}

func (c *Config) EffectiveCount(ch ChannelConfig) int {
	if ch.Count > 0 {
		return ch.Count
	}
	return c.DefaultCount
}

func (c *Config) EffectiveFilter(ch ChannelConfig) FilterConfig {
	if ch.Filter != nil {
		return *ch.Filter
	}
	return c.Filter
}

func ptrBool(v bool) *bool { return &v }
