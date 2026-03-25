package summarizer

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/kouko/youtube-summarize-scraper/prompts/builtin"
)

// KeywordPrompt generates a Stage 2 keyword extraction prompt.
// The prompt template is loaded from embedded files by language, with variable substitution.
func KeywordPrompt(summary string, language string, count int) (string, error) {
	template, err := loadBuiltinPromptByPrefix("keywords", language)
	if err != nil {
		return "", fmt.Errorf("loading keyword prompt: %w", err)
	}

	replacer := strings.NewReplacer(
		"{{count}}", strconv.Itoa(count),
		"{{summary}}", summary,
	)
	return replacer.Replace(template), nil
}

// loadBuiltinPromptByPrefix loads a built-in prompt template for the given prefix and language.
// Falls back to English if the requested language is not found.
func loadBuiltinPromptByPrefix(prefix string, language string) (string, error) {
	filename := prefix + "-" + language + ".md"
	data, err := builtin.Prompts.ReadFile(filename)
	if err != nil {
		// Fallback to English
		data, err = builtin.Prompts.ReadFile(prefix + "-en.md")
		if err != nil {
			return "", fmt.Errorf("loading built-in %s prompt: %w", prefix, err)
		}
	}
	return string(data), nil
}

// ParseKeywords splits an LLM response into individual keywords.
// It trims whitespace, removes bullet markers (-, *, bullet, numbers), and discards empty lines.
func ParseKeywords(response string) []string {
	lines := strings.Split(response, "\n")
	var keywords []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Remove common bullet markers
		line = strings.TrimLeft(line, "-*\u2022 ")
		// Remove numbered list prefixes like "1.", "2)", "10."
		line = trimNumberPrefix(line)
		line = strings.TrimSpace(line)

		if line != "" {
			keywords = append(keywords, line)
		}
	}

	return keywords
}

// trimNumberPrefix removes leading number prefixes like "1.", "2)", "10." from a line.
func trimNumberPrefix(s string) string {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i > 0 && i < len(s) && (s[i] == '.' || s[i] == ')') {
		return strings.TrimSpace(s[i+1:])
	}
	return s
}
