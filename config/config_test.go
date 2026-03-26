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
	if cfg.LLM.Provider != "claude-api" {
		t.Errorf("LLM.Provider: got %q, want 'claude-api' (preserved)", cfg.LLM.Provider)
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

func writeTestFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}
