package summarizer

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/kouko/youtube-summarize-scraper/config"
)

// PromptVars holds all variables available for prompt template substitution.
type PromptVars struct {
	Title               string
	ChannelName         string
	Language            string
	UploadDate          string
	Duration            string
	Tags                string
	Transcript          string
	TranscriptionLength int
}

// ResolvePrompt resolves the prompt template using 4-level resolution:
//  1. channelConfig.SummaryPromptFile (if set and channelConfig != nil)
//  2. summaryConfig.SummaryPromptFile (if set)
//  3. summaryConfig.Prompt (inline, if set)
//  4. Built-in prompt for summaryConfig.Language (default)
func ResolvePrompt(summaryConfig config.SummaryConfig, channelConfig *config.ChannelConfig) (string, error) {
	// Level 1: per-channel prompt file
	if channelConfig != nil && channelConfig.SummaryPromptFile != "" {
		return readPromptFile(channelConfig.SummaryPromptFile)
	}

	// Level 2: global prompt file
	if summaryConfig.SummaryPromptFile != "" {
		return readPromptFile(summaryConfig.SummaryPromptFile)
	}

	// Level 3: inline prompt
	if summaryConfig.Prompt != "" {
		return summaryConfig.Prompt, nil
	}

	// Level 4: built-in prompt by language + style
	return loadBuiltinPrompt(summaryConfig.Language, summaryConfig.Style)
}

// readPromptFile reads a prompt template from the given file path.
func readPromptFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading prompt file %q: %w", path, err)
	}
	return string(data), nil
}

// loadBuiltinPrompt loads a built-in summary prompt template for the given
// language and style. style "article" selects the article-style built-in; any
// other value (including "" and "outline") uses the default outline (list) one.
func loadBuiltinPrompt(language, style string) (string, error) {
	prefix := "summary"
	if style == "article" {
		prefix = "summary-article"
	}
	return loadBuiltinPromptByPrefix(prefix, language)
}

// SubstituteVars replaces {{variable}} placeholders in a template with values from vars.
// For inline prompts (those without {{transcript}}), the transcript is appended after the prompt.
func SubstituteVars(template string, vars PromptVars) string {
	tier := CalculateTier(vars.TranscriptionLength, vars.Language)
	lengthStr := strconv.Itoa(vars.TranscriptionLength)

	replacer := strings.NewReplacer(
		"{{title}}", vars.Title,
		"{{channel_name}}", vars.ChannelName,
		"{{language}}", vars.Language,
		"{{upload_date}}", vars.UploadDate,
		"{{duration}}", vars.Duration,
		"{{tags}}", vars.Tags,
		"{{transcript}}", vars.Transcript,
		"{{transcription_length}}", lengthStr,
		"{{transcription_tier}}", tier,
	)

	result := replacer.Replace(template)

	// If the template had no {{transcript}} placeholder, append transcript
	if !strings.Contains(template, "{{transcript}}") {
		result = result + "\n\n" + vars.Transcript
	}

	return result
}

// CalculateTier returns a tier label based on character count and language.
// CJK languages (zh-Hant, ja) use lower thresholds and language-specific units.
// English and other languages use higher thresholds with "chars" unit.
func CalculateTier(charCount int, language string) string {
	switch language {
	case "zh-Hant":
		return calculateCJKTier(charCount, "字")
	case "ja":
		return calculateCJKTier(charCount, "文字")
	default:
		return calculateEnTier(charCount)
	}
}

func calculateCJKTier(charCount int, unit string) string {
	switch {
	case charCount < 500:
		return fmt.Sprintf("< 500 %s", unit)
	case charCount <= 3000:
		return fmt.Sprintf("500-3,000 %s", unit)
	case charCount <= 10000:
		return fmt.Sprintf("3,000-10,000 %s", unit)
	default:
		return fmt.Sprintf("> 10,000 %s", unit)
	}
}

func calculateEnTier(charCount int) string {
	switch {
	case charCount < 1000:
		return "< 1,000 chars"
	case charCount <= 5000:
		return "1,000-5,000 chars"
	case charCount <= 15000:
		return "5,000-15,000 chars"
	default:
		return "> 15,000 chars"
	}
}
