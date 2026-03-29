package config

import (
	"os"
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

func writeTestFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}
