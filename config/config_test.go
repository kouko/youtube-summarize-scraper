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

func writeTestFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}
