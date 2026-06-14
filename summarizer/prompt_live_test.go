package summarizer

import (
	"os"
	"strings"
	"testing"

	"github.com/kouko/youtube-summarize-scraper/config"
)

// TestSummaryPromptLive renders the summary prompt against a sample transcript
// and runs it through your real configured provider chain, printing the result
// so you can eyeball prompt edits. It is a manual dev harness, NOT part of the
// default suite: it is skipped unless YTSS_LIVE_PROMPT=1 because it makes a real
// LLM call (costs tokens, needs an authenticated provider) — so it never runs
// in `go test ./...` or CI.
//
// It goes through the exact app path — ResolvePrompt -> SubstituteVars ->
// NewSummarizer -> Summarize — so what you see matches `ytss video`, only
// scoped to the summary stage (no yt-dlp / whisper / keywords / mermaid).
//
// Fast iteration loop (edit the prompt file, rerun this one test):
//
//	YTSS_LIVE_PROMPT=1 YTSS_PROMPT_FILE=./prompts/my-summary.md \
//	  go test ./summarizer/ -run TestSummaryPromptLive -v
//
// Env knobs (only the gate is required):
//
//	YTSS_LIVE_PROMPT=1          opt in to the real LLM call
//	YTSS_CONFIG=./config.yaml   config to read the provider chain + language from
//	YTSS_PROMPT_FILE=path       prompt template to test; default = app resolution (built-in by language)
//	YTSS_TRANSCRIPT=path        transcript to summarize; default = a short sample
func TestSummaryPromptLive(t *testing.T) {
	if os.Getenv("YTSS_LIVE_PROMPT") == "" {
		t.Skip("set YTSS_LIVE_PROMPT=1 to run (makes a real LLM call)")
	}

	cfgPath := envOrDefault("YTSS_CONFIG", "./config.yaml")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Skipf("need a config to pick the provider chain (%s): %v", cfgPath, err)
	}

	// Prompt template: explicit file override, else the app's own resolution
	// (per-config summary_prompt_file / inline prompt / built-in by language).
	var tmpl string
	if pf := os.Getenv("YTSS_PROMPT_FILE"); pf != "" {
		b, rerr := os.ReadFile(pf)
		if rerr != nil {
			t.Fatalf("read YTSS_PROMPT_FILE %q: %v", pf, rerr)
		}
		tmpl = string(b)
	} else {
		tmpl, err = ResolvePrompt(cfg.Summary, nil)
		if err != nil {
			t.Fatalf("resolve prompt: %v", err)
		}
	}

	transcript := sampleTranscript
	if tf := os.Getenv("YTSS_TRANSCRIPT"); tf != "" {
		b, rerr := os.ReadFile(tf)
		if rerr != nil {
			t.Fatalf("read YTSS_TRANSCRIPT %q: %v", tf, rerr)
		}
		transcript = string(b)
	}

	// Mirror pipeline.runSummarization's variable-building exactly so the
	// rendered prompt matches what the app sends.
	prompt := SubstituteVars(tmpl, PromptVars{
		Title:               "Sample Video",
		ChannelName:         "Sample Channel",
		Language:            cfg.Summary.Language,
		Transcript:          transcript,
		TranscriptionLength: len([]rune(transcript)),
	})

	s, err := NewSummarizer(cfg.LLM)
	if err != nil {
		t.Fatalf("new summarizer: %v", err)
	}

	res, err := s.Summarize(transcript, SummarizeOptions{
		Prompt:    prompt,
		MaxTokens: cfg.Summary.MaxTokens,
	})
	if err != nil {
		t.Fatalf("summarize: %v", err)
	}

	t.Logf("provider=%q model=%q response_len=%d", res.Provider, res.Model, len(res.Text))
	t.Logf("\n----- summary -----\n%s\n-------------------", res.Text)

	if strings.TrimSpace(res.Text) == "" {
		t.Fatal("provider returned an EMPTY summary")
	}
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

const sampleTranscript = "今日はAIを活用したiOSアプリ開発の進め方について話します。" +
	"まずアイデアの出し方、次に需要の検証、最後に実装とリリースの流れを説明します。" +
	"ポイントは小さく作って早く出し、ユーザーの反応を見ながら改善することです。"
