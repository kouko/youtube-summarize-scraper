package summarizer

import (
	"os"
	"strings"
	"testing"
	"time"
)

// TestClaudeCodeLive exercises the REAL claude CLI through the actual
// ClaudeCodeSummarizer code path (args, stdin, StripThinkingTags, result
// fields). It is skipped unless YTSS_LIVE_CLAUDE=1 because it makes a real
// LLM call (costs tokens, needs the claude binary authenticated) — so it never
// runs in the default suite or CI.
//
//	YTSS_LIVE_CLAUDE=1 go test ./summarizer/ -run TestClaudeCodeLive -v
func TestClaudeCodeLive(t *testing.T) {
	if os.Getenv("YTSS_LIVE_CLAUDE") == "" {
		t.Skip("set YTSS_LIVE_CLAUDE=1 to run the live claude-code provider test")
	}

	model := os.Getenv("YTSS_LIVE_CLAUDE_MODEL")
	if model == "" {
		model = "haiku"
	}

	s := &ClaudeCodeSummarizer{
		model:   model,
		timeout: 2 * time.Minute,
	}

	// Japanese transcript — mirrors the real failure case (a ja video whose
	// summary came back empty from the user's configured provider).
	transcript := "今日はAIを使ったiOSアプリ開発について話します。" +
		"まずアイデアの出し方、次に需要の検証方法、最後に実装の進め方を説明します。" +
		"重要なのは小さく作って早く出すことです。"

	prompt := "以下の動画字幕を、Markdownの見出し（##）と箇条書きで日本語で要約してください。" +
		"最初の行は必ず「## 概要」で始めてください。\n\n字幕:\n" + transcript

	start := time.Now()
	result, err := s.Summarize(transcript, SummarizeOptions{Prompt: prompt, MaxTokens: 1000})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("claude-code Summarize error after %s: %v", elapsed, err)
	}

	t.Logf("elapsed=%s provider=%q model=%q response_len=%d", elapsed, result.Provider, result.Model, len(result.Text))
	t.Logf("\n----- response -----\n%s\n--------------------", result.Text)

	if strings.TrimSpace(result.Text) == "" {
		t.Fatal("claude-code returned an EMPTY response (this is the failure the pipeline flags as 'LLM returned empty response')")
	}
	if !strings.Contains(result.Text, "##") {
		t.Errorf("response has no Markdown heading — expected structured output, got: %q", result.Text)
	}
}
