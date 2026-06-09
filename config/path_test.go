package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandHome_TildeSlash(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot get home dir")
	}

	got := ExpandHome("~/Documents/config.yaml")
	want := filepath.Join(home, "Documents/config.yaml")
	if got != want {
		t.Errorf("ExpandHome ~/...: got %q, want %q", got, want)
	}
}

func TestExpandHome_TildeOnly(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot get home dir")
	}

	got := ExpandHome("~")
	if got != home {
		t.Errorf("ExpandHome ~: got %q, want %q", got, home)
	}
}

func TestExpandHome_NoTilde(t *testing.T) {
	got := ExpandHome("/absolute/path")
	if got != "/absolute/path" {
		t.Errorf("ExpandHome no tilde: got %q", got)
	}
}

func TestExpandHome_RelativePath(t *testing.T) {
	got := ExpandHome("./relative/path")
	if got != "./relative/path" {
		t.Errorf("ExpandHome relative: got %q", got)
	}
}

func TestExpandHome_Empty(t *testing.T) {
	got := ExpandHome("")
	if got != "" {
		t.Errorf("ExpandHome empty: got %q", got)
	}
}

func TestLoad_ExpandsPaths(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot get home dir")
	}

	yaml := `
output_dir: "~/ytss-output"
cookie:
  file: "~/cookies.txt"
llm:
  claude-code:
    path: "~/bin/claude"
  gemini-cli:
    path: "~/bin/gemini"
summary:
  summary_prompt_file: "~/prompts/summary.md"
channels:
  - url: "https://www.youtube.com/@test"
    summary_prompt_file: "~/prompts/channel.md"
    copy_to:
      path: "~/notes/youtube"
`
	path := t.TempDir() + "/config.yaml"
	if err := writeTestFile(path, yaml); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	checks := map[string]string{
		"OutputDir":                     filepath.Join(home, "ytss-output"),
		"Cookie.File":                   filepath.Join(home, "cookies.txt"),
		"LLM.ClaudeCode.Path":           filepath.Join(home, "bin/claude"),
		"LLM.GeminiCLI.Path":            filepath.Join(home, "bin/gemini"),
		"Summary.SummaryPromptFile":     filepath.Join(home, "prompts/summary.md"),
		"Channels[0].SummaryPromptFile": filepath.Join(home, "prompts/channel.md"),
		"Channels[0].CopyTo.Path":       filepath.Join(home, "notes/youtube"),
	}

	actuals := map[string]string{
		"OutputDir":                     cfg.OutputDir,
		"Cookie.File":                   cfg.Cookie.File,
		"LLM.ClaudeCode.Path":           cfg.LLM.ClaudeCode.Path,
		"LLM.GeminiCLI.Path":            cfg.LLM.GeminiCLI.Path,
		"Summary.SummaryPromptFile":     cfg.Summary.SummaryPromptFile,
		"Channels[0].SummaryPromptFile": cfg.Channels[0].SummaryPromptFile,
		"Channels[0].CopyTo.Path":       cfg.Channels[0].CopyTo.Path,
	}

	for name, want := range checks {
		got := actuals[name]
		if got != want {
			t.Errorf("%s: got %q, want %q", name, got, want)
		}
	}
}
