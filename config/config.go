package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// ProviderList holds one or more LLM provider names.
// The first entry is the primary provider; subsequent entries are fallbacks
// tried in order when the primary is unavailable (e.g., quota exhausted).
//
// YAML accepts both a scalar string and a list:
//
//	provider: "gemini-cli"          # single provider
//	provider:                       # provider chain
//	  - "gemini-cli"
//	  - "claude-code"
type ProviderList []string

func (p *ProviderList) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		*p = ProviderList{value.Value}
		return nil
	}
	var list []string
	if err := value.Decode(&list); err != nil {
		return err
	}
	*p = list
	return nil
}

// Primary returns the first (highest priority) provider name.
func (p ProviderList) Primary() string {
	if len(p) == 0 {
		return ""
	}
	return p[0]
}

// Fallbacks returns all providers after the primary.
func (p ProviderList) Fallbacks() []string {
	if len(p) <= 1 {
		return nil
	}
	return p[1:]
}

// String returns the primary provider name for display/frontmatter.
func (p ProviderList) String() string {
	return p.Primary()
}

// FallbackStrategyConfig controls how the provider fallback chain behaves.
type FallbackStrategyConfig struct {
	CooldownSeconds  int `yaml:"cooldown_seconds"`  // Seconds before retrying a failed provider (default: 300)
	FailureThreshold int `yaml:"failure_threshold"`  // Quota errors before skipping a provider (default: 1)
}

// SetPrimary replaces the primary provider while keeping fallbacks.
func (p *ProviderList) SetPrimary(name string) {
	if len(*p) == 0 {
		*p = ProviderList{name}
		return
	}
	// Rebuild list: new primary + all others (deduplicating name).
	result := ProviderList{name}
	for _, existing := range *p {
		if existing != name {
			result = append(result, existing)
		}
	}
	*p = result
}

// Equal compares two ProviderList values for testing.
func (p ProviderList) Equal(other ProviderList) bool {
	if len(p) != len(other) {
		return false
	}
	for i := range p {
		if p[i] != other[i] {
			return false
		}
	}
	return true
}

// MarshalYAML serializes ProviderList back to YAML.
// A single-element list is written as a scalar string for backward compatibility.
func (p ProviderList) MarshalYAML() (interface{}, error) {
	if len(p) == 1 {
		return p[0], nil
	}
	return []string(p), nil
}

// Contains reports whether the list includes the named provider.
func (p ProviderList) Contains(name string) bool {
	for _, v := range p {
		if strings.EqualFold(v, name) {
			return true
		}
	}
	return false
}

type Config struct {
	OutputDir          string          `yaml:"output_dir"`
	PreferredLanguages []string        `yaml:"preferred_languages"`
	DefaultCount       int             `yaml:"default_count"`
	Timezone           string          `yaml:"timezone"`
	Whisper            WhisperConfig   `yaml:"whisper"`
	Cookie             CookieConfig    `yaml:"cookie"`
	LLM                LLMConfig       `yaml:"llm"`
	Summary            SummaryConfig   `yaml:"summary"`
	Filter             FilterConfig    `yaml:"filter"`
	Batch              BatchConfig     `yaml:"batch"`
	VideoEmbed         bool            `yaml:"video_embed"`
	Obsidian           ObsidianConfig  `yaml:"obsidian"`
	Playlists          []PlaylistConfig `yaml:"playlists"`
	Channels           []ChannelConfig  `yaml:"channels"`
}

type BatchConfig struct {
	RandomOrder      bool `yaml:"random_order"`      // Shuffle channel processing order
	DelayMin         int  `yaml:"delay_min"`          // Min seconds delay between channels
	DelayMax         int  `yaml:"delay_max"`          // Max seconds delay between channels
	Watch            bool `yaml:"watch"`              // Enable watch mode (loop)
	WatchInterval    int  `yaml:"watch_interval"`     // Minutes between iterations (default: 10)
	FetchConcurrency int  `yaml:"fetch_concurrency"` // Max parallel yt-dlp list fetches (default: 3)
}

type WhisperConfig struct {
	ModelDir           string            `yaml:"model_dir"`
	DefaultModel       string            `yaml:"default_model"`
	LanguageModels     map[string]string `yaml:"language_models"`
	ModelSources       map[string]string `yaml:"model_sources"`
	TranscribeTimeout  int              `yaml:"transcribe_timeout"`
	DownloadTimeout    int              `yaml:"download_timeout"`
}

type CookieConfig struct {
	File          string `yaml:"file"`
	Browser       string `yaml:"browser"`
	ChromeProfile string `yaml:"chrome_profile"`
}

type LLMConfig struct {
	Provider                 ProviderList           `yaml:"provider"`
	ProviderFallbackStrategy FallbackStrategyConfig `yaml:"provider_fallback_strategy"`
	Ollama                   OllamaConfig           `yaml:"ollama"`
	LlamaCpp                 LlamaCppConfig         `yaml:"llamacpp"`
	ClaudeAPI                ClaudeAPIConfig        `yaml:"claude-api"`
	ClaudeCode               ClaudeCodeConfig       `yaml:"claude-code"`
	GeminiCLI                GeminiCLIConfig        `yaml:"gemini-cli"`
	Antigravity              AntigravityCLIConfig   `yaml:"antigravity-cli"`
	QwenCode                 QwenCodeConfig         `yaml:"qwen-code"`
	OpenAICompat             OpenAICompatConfig     `yaml:"openai-compat"`
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

type ClaudeCodeConfig struct {
	Model   string `yaml:"model"`   // e.g. "sonnet", "opus", "claude-sonnet-4-6"
	Path    string `yaml:"path"`    // Path to claude binary (default: search in PATH)
	Timeout int    `yaml:"timeout"` // Seconds per LLM request (default: 900)
}

type GeminiCLIConfig struct {
	Model   string `yaml:"model"`
	Path    string `yaml:"path"`
	Timeout int    `yaml:"timeout"` // Seconds per LLM request (default: 900)
}

// AntigravityCLIConfig configures the Google Antigravity CLI (`agy`) backend.
// agy print mode has no per-call model flag (the model is chosen interactively
// via /model and persists across sessions), so there is intentionally no Model
// field here.
type AntigravityCLIConfig struct {
	Path    string `yaml:"path"`    // Path to agy binary (default: search in PATH)
	Timeout int    `yaml:"timeout"` // Seconds per LLM request (default: 900)
}

type QwenCodeConfig struct {
	Model   string `yaml:"model"`   // e.g. "coder-model" (free tier), "qwen3-coder-plus" (paid)
	Path    string `yaml:"path"`    // Path to qwen binary (default: search in PATH)
	Timeout int    `yaml:"timeout"` // Seconds per LLM request (default: 900)
}

type OpenAICompatConfig struct {
	Endpoint string `yaml:"endpoint"` // e.g. "http://localhost:8000/v1"
	Model    string `yaml:"model"`
	APIKey   string `yaml:"api_key"` // optional
	Timeout  int    `yaml:"timeout"` // Seconds per LLM request (default: 900)
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
	Types               []string `yaml:"types"`
	MinDuration         int      `yaml:"min_duration"`
	MaxDuration         int      `yaml:"max_duration"`
	ExcludeAvailability []string `yaml:"exclude_availability"`
}

// FilterOverride allows per-channel/playlist partial filter overrides.
// nil fields mean "inherit from global"; non-nil fields (even zero/empty) mean "override".
type FilterOverride struct {
	Types               *[]string `yaml:"types"`
	MinDuration         *int      `yaml:"min_duration"`
	MaxDuration         *int      `yaml:"max_duration"`
	ExcludeAvailability *[]string `yaml:"exclude_availability"`
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
	VideoEmbed       *bool         `yaml:"video_embed"`
	Filter           *FilterOverride `yaml:"filter"`
	Cookie           *CookieConfig   `yaml:"cookie"`
	CopyTo           *CopyToConfig   `yaml:"copy_to"`
}

type PlaylistConfig struct {
	URL              string          `yaml:"url"`
	Name             string          `yaml:"name"`
	Count            int             `yaml:"count"`
	SummaryPromptFile string         `yaml:"summary_prompt_file"`
	VideoEmbed       *bool           `yaml:"video_embed"`
	Filter           *FilterOverride `yaml:"filter"`
	Cookie           *CookieConfig   `yaml:"cookie"`
	CopyTo           *CopyToConfig   `yaml:"copy_to"`
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
	cfg.expandPaths()
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
			TranscribeTimeout: 30,
			DownloadTimeout:   10,
			ModelSources: map[string]string{
				"tiny":            "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-tiny.bin",
				"base":            "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.bin",
				"small":           "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small.bin",
				"medium":          "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-medium.bin",
				"large-v3":        "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-large-v3.bin",
				"large-v3-turbo":  "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-large-v3-turbo.bin",
				"belle-zh":        "https://huggingface.co/BELLE-2/Belle-whisper-large-v3-turbo-zh-ggml/resolve/main/ggml-model.bin",
				"kotoba-ja":       "https://huggingface.co/kotoba-tech/kotoba-whisper-v2.0-ggml/resolve/main/ggml-kotoba-whisper-v2.0.bin",
				"kotoba-ja-q5":    "https://huggingface.co/kotoba-tech/kotoba-whisper-v2.0-ggml/resolve/main/ggml-kotoba-whisper-v2.0-q5_0.bin",
			},
		},
		LLM: LLMConfig{
			Provider: ProviderList{"ollama"},
			ProviderFallbackStrategy: FallbackStrategyConfig{
				CooldownSeconds:  300,
				FailureThreshold: 1,
			},
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
			ClaudeCode: ClaudeCodeConfig{
				Model:   "haiku",
				Timeout: 900,
			},
			GeminiCLI: GeminiCLIConfig{
				Model:   "auto",
				Timeout: 900,
			},
			Antigravity: AntigravityCLIConfig{
				Timeout: 900,
			},
			QwenCode: QwenCodeConfig{
				Model:   "coder-model",
				Timeout: 900,
			},
			OpenAICompat: OpenAICompatConfig{
				Endpoint: "http://localhost:8000/v1",
				Timeout:  900,
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
			Types:               []string{"video", "live", "short"},
			ExcludeAvailability: []string{"members_only", "private"},
		},
		VideoEmbed: true,
		Batch: BatchConfig{
			RandomOrder:      true,
			WatchInterval:    10,
			FetchConcurrency: 3,
		},
	}
}

func (c *Config) expandEnvVars() {
	c.LLM.ClaudeAPI.APIKey = os.ExpandEnv(c.LLM.ClaudeAPI.APIKey)
}

func (c *Config) expandPaths() {
	c.OutputDir = ExpandHome(c.OutputDir)
	c.Whisper.ModelDir = ExpandHome(c.Whisper.ModelDir)
	c.Cookie.File = ExpandHome(c.Cookie.File)
	c.LLM.ClaudeCode.Path = ExpandHome(c.LLM.ClaudeCode.Path)
	c.LLM.GeminiCLI.Path = ExpandHome(c.LLM.GeminiCLI.Path)
	c.LLM.Antigravity.Path = ExpandHome(c.LLM.Antigravity.Path)
	c.LLM.QwenCode.Path = ExpandHome(c.LLM.QwenCode.Path)
	c.Summary.SummaryPromptFile = ExpandHome(c.Summary.SummaryPromptFile)

	for i := range c.Channels {
		c.Channels[i].SummaryPromptFile = ExpandHome(c.Channels[i].SummaryPromptFile)
		if c.Channels[i].CopyTo != nil {
			c.Channels[i].CopyTo.Path = ExpandHome(c.Channels[i].CopyTo.Path)
		}
	}
	for i := range c.Playlists {
		c.Playlists[i].SummaryPromptFile = ExpandHome(c.Playlists[i].SummaryPromptFile)
		if c.Playlists[i].CopyTo != nil {
			c.Playlists[i].CopyTo.Path = ExpandHome(c.Playlists[i].CopyTo.Path)
		}
	}
}

func (c *Config) EffectiveCount(ch ChannelConfig) int {
	if ch.Count > 0 {
		return ch.Count
	}
	return c.DefaultCount
}

// mergeFilter applies a FilterOverride on top of the global FilterConfig.
// nil fields in the override are inherited from base; non-nil fields (even
// zero/empty) override the base value.
func mergeFilter(base FilterConfig, over *FilterOverride) FilterConfig {
	if over == nil {
		return base
	}
	if over.Types != nil {
		base.Types = *over.Types
	}
	if over.MinDuration != nil {
		base.MinDuration = *over.MinDuration
	}
	if over.MaxDuration != nil {
		base.MaxDuration = *over.MaxDuration
	}
	if over.ExcludeAvailability != nil {
		base.ExcludeAvailability = *over.ExcludeAvailability
	}
	return base
}

func (c *Config) EffectiveFilter(ch ChannelConfig) FilterConfig {
	return mergeFilter(c.Filter, ch.Filter)
}

func (c *Config) EffectivePlaylistFilter(pl PlaylistConfig) FilterConfig {
	return mergeFilter(c.Filter, pl.Filter)
}

func (c *Config) EffectiveVideoEmbed(ch ChannelConfig) bool {
	if ch.VideoEmbed != nil {
		return *ch.VideoEmbed
	}
	return c.VideoEmbed
}

func (c *Config) EffectivePlaylistVideoEmbed(pl PlaylistConfig) bool {
	if pl.VideoEmbed != nil {
		return *pl.VideoEmbed
	}
	return c.VideoEmbed
}

// ReloadPartial re-reads the config file and updates only the fields that are
// safe to change at runtime: channels, playlists, filter, batch, default_count,
// and obsidian settings. LLM, whisper, and cookie settings are preserved since
// their dependent modules are not rebuilt.
func (c *Config) ReloadPartial(path string) error {
	path = ExpandHome(path)
	fresh, err := Load(path)
	if err != nil {
		return err
	}

	c.Channels = fresh.Channels
	c.Playlists = fresh.Playlists
	c.Filter = fresh.Filter
	c.Batch = fresh.Batch
	c.DefaultCount = fresh.DefaultCount
	c.Timezone = fresh.Timezone
	c.Obsidian = fresh.Obsidian
	c.VideoEmbed = fresh.VideoEmbed
	c.OutputDir = fresh.OutputDir
	c.PreferredLanguages = fresh.PreferredLanguages

	return nil
}

func ptrBool(v bool) *bool { return &v }
