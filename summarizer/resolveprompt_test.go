package summarizer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kouko/youtube-summarize-scraper/config"
)

// TestResolvePromptTemplate pins the 4-level resolution order of the package's
// ResolvePrompt(summaryConfig, channelConfig): per-channel file > global file >
// inline prompt > built-in by language (with an en fallback for unknown langs).
//
// NOTE: this is the *uppercase* ResolvePrompt that picks WHICH template is used.
// It is distinct from the lowercase resolvePrompt(text, opts) covered by
// TestResolvePrompt in summarizer_test.go — don't confuse the two.
func TestResolvePromptTemplate(t *testing.T) {
	writeFile := func(t *testing.T, name, content string) string {
		t.Helper()
		p := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	t.Run("per-channel file wins over global file and inline", func(t *testing.T) {
		chFile := writeFile(t, "ch.md", "CHANNEL PROMPT")
		glFile := writeFile(t, "gl.md", "GLOBAL PROMPT")
		got, err := ResolvePrompt(
			config.SummaryConfig{SummaryPromptFile: glFile, Prompt: "INLINE", Language: "zh-Hant"},
			&config.ChannelConfig{SummaryPromptFile: chFile},
		)
		if err != nil {
			t.Fatal(err)
		}
		if got != "CHANNEL PROMPT" {
			t.Errorf("got %q, want channel-file content (highest priority)", got)
		}
	})

	t.Run("global file wins over inline", func(t *testing.T) {
		glFile := writeFile(t, "gl.md", "GLOBAL PROMPT")
		got, err := ResolvePrompt(
			config.SummaryConfig{SummaryPromptFile: glFile, Prompt: "INLINE"},
			nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		if got != "GLOBAL PROMPT" {
			t.Errorf("got %q, want global-file content over inline", got)
		}
	})

	t.Run("inline used when no files set", func(t *testing.T) {
		got, err := ResolvePrompt(config.SummaryConfig{Prompt: "INLINE PROMPT"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got != "INLINE PROMPT" {
			t.Errorf("got %q, want inline prompt", got)
		}
	})

	t.Run("built-in by language when nothing set", func(t *testing.T) {
		got, err := ResolvePrompt(config.SummaryConfig{Language: "zh-Hant"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		want, _ := loadBuiltinPrompt("zh-Hant", "")
		if got == "" || got != want {
			t.Errorf("expected the built-in zh-Hant prompt, got len=%d", len(got))
		}
	})

	t.Run("unknown language falls back to en built-in", func(t *testing.T) {
		got, err := ResolvePrompt(config.SummaryConfig{Language: "xx-unknown"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		en, _ := loadBuiltinPrompt("en", "")
		zh, _ := loadBuiltinPrompt("zh-Hant", "")
		if got != en {
			t.Error("unknown language should fall back to the en built-in")
		}
		if got == zh {
			t.Error("unknown language unexpectedly returned the zh-Hant prompt")
		}
	})

	t.Run("missing channel prompt file returns a wrapped error", func(t *testing.T) {
		_, err := ResolvePrompt(
			config.SummaryConfig{},
			&config.ChannelConfig{SummaryPromptFile: filepath.Join(t.TempDir(), "nope.md")},
		)
		if err == nil {
			t.Fatal("expected an error for a missing channel prompt file")
		}
		if !strings.Contains(err.Error(), "reading prompt file") {
			t.Errorf("error should mention reading the prompt file, got: %v", err)
		}
	})

	t.Run("nil channel config falls through to global", func(t *testing.T) {
		// Regression guard: a nil channelConfig must not panic and must use the global file.
		glFile := writeFile(t, "gl.md", "GLOBAL ONLY")
		got, err := ResolvePrompt(config.SummaryConfig{SummaryPromptFile: glFile}, nil)
		if err != nil || got != "GLOBAL ONLY" {
			t.Errorf("nil channel should use the global file; got %q err %v", got, err)
		}
	})

	t.Run("built-in style selects article vs classic", func(t *testing.T) {
		article, err := ResolvePrompt(config.SummaryConfig{Language: "zh-Hant", Style: "article"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		classic, err := ResolvePrompt(config.SummaryConfig{Language: "zh-Hant", Style: "classic"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if article == "" || classic == "" || article == classic {
			t.Fatal("article and classic built-ins should both load and differ")
		}
		if want, _ := loadBuiltinPromptByPrefix("summary", "zh-Hant"); article != want {
			t.Error("style=article should load the summary-<lang> built-in")
		}
		if want, _ := loadBuiltinPromptByPrefix("summary-classic", "zh-Hant"); classic != want {
			t.Error("style=classic should load the summary-classic-<lang> built-in")
		}
	})

	t.Run("unknown or empty style falls back to the article default", func(t *testing.T) {
		article, _ := ResolvePrompt(config.SummaryConfig{Language: "zh-Hant", Style: "article"}, nil)
		for _, s := range []string{"", "bogus"} {
			got, err := ResolvePrompt(config.SummaryConfig{Language: "zh-Hant", Style: s}, nil)
			if err != nil {
				t.Fatalf("style %q: %v", s, err)
			}
			if got != article {
				t.Errorf("style %q should fall back to the article default", s)
			}
		}
	})
}
