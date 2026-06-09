package config

import (
	"os"
	"slices"
	"testing"
)

func TestDefaultConfig_WhisperTimeout(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Whisper.TranscribeTimeout != 30 {
		t.Errorf("TranscribeTimeout: got %d, want 30", cfg.Whisper.TranscribeTimeout)
	}
	if cfg.Whisper.DownloadTimeout != 10 {
		t.Errorf("DownloadTimeout: got %d, want 10", cfg.Whisper.DownloadTimeout)
	}
}

func TestLoad_WhisperTimeoutOverride(t *testing.T) {
	yaml := `
whisper:
  transcribe_timeout: 60
  download_timeout: 20
`
	path := t.TempDir() + "/config.yaml"
	if err := writeTestFile(path, yaml); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Whisper.TranscribeTimeout != 60 {
		t.Errorf("TranscribeTimeout: got %d, want 60", cfg.Whisper.TranscribeTimeout)
	}
	if cfg.Whisper.DownloadTimeout != 20 {
		t.Errorf("DownloadTimeout: got %d, want 20", cfg.Whisper.DownloadTimeout)
	}
}

func TestLoad_WhisperTimeoutDefault(t *testing.T) {
	// When not specified in YAML, defaults should apply.
	yaml := `
output_dir: "./test-output"
`
	path := t.TempDir() + "/config.yaml"
	if err := writeTestFile(path, yaml); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Whisper.TranscribeTimeout != 30 {
		t.Errorf("TranscribeTimeout: got %d, want 30 (default)", cfg.Whisper.TranscribeTimeout)
	}
	if cfg.Whisper.DownloadTimeout != 10 {
		t.Errorf("DownloadTimeout: got %d, want 10 (default)", cfg.Whisper.DownloadTimeout)
	}
}

func TestDefaultConfig_WatchMode(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Batch.Watch {
		t.Error("Watch: expected false by default")
	}
	if cfg.Batch.WatchInterval != 10 {
		t.Errorf("WatchInterval: got %d, want 10", cfg.Batch.WatchInterval)
	}
}

func TestLoad_WatchModeOverride(t *testing.T) {
	yaml := `
batch:
  watch: true
  watch_interval: 30
`
	path := t.TempDir() + "/config.yaml"
	if err := writeTestFile(path, yaml); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if !cfg.Batch.Watch {
		t.Error("Watch: expected true")
	}
	if cfg.Batch.WatchInterval != 30 {
		t.Errorf("WatchInterval: got %d, want 30", cfg.Batch.WatchInterval)
	}
}

func TestLoad_WatchModeDefault(t *testing.T) {
	yaml := `
output_dir: "./test-output"
`
	path := t.TempDir() + "/config.yaml"
	if err := writeTestFile(path, yaml); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Batch.Watch {
		t.Error("Watch: expected false (default)")
	}
	if cfg.Batch.WatchInterval != 10 {
		t.Errorf("WatchInterval: got %d, want 10 (default)", cfg.Batch.WatchInterval)
	}
}

func TestReloadPartial_UpdatesChannelsAndBatch(t *testing.T) {
	// Initial config.
	initial := `
channels:
  - url: "https://www.youtube.com/@old-channel"
    count: 3
batch:
  watch_interval: 10
`
	path := t.TempDir() + "/config.yaml"
	if err := writeTestFile(path, initial); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(cfg.Channels) != 1 || cfg.Channels[0].URL != "https://www.youtube.com/@old-channel" {
		t.Fatal("initial channels not loaded")
	}

	// Update config file: add a new channel and change interval.
	updated := `
channels:
  - url: "https://www.youtube.com/@old-channel"
    count: 3
  - url: "https://www.youtube.com/@new-channel"
    count: 5
batch:
  watch_interval: 30
filter:
  max_duration: 7200
`
	if err := writeTestFile(path, updated); err != nil {
		t.Fatal(err)
	}

	// Reload should update channels, batch, filter.
	if err := cfg.ReloadPartial(path); err != nil {
		t.Fatal(err)
	}

	if len(cfg.Channels) != 2 {
		t.Errorf("Channels: got %d, want 2", len(cfg.Channels))
	}
	if cfg.Channels[1].URL != "https://www.youtube.com/@new-channel" {
		t.Errorf("Channels[1].URL: got %q", cfg.Channels[1].URL)
	}
	if cfg.Batch.WatchInterval != 30 {
		t.Errorf("WatchInterval: got %d, want 30", cfg.Batch.WatchInterval)
	}
	if cfg.Filter.MaxDuration != 7200 {
		t.Errorf("MaxDuration: got %d, want 7200", cfg.Filter.MaxDuration)
	}
}

func TestReloadPartial_PreservesLLMAndWhisper(t *testing.T) {
	initial := `
llm:
  provider: "claude-api"
whisper:
  default_model: "large-v3"
channels:
  - url: "https://www.youtube.com/@ch1"
`
	path := t.TempDir() + "/config.yaml"
	if err := writeTestFile(path, initial); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	// Update config: change LLM provider in file (should be ignored by ReloadPartial).
	updated := `
llm:
  provider: "ollama"
whisper:
  default_model: "tiny"
channels:
  - url: "https://www.youtube.com/@ch1"
  - url: "https://www.youtube.com/@ch2"
`
	if err := writeTestFile(path, updated); err != nil {
		t.Fatal(err)
	}

	if err := cfg.ReloadPartial(path); err != nil {
		t.Fatal(err)
	}

	// LLM and whisper should NOT be updated.
	if cfg.LLM.Provider.Primary() != "claude-api" {
		t.Errorf("LLM.Provider: got %q, want 'claude-api' (preserved)", cfg.LLM.Provider.Primary())
	}
	if cfg.Whisper.DefaultModel != "large-v3" {
		t.Errorf("Whisper.DefaultModel: got %q, want 'large-v3' (preserved)", cfg.Whisper.DefaultModel)
	}

	// Channels should be updated.
	if len(cfg.Channels) != 2 {
		t.Errorf("Channels: got %d, want 2", len(cfg.Channels))
	}
}

func TestReloadPartial_FileNotFound(t *testing.T) {
	cfg := DefaultConfig()
	err := cfg.ReloadPartial("/nonexistent/config.yaml")
	if err == nil {
		t.Error("ReloadPartial: expected error for missing file")
	}
}

func TestLoad_Timezone(t *testing.T) {
	yaml := `
timezone: "Asia/Tokyo"
output_dir: "./out"
`
	path := t.TempDir() + "/config.yaml"
	if err := writeTestFile(path, yaml); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Timezone != "Asia/Tokyo" {
		t.Errorf("Timezone: got %q, want 'Asia/Tokyo'", cfg.Timezone)
	}
}

func TestLoad_TimezoneEmpty(t *testing.T) {
	yaml := `
output_dir: "./out"
`
	path := t.TempDir() + "/config.yaml"
	if err := writeTestFile(path, yaml); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Timezone != "" {
		t.Errorf("Timezone: got %q, want empty string", cfg.Timezone)
	}
}

func TestReloadPartial_UpdatesTimezone(t *testing.T) {
	initial := `
timezone: "UTC"
channels:
  - url: "https://www.youtube.com/@ch1"
`
	path := t.TempDir() + "/config.yaml"
	if err := writeTestFile(path, initial); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Timezone != "UTC" {
		t.Fatalf("initial Timezone: got %q, want 'UTC'", cfg.Timezone)
	}

	updated := `
timezone: "Asia/Taipei"
channels:
  - url: "https://www.youtube.com/@ch1"
`
	if err := writeTestFile(path, updated); err != nil {
		t.Fatal(err)
	}

	if err := cfg.ReloadPartial(path); err != nil {
		t.Fatal(err)
	}

	if cfg.Timezone != "Asia/Taipei" {
		t.Errorf("Timezone after reload: got %q, want 'Asia/Taipei'", cfg.Timezone)
	}
}

func TestProviderList_SingleString(t *testing.T) {
	yamlContent := `
llm:
  provider: "gemini-cli"
`
	path := t.TempDir() + "/config.yaml"
	if err := writeTestFile(path, yamlContent); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.LLM.Provider.Primary() != "gemini-cli" {
		t.Errorf("Primary: got %q, want %q", cfg.LLM.Provider.Primary(), "gemini-cli")
	}
	if len(cfg.LLM.Provider.Fallbacks()) != 0 {
		t.Errorf("Fallbacks: got %v, want empty", cfg.LLM.Provider.Fallbacks())
	}
}

func TestProviderList_MultipleProviders(t *testing.T) {
	yamlContent := `
llm:
  provider:
    - "gemini-cli"
    - "claude-code"
    - "ollama"
`
	path := t.TempDir() + "/config.yaml"
	if err := writeTestFile(path, yamlContent); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.LLM.Provider.Primary() != "gemini-cli" {
		t.Errorf("Primary: got %q, want %q", cfg.LLM.Provider.Primary(), "gemini-cli")
	}
	fallbacks := cfg.LLM.Provider.Fallbacks()
	if len(fallbacks) != 2 {
		t.Fatalf("Fallbacks: got %d items, want 2", len(fallbacks))
	}
	if fallbacks[0] != "claude-code" || fallbacks[1] != "ollama" {
		t.Errorf("Fallbacks: got %v, want [claude-code ollama]", fallbacks)
	}
}

func TestProviderList_FallbackStrategy(t *testing.T) {
	yamlContent := `
llm:
  provider:
    - "gemini-cli"
    - "claude-code"
  provider_fallback_strategy:
    cooldown_seconds: 600
    failure_threshold: 3
    rate_limit_cooldown_seconds: 90
    empty_response_threshold: 5
`
	path := t.TempDir() + "/config.yaml"
	if err := writeTestFile(path, yamlContent); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.LLM.ProviderFallbackStrategy.CooldownSeconds != 600 {
		t.Errorf("CooldownSeconds: got %d, want 600", cfg.LLM.ProviderFallbackStrategy.CooldownSeconds)
	}
	if cfg.LLM.ProviderFallbackStrategy.FailureThreshold != 3 {
		t.Errorf("FailureThreshold: got %d, want 3", cfg.LLM.ProviderFallbackStrategy.FailureThreshold)
	}
	if cfg.LLM.ProviderFallbackStrategy.RateLimitCooldownSeconds != 90 {
		t.Errorf("RateLimitCooldownSeconds: got %d, want 90", cfg.LLM.ProviderFallbackStrategy.RateLimitCooldownSeconds)
	}
	if cfg.LLM.ProviderFallbackStrategy.EmptyResponseThreshold != 5 {
		t.Errorf("EmptyResponseThreshold: got %d, want 5", cfg.LLM.ProviderFallbackStrategy.EmptyResponseThreshold)
	}
}

func TestProviderList_SetPrimary(t *testing.T) {
	p := ProviderList{"gemini-cli", "claude-code", "ollama"}
	p.SetPrimary("claude-code")

	if p.Primary() != "claude-code" {
		t.Errorf("Primary: got %q, want %q", p.Primary(), "claude-code")
	}
	// claude-code should not appear in fallbacks (deduplicated).
	fallbacks := p.Fallbacks()
	for _, fb := range fallbacks {
		if fb == "claude-code" {
			t.Error("SetPrimary should remove duplicate from fallbacks")
		}
	}
}

func TestProviderList_Default(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.LLM.Provider.Primary() != "ollama" {
		t.Errorf("Default primary: got %q, want %q", cfg.LLM.Provider.Primary(), "ollama")
	}
	if cfg.LLM.ProviderFallbackStrategy.CooldownSeconds != 300 {
		t.Errorf("Default cooldown: got %d, want 300", cfg.LLM.ProviderFallbackStrategy.CooldownSeconds)
	}
	if cfg.LLM.ProviderFallbackStrategy.RateLimitCooldownSeconds != 60 {
		t.Errorf("Default rate-limit cooldown: got %d, want 60", cfg.LLM.ProviderFallbackStrategy.RateLimitCooldownSeconds)
	}
	if cfg.LLM.ProviderFallbackStrategy.EmptyResponseThreshold != 3 {
		t.Errorf("Default empty-response threshold: got %d, want 3", cfg.LLM.ProviderFallbackStrategy.EmptyResponseThreshold)
	}
}

func TestProviderList_String(t *testing.T) {
	p := ProviderList{"gemini-cli", "claude-code"}
	if got := p.String(); got != "gemini-cli" {
		t.Errorf("String: got %q, want %q", got, "gemini-cli")
	}

	empty := ProviderList{}
	if got := empty.String(); got != "" {
		t.Errorf("String (empty): got %q, want empty", got)
	}
}

func TestProviderList_Primary_Empty(t *testing.T) {
	p := ProviderList{}
	if got := p.Primary(); got != "" {
		t.Errorf("Primary (empty): got %q, want empty", got)
	}
}

func TestProviderList_Equal(t *testing.T) {
	a := ProviderList{"gemini-cli", "claude-code"}
	b := ProviderList{"gemini-cli", "claude-code"}
	c := ProviderList{"gemini-cli"}
	d := ProviderList{"ollama", "claude-code"}

	if !a.Equal(b) {
		t.Error("identical lists should be equal")
	}
	if a.Equal(c) {
		t.Error("different length lists should not be equal")
	}
	if a.Equal(d) {
		t.Error("different content lists should not be equal")
	}
}

func TestProviderList_Contains(t *testing.T) {
	p := ProviderList{"gemini-cli", "Claude-Code"}
	if !p.Contains("gemini-cli") {
		t.Error("should contain gemini-cli")
	}
	if !p.Contains("claude-code") {
		t.Error("should contain claude-code (case-insensitive)")
	}
	if p.Contains("ollama") {
		t.Error("should not contain ollama")
	}
}

func TestProviderList_MarshalYAML_Single(t *testing.T) {
	p := ProviderList{"ollama"}
	v, err := p.MarshalYAML()
	if err != nil {
		t.Fatal(err)
	}
	s, ok := v.(string)
	if !ok {
		t.Fatalf("expected string, got %T", v)
	}
	if s != "ollama" {
		t.Errorf("got %q, want %q", s, "ollama")
	}
}

func TestProviderList_MarshalYAML_Multiple(t *testing.T) {
	p := ProviderList{"gemini-cli", "claude-code"}
	v, err := p.MarshalYAML()
	if err != nil {
		t.Fatal(err)
	}
	list, ok := v.([]string)
	if !ok {
		t.Fatalf("expected []string, got %T", v)
	}
	if len(list) != 2 || list[0] != "gemini-cli" || list[1] != "claude-code" {
		t.Errorf("got %v, want [gemini-cli claude-code]", list)
	}
}

func TestProviderList_SetPrimary_Empty(t *testing.T) {
	p := ProviderList{}
	p.SetPrimary("ollama")
	if p.Primary() != "ollama" {
		t.Errorf("Primary: got %q, want %q", p.Primary(), "ollama")
	}
	if len(p) != 1 {
		t.Errorf("length: got %d, want 1", len(p))
	}
}

func TestProviderList_SetPrimary_NoDuplicate(t *testing.T) {
	p := ProviderList{"gemini-cli", "ollama"}
	p.SetPrimary("ollama")
	if p.Primary() != "ollama" {
		t.Errorf("Primary: got %q, want %q", p.Primary(), "ollama")
	}
	// "ollama" should appear only once (as primary), "gemini-cli" stays as fallback.
	if len(p) != 2 {
		t.Errorf("length: got %d, want 2", len(p))
	}
	if p[1] != "gemini-cli" {
		t.Errorf("fallback[0]: got %q, want %q", p[1], "gemini-cli")
	}
}

func TestProviderList_UnmarshalYAML_InvalidType(t *testing.T) {
	yamlContent := `
llm:
  provider:
    key: value
`
	path := t.TempDir() + "/config.yaml"
	if err := writeTestFile(path, yamlContent); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Error("expected error for invalid provider YAML type")
	}
}

func TestDefaultConfig_ExcludeAvailability(t *testing.T) {
	cfg := DefaultConfig()

	want := []string{"members_only", "private"}
	got := cfg.Filter.ExcludeAvailability
	if len(got) != len(want) {
		t.Fatalf("ExcludeAvailability: got %v, want %v", got, want)
	}
	for i, v := range want {
		if got[i] != v {
			t.Errorf("ExcludeAvailability[%d]: got %q, want %q", i, got[i], v)
		}
	}
}

func TestDefaultConfig_FetchConcurrency(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Batch.FetchConcurrency != 3 {
		t.Errorf("FetchConcurrency: got %d, want 3", cfg.Batch.FetchConcurrency)
	}
}

func TestDefaultConfig_VideoEmbed(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.VideoEmbed {
		t.Error("VideoEmbed: default should be true")
	}
}

func TestLoad_VideoEmbedExplicitFalse(t *testing.T) {
	yaml := `video_embed: false`
	path := t.TempDir() + "/config.yaml"
	if err := writeTestFile(path, yaml); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.VideoEmbed {
		t.Error("VideoEmbed: should be false when explicitly set")
	}
}

func TestLoad_VideoEmbedOmitted(t *testing.T) {
	yaml := `default_count: 3`
	path := t.TempDir() + "/config.yaml"
	if err := writeTestFile(path, yaml); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.VideoEmbed {
		t.Error("VideoEmbed: should remain true when omitted from YAML")
	}
}

func TestEffectiveVideoEmbed_GlobalDefault(t *testing.T) {
	cfg := DefaultConfig()
	ch := ChannelConfig{URL: "https://www.youtube.com/@ch"}

	// Global true, no per-channel override → true.
	if !cfg.EffectiveVideoEmbed(ch) {
		t.Error("expected true (global default)")
	}

	// Global false, no per-channel override → false.
	cfg.VideoEmbed = false
	if cfg.EffectiveVideoEmbed(ch) {
		t.Error("expected false (global override)")
	}
}

func TestEffectiveVideoEmbed_PerChannelOverride(t *testing.T) {
	cfg := DefaultConfig() // VideoEmbed: true

	f := false
	ch := ChannelConfig{VideoEmbed: &f}
	if cfg.EffectiveVideoEmbed(ch) {
		t.Error("expected false (per-channel override)")
	}

	tr := true
	cfg.VideoEmbed = false
	ch.VideoEmbed = &tr
	if !cfg.EffectiveVideoEmbed(ch) {
		t.Error("expected true (per-channel override over global false)")
	}
}

func TestEffectivePlaylistVideoEmbed_GlobalDefault(t *testing.T) {
	cfg := DefaultConfig()
	pl := PlaylistConfig{URL: "https://www.youtube.com/playlist?list=WL"}

	if !cfg.EffectivePlaylistVideoEmbed(pl) {
		t.Error("expected true (global default)")
	}

	cfg.VideoEmbed = false
	if cfg.EffectivePlaylistVideoEmbed(pl) {
		t.Error("expected false (global override)")
	}
}

func TestEffectivePlaylistVideoEmbed_PerPlaylistOverride(t *testing.T) {
	cfg := DefaultConfig()

	f := false
	pl := PlaylistConfig{VideoEmbed: &f}
	if cfg.EffectivePlaylistVideoEmbed(pl) {
		t.Error("expected false (per-playlist override)")
	}

	tr := true
	cfg.VideoEmbed = false
	pl.VideoEmbed = &tr
	if !cfg.EffectivePlaylistVideoEmbed(pl) {
		t.Error("expected true (per-playlist override over global false)")
	}
}

func TestLoad_PerChannelVideoEmbed(t *testing.T) {
	yaml := `
video_embed: true
channels:
  - url: "https://www.youtube.com/@ch-a"
    video_embed: false
  - url: "https://www.youtube.com/@ch-b"
`
	path := t.TempDir() + "/config.yaml"
	if err := writeTestFile(path, yaml); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	// Channel A: explicit false.
	if cfg.EffectiveVideoEmbed(cfg.Channels[0]) {
		t.Error("channel-a: expected false")
	}
	// Channel B: no override → global true.
	if !cfg.EffectiveVideoEmbed(cfg.Channels[1]) {
		t.Error("channel-b: expected true (global default)")
	}
}

func TestLoad_PerPlaylistVideoEmbed(t *testing.T) {
	yaml := `
video_embed: true
playlists:
  - url: "https://www.youtube.com/playlist?list=PL1"
    video_embed: false
  - url: "https://www.youtube.com/playlist?list=PL2"
`
	path := t.TempDir() + "/config.yaml"
	if err := writeTestFile(path, yaml); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.EffectivePlaylistVideoEmbed(cfg.Playlists[0]) {
		t.Error("playlist-1: expected false")
	}
	if !cfg.EffectivePlaylistVideoEmbed(cfg.Playlists[1]) {
		t.Error("playlist-2: expected true (global default)")
	}
}

func TestReloadPartial_UpdatesVideoEmbed(t *testing.T) {
	initial := `video_embed: true`
	path := t.TempDir() + "/config.yaml"
	if err := writeTestFile(path, initial); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.VideoEmbed {
		t.Fatal("initial VideoEmbed should be true")
	}

	updated := `video_embed: false`
	if err := writeTestFile(path, updated); err != nil {
		t.Fatal(err)
	}
	if err := cfg.ReloadPartial(path); err != nil {
		t.Fatal(err)
	}
	if cfg.VideoEmbed {
		t.Error("VideoEmbed should be false after reload")
	}
}

// --- FilterOverride merge tests ---

func ptrInt(v int) *int               { return &v }
func ptrStrings(v []string) *[]string { return &v }

func TestEffectiveFilter_NilOverride(t *testing.T) {
	cfg := DefaultConfig()
	ch := ChannelConfig{URL: "https://www.youtube.com/@ch"}
	got := cfg.EffectiveFilter(ch)
	if !slices.Equal(got.Types, cfg.Filter.Types) {
		t.Errorf("Types: got %v, want %v", got.Types, cfg.Filter.Types)
	}
	if got.MinDuration != cfg.Filter.MinDuration {
		t.Errorf("MinDuration: got %d, want %d", got.MinDuration, cfg.Filter.MinDuration)
	}
	if !slices.Equal(got.ExcludeAvailability, cfg.Filter.ExcludeAvailability) {
		t.Errorf("ExcludeAvailability: got %v, want %v", got.ExcludeAvailability, cfg.Filter.ExcludeAvailability)
	}
}

func TestEffectiveFilter_PartialOverride_MinDuration(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Filter.Types = []string{"video", "live"}
	cfg.Filter.ExcludeAvailability = []string{"members_only", "private"}
	cfg.Filter.MinDuration = 300

	ch := ChannelConfig{
		Filter: &FilterOverride{MinDuration: ptrInt(60)},
	}
	got := cfg.EffectiveFilter(ch)

	if !slices.Equal(got.Types, []string{"video", "live"}) {
		t.Errorf("Types should inherit global, got %v", got.Types)
	}
	if got.MinDuration != 60 {
		t.Errorf("MinDuration: got %d, want 60", got.MinDuration)
	}
	if got.MaxDuration != 0 {
		t.Errorf("MaxDuration: got %d, want 0", got.MaxDuration)
	}
	if !slices.Equal(got.ExcludeAvailability, []string{"members_only", "private"}) {
		t.Errorf("ExcludeAvailability should inherit global, got %v", got.ExcludeAvailability)
	}
}

func TestEffectiveFilter_PartialOverride_Types(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Filter.MinDuration = 300
	cfg.Filter.ExcludeAvailability = []string{"members_only"}

	ch := ChannelConfig{
		Filter: &FilterOverride{Types: ptrStrings([]string{"short"})},
	}
	got := cfg.EffectiveFilter(ch)

	if !slices.Equal(got.Types, []string{"short"}) {
		t.Errorf("Types: got %v, want [short]", got.Types)
	}
	if got.MinDuration != 300 {
		t.Errorf("MinDuration should inherit global 300, got %d", got.MinDuration)
	}
	if !slices.Equal(got.ExcludeAvailability, []string{"members_only"}) {
		t.Errorf("ExcludeAvailability should inherit global, got %v", got.ExcludeAvailability)
	}
}

func TestEffectiveFilter_ExplicitZero_MinDuration(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Filter.MinDuration = 300

	ch := ChannelConfig{
		Filter: &FilterOverride{MinDuration: ptrInt(0)},
	}
	got := cfg.EffectiveFilter(ch)

	if got.MinDuration != 0 {
		t.Errorf("MinDuration: got %d, want 0 (explicit clear)", got.MinDuration)
	}
}

func TestEffectiveFilter_ExplicitEmpty_Types(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Filter.Types = []string{"video", "live"}

	ch := ChannelConfig{
		Filter: &FilterOverride{Types: ptrStrings([]string{})},
	}
	got := cfg.EffectiveFilter(ch)

	if len(got.Types) != 0 {
		t.Errorf("Types: got %v, want empty (explicit clear)", got.Types)
	}
}

func TestEffectiveFilter_ExplicitZero_MaxDuration(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Filter.MaxDuration = 3600

	ch := ChannelConfig{
		Filter: &FilterOverride{MaxDuration: ptrInt(0)},
	}
	got := cfg.EffectiveFilter(ch)

	if got.MaxDuration != 0 {
		t.Errorf("MaxDuration: got %d, want 0 (explicit clear)", got.MaxDuration)
	}
	// Other fields should inherit.
	if !slices.Equal(got.Types, cfg.Filter.Types) {
		t.Errorf("Types should inherit global, got %v", got.Types)
	}
}

func TestEffectiveFilter_ExplicitEmpty_ExcludeAvailability(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Filter.ExcludeAvailability = []string{"members_only", "private"}

	ch := ChannelConfig{
		Filter: &FilterOverride{ExcludeAvailability: ptrStrings([]string{})},
	}
	got := cfg.EffectiveFilter(ch)

	if len(got.ExcludeAvailability) != 0 {
		t.Errorf("ExcludeAvailability: got %v, want empty (explicit clear)", got.ExcludeAvailability)
	}
	// Other fields should inherit.
	if !slices.Equal(got.Types, cfg.Filter.Types) {
		t.Errorf("Types should inherit global, got %v", got.Types)
	}
}

func TestEffectiveFilter_EmptyOverride(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Filter = FilterConfig{
		Types:               []string{"video", "live"},
		MinDuration:         300,
		MaxDuration:         3600,
		ExcludeAvailability: []string{"members_only", "private"},
	}

	// FilterOverride is non-nil but all fields are nil → should behave like no override.
	ch := ChannelConfig{
		Filter: &FilterOverride{},
	}
	got := cfg.EffectiveFilter(ch)

	if !slices.Equal(got.Types, []string{"video", "live"}) {
		t.Errorf("Types should inherit global, got %v", got.Types)
	}
	if got.MinDuration != 300 {
		t.Errorf("MinDuration should inherit global 300, got %d", got.MinDuration)
	}
	if got.MaxDuration != 3600 {
		t.Errorf("MaxDuration should inherit global 3600, got %d", got.MaxDuration)
	}
	if !slices.Equal(got.ExcludeAvailability, []string{"members_only", "private"}) {
		t.Errorf("ExcludeAvailability should inherit global, got %v", got.ExcludeAvailability)
	}
}

func TestEffectiveFilter_FullOverride(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Filter = FilterConfig{
		Types:               []string{"video"},
		MinDuration:         100,
		MaxDuration:         500,
		ExcludeAvailability: []string{"private"},
	}

	ch := ChannelConfig{
		Filter: &FilterOverride{
			Types:               ptrStrings([]string{"short", "live"}),
			MinDuration:         ptrInt(200),
			MaxDuration:         ptrInt(1000),
			ExcludeAvailability: ptrStrings([]string{"members_only"}),
		},
	}
	got := cfg.EffectiveFilter(ch)

	if !slices.Equal(got.Types, []string{"short", "live"}) {
		t.Errorf("Types: got %v", got.Types)
	}
	if got.MinDuration != 200 {
		t.Errorf("MinDuration: got %d", got.MinDuration)
	}
	if got.MaxDuration != 1000 {
		t.Errorf("MaxDuration: got %d", got.MaxDuration)
	}
	if !slices.Equal(got.ExcludeAvailability, []string{"members_only"}) {
		t.Errorf("ExcludeAvailability: got %v", got.ExcludeAvailability)
	}
}

func TestEffectivePlaylistFilter_NilOverride(t *testing.T) {
	cfg := DefaultConfig()
	pl := PlaylistConfig{URL: "https://www.youtube.com/playlist?list=PL1"}
	got := cfg.EffectivePlaylistFilter(pl)
	if !slices.Equal(got.Types, cfg.Filter.Types) {
		t.Errorf("Types: got %v, want %v", got.Types, cfg.Filter.Types)
	}
}

func TestEffectivePlaylistFilter_PartialOverride(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Filter.MinDuration = 300
	cfg.Filter.ExcludeAvailability = []string{"members_only", "private"}

	pl := PlaylistConfig{
		URL:    "https://www.youtube.com/playlist?list=PL1",
		Filter: &FilterOverride{MinDuration: ptrInt(60)},
	}
	got := cfg.EffectivePlaylistFilter(pl)

	if got.MinDuration != 60 {
		t.Errorf("MinDuration: got %d, want 60", got.MinDuration)
	}
	if !slices.Equal(got.ExcludeAvailability, []string{"members_only", "private"}) {
		t.Errorf("ExcludeAvailability should inherit global, got %v", got.ExcludeAvailability)
	}
}

func TestEffectivePlaylistFilter_FullOverride(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Filter = FilterConfig{
		Types:               []string{"video"},
		MinDuration:         100,
		MaxDuration:         500,
		ExcludeAvailability: []string{"private"},
	}

	pl := PlaylistConfig{
		URL: "https://www.youtube.com/playlist?list=PL1",
		Filter: &FilterOverride{
			Types:               ptrStrings([]string{"short", "live"}),
			MinDuration:         ptrInt(200),
			MaxDuration:         ptrInt(1000),
			ExcludeAvailability: ptrStrings([]string{"members_only"}),
		},
	}
	got := cfg.EffectivePlaylistFilter(pl)

	if !slices.Equal(got.Types, []string{"short", "live"}) {
		t.Errorf("Types: got %v", got.Types)
	}
	if got.MinDuration != 200 {
		t.Errorf("MinDuration: got %d", got.MinDuration)
	}
	if got.MaxDuration != 1000 {
		t.Errorf("MaxDuration: got %d", got.MaxDuration)
	}
	if !slices.Equal(got.ExcludeAvailability, []string{"members_only"}) {
		t.Errorf("ExcludeAvailability: got %v", got.ExcludeAvailability)
	}
}

func TestEffectivePlaylistFilter_ExplicitZero(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Filter.MinDuration = 300
	cfg.Filter.MaxDuration = 3600
	cfg.Filter.ExcludeAvailability = []string{"members_only"}

	pl := PlaylistConfig{
		URL: "https://www.youtube.com/playlist?list=PL1",
		Filter: &FilterOverride{
			MinDuration:         ptrInt(0),
			ExcludeAvailability: ptrStrings([]string{}),
		},
	}
	got := cfg.EffectivePlaylistFilter(pl)

	if got.MinDuration != 0 {
		t.Errorf("MinDuration: got %d, want 0 (explicit clear)", got.MinDuration)
	}
	if got.MaxDuration != 3600 {
		t.Errorf("MaxDuration should inherit global 3600, got %d", got.MaxDuration)
	}
	if len(got.ExcludeAvailability) != 0 {
		t.Errorf("ExcludeAvailability: got %v, want empty (explicit clear)", got.ExcludeAvailability)
	}
}

func TestLoad_FilterOverrideMerge(t *testing.T) {
	yamlContent := `
filter:
  types: ["video", "live"]
  min_duration: 300
  exclude_availability: ["members_only", "private"]

channels:
  - url: "https://www.youtube.com/@ch-a"
    filter:
      min_duration: 60
  - url: "https://www.youtube.com/@ch-b"

playlists:
  - url: "https://www.youtube.com/playlist?list=PL1"
    filter:
      types: ["short"]
  - url: "https://www.youtube.com/playlist?list=PL2"
`
	path := t.TempDir() + "/config.yaml"
	if err := writeTestFile(path, yamlContent); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	// Channel A: min_duration overridden, rest inherited.
	chA := cfg.EffectiveFilter(cfg.Channels[0])
	if !slices.Equal(chA.Types, []string{"video", "live"}) {
		t.Errorf("ch-a Types: got %v, want [video live]", chA.Types)
	}
	if chA.MinDuration != 60 {
		t.Errorf("ch-a MinDuration: got %d, want 60", chA.MinDuration)
	}
	if !slices.Equal(chA.ExcludeAvailability, []string{"members_only", "private"}) {
		t.Errorf("ch-a ExcludeAvailability: got %v", chA.ExcludeAvailability)
	}

	// Channel B: no override, full global.
	chB := cfg.EffectiveFilter(cfg.Channels[1])
	if chB.MinDuration != 300 {
		t.Errorf("ch-b MinDuration: got %d, want 300", chB.MinDuration)
	}

	// Playlist 1: types overridden, rest inherited.
	pl1 := cfg.EffectivePlaylistFilter(cfg.Playlists[0])
	if !slices.Equal(pl1.Types, []string{"short"}) {
		t.Errorf("pl-1 Types: got %v, want [short]", pl1.Types)
	}
	if pl1.MinDuration != 300 {
		t.Errorf("pl-1 MinDuration: got %d, want 300", pl1.MinDuration)
	}

	// Playlist 2: no override, full global.
	pl2 := cfg.EffectivePlaylistFilter(cfg.Playlists[1])
	if !slices.Equal(pl2.Types, []string{"video", "live"}) {
		t.Errorf("pl-2 Types: got %v, want [video live]", pl2.Types)
	}
}

func TestReloadPartial_FilterMergeAfterReload(t *testing.T) {
	initial := `
filter:
  min_duration: 300
  types: ["video", "live"]

channels:
  - url: "https://www.youtube.com/@ch"
    filter:
      min_duration: 60
`
	path := t.TempDir() + "/config.yaml"
	if err := writeTestFile(path, initial); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	// Before reload: channel inherits types from global, overrides min_duration.
	got := cfg.EffectiveFilter(cfg.Channels[0])
	if got.MinDuration != 60 {
		t.Errorf("before reload: MinDuration: got %d, want 60", got.MinDuration)
	}
	if !slices.Equal(got.Types, []string{"video", "live"}) {
		t.Errorf("before reload: Types: got %v", got.Types)
	}

	// Reload with changed global filter.
	updated := `
filter:
  min_duration: 500
  max_duration: 7200
  types: ["video"]

channels:
  - url: "https://www.youtube.com/@ch"
    filter:
      min_duration: 60
`
	if err := writeTestFile(path, updated); err != nil {
		t.Fatal(err)
	}
	if err := cfg.ReloadPartial(path); err != nil {
		t.Fatal(err)
	}

	// After reload: channel still overrides min_duration=60, but inherits new global types and max_duration.
	got2 := cfg.EffectiveFilter(cfg.Channels[0])
	if got2.MinDuration != 60 {
		t.Errorf("after reload: MinDuration: got %d, want 60", got2.MinDuration)
	}
	if got2.MaxDuration != 7200 {
		t.Errorf("after reload: MaxDuration should inherit new global 7200, got %d", got2.MaxDuration)
	}
	if !slices.Equal(got2.Types, []string{"video"}) {
		t.Errorf("after reload: Types should inherit new global [video], got %v", got2.Types)
	}
}

func TestLoad_FilterOverrideYAMLPointerDistinction(t *testing.T) {
	// Verify that YAML unmarshal correctly distinguishes between
	// "field not written" (nil) and "field explicitly set to zero/empty" (non-nil).
	yamlContent := `
filter:
  min_duration: 300
  max_duration: 3600
  types: ["video", "live"]
  exclude_availability: ["members_only"]

channels:
  - url: "https://www.youtube.com/@explicit-zero"
    filter:
      min_duration: 0
      types: []
  - url: "https://www.youtube.com/@omitted"
    filter:
      max_duration: 1800
`
	path := t.TempDir() + "/config.yaml"
	if err := writeTestFile(path, yamlContent); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	// Channel "explicit-zero": min_duration=0 and types=[] are explicitly set.
	chExplicit := cfg.Channels[0]
	if chExplicit.Filter == nil {
		t.Fatal("explicit-zero: Filter should not be nil")
	}
	if chExplicit.Filter.MinDuration == nil {
		t.Fatal("explicit-zero: MinDuration should be non-nil (explicitly 0)")
	}
	if *chExplicit.Filter.MinDuration != 0 {
		t.Errorf("explicit-zero: MinDuration: got %d, want 0", *chExplicit.Filter.MinDuration)
	}
	if chExplicit.Filter.Types == nil {
		t.Fatal("explicit-zero: Types should be non-nil (explicitly empty)")
	}
	if len(*chExplicit.Filter.Types) != 0 {
		t.Errorf("explicit-zero: Types: got %v, want empty", *chExplicit.Filter.Types)
	}
	// Fields not written should remain nil.
	if chExplicit.Filter.MaxDuration != nil {
		t.Errorf("explicit-zero: MaxDuration should be nil (not written), got %d", *chExplicit.Filter.MaxDuration)
	}
	if chExplicit.Filter.ExcludeAvailability != nil {
		t.Errorf("explicit-zero: ExcludeAvailability should be nil (not written), got %v", *chExplicit.Filter.ExcludeAvailability)
	}

	// Verify merge result: explicit zero overrides global, nil inherits global.
	merged := cfg.EffectiveFilter(cfg.Channels[0])
	if merged.MinDuration != 0 {
		t.Errorf("explicit-zero merged: MinDuration: got %d, want 0", merged.MinDuration)
	}
	if len(merged.Types) != 0 {
		t.Errorf("explicit-zero merged: Types: got %v, want empty", merged.Types)
	}
	if merged.MaxDuration != 3600 {
		t.Errorf("explicit-zero merged: MaxDuration should inherit 3600, got %d", merged.MaxDuration)
	}
	if !slices.Equal(merged.ExcludeAvailability, []string{"members_only"}) {
		t.Errorf("explicit-zero merged: ExcludeAvailability should inherit, got %v", merged.ExcludeAvailability)
	}

	// Channel "omitted": only max_duration is set.
	chOmitted := cfg.Channels[1]
	if chOmitted.Filter == nil {
		t.Fatal("omitted: Filter should not be nil")
	}
	if chOmitted.Filter.MinDuration != nil {
		t.Errorf("omitted: MinDuration should be nil (not written), got %d", *chOmitted.Filter.MinDuration)
	}
	if chOmitted.Filter.MaxDuration == nil || *chOmitted.Filter.MaxDuration != 1800 {
		t.Errorf("omitted: MaxDuration: got %v, want ptr(1800)", chOmitted.Filter.MaxDuration)
	}

	// Verify merge result: omitted fields inherit global.
	merged2 := cfg.EffectiveFilter(cfg.Channels[1])
	if merged2.MinDuration != 300 {
		t.Errorf("omitted merged: MinDuration should inherit 300, got %d", merged2.MinDuration)
	}
	if merged2.MaxDuration != 1800 {
		t.Errorf("omitted merged: MaxDuration: got %d, want 1800", merged2.MaxDuration)
	}
}

func TestLoad_PlaylistFilterYAMLPointerDistinction(t *testing.T) {
	yamlContent := `
filter:
  min_duration: 300
  max_duration: 3600
  types: ["video", "live"]
  exclude_availability: ["members_only"]

playlists:
  - url: "https://www.youtube.com/playlist?list=PL-explicit"
    filter:
      min_duration: 0
      exclude_availability: []
  - url: "https://www.youtube.com/playlist?list=PL-partial"
    filter:
      max_duration: 1800
`
	path := t.TempDir() + "/config.yaml"
	if err := writeTestFile(path, yamlContent); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	// Playlist "explicit": min_duration=0 and exclude_availability=[] are explicitly set.
	plExplicit := cfg.Playlists[0]
	if plExplicit.Filter == nil {
		t.Fatal("explicit: Filter should not be nil")
	}
	if plExplicit.Filter.MinDuration == nil {
		t.Fatal("explicit: MinDuration should be non-nil (explicitly 0)")
	}
	if *plExplicit.Filter.MinDuration != 0 {
		t.Errorf("explicit: MinDuration: got %d, want 0", *plExplicit.Filter.MinDuration)
	}
	if plExplicit.Filter.ExcludeAvailability == nil {
		t.Fatal("explicit: ExcludeAvailability should be non-nil (explicitly empty)")
	}
	if len(*plExplicit.Filter.ExcludeAvailability) != 0 {
		t.Errorf("explicit: ExcludeAvailability: got %v, want empty", *plExplicit.Filter.ExcludeAvailability)
	}
	// Fields not written should remain nil.
	if plExplicit.Filter.MaxDuration != nil {
		t.Errorf("explicit: MaxDuration should be nil (not written), got %d", *plExplicit.Filter.MaxDuration)
	}
	if plExplicit.Filter.Types != nil {
		t.Errorf("explicit: Types should be nil (not written), got %v", *plExplicit.Filter.Types)
	}

	// Verify merge result.
	merged := cfg.EffectivePlaylistFilter(cfg.Playlists[0])
	if merged.MinDuration != 0 {
		t.Errorf("explicit merged: MinDuration: got %d, want 0", merged.MinDuration)
	}
	if len(merged.ExcludeAvailability) != 0 {
		t.Errorf("explicit merged: ExcludeAvailability: got %v, want empty", merged.ExcludeAvailability)
	}
	if merged.MaxDuration != 3600 {
		t.Errorf("explicit merged: MaxDuration should inherit 3600, got %d", merged.MaxDuration)
	}
	if !slices.Equal(merged.Types, []string{"video", "live"}) {
		t.Errorf("explicit merged: Types should inherit, got %v", merged.Types)
	}

	// Playlist "partial": only max_duration is set.
	plPartial := cfg.Playlists[1]
	if plPartial.Filter == nil {
		t.Fatal("partial: Filter should not be nil")
	}
	if plPartial.Filter.MinDuration != nil {
		t.Errorf("partial: MinDuration should be nil (not written), got %d", *plPartial.Filter.MinDuration)
	}
	if plPartial.Filter.MaxDuration == nil || *plPartial.Filter.MaxDuration != 1800 {
		t.Errorf("partial: MaxDuration: got %v, want ptr(1800)", plPartial.Filter.MaxDuration)
	}

	merged2 := cfg.EffectivePlaylistFilter(cfg.Playlists[1])
	if merged2.MinDuration != 300 {
		t.Errorf("partial merged: MinDuration should inherit 300, got %d", merged2.MinDuration)
	}
	if merged2.MaxDuration != 1800 {
		t.Errorf("partial merged: MaxDuration: got %d, want 1800", merged2.MaxDuration)
	}
}

func writeTestFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

func TestExpandPaths_AntigravityPath(t *testing.T) {
	// All CLI-backend binary paths must support ~ expansion; antigravity-cli
	// included (regression: it was initially omitted from expandPaths).
	c := &Config{}
	c.LLM.Antigravity.Path = "~/bin/agy"
	c.expandPaths()
	if got, want := c.LLM.Antigravity.Path, ExpandHome("~/bin/agy"); got != want {
		t.Errorf("Antigravity.Path not expanded: got %q, want %q", got, want)
	}
}
