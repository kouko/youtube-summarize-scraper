package summarizer

import (
	"strings"
	"testing"
)

func TestCalculateTier(t *testing.T) {
	tests := []struct {
		name     string
		count    int
		language string
		want     string
	}{
		// zh-Hant tiers
		{"zh-Hant tiny", 100, "zh-Hant", "< 500 字"},
		{"zh-Hant small", 499, "zh-Hant", "< 500 字"},
		{"zh-Hant medium-low", 500, "zh-Hant", "500-3,000 字"},
		{"zh-Hant medium", 2000, "zh-Hant", "500-3,000 字"},
		{"zh-Hant medium-high", 3000, "zh-Hant", "500-3,000 字"},
		{"zh-Hant large", 5000, "zh-Hant", "3,000-10,000 字"},
		{"zh-Hant large boundary", 10000, "zh-Hant", "3,000-10,000 字"},
		{"zh-Hant xlarge", 15000, "zh-Hant", "> 10,000 字"},

		// ja tiers
		{"ja tiny", 100, "ja", "< 500 文字"},
		{"ja medium", 1500, "ja", "500-3,000 文字"},
		{"ja large", 8000, "ja", "3,000-10,000 文字"},
		{"ja xlarge", 20000, "ja", "> 10,000 文字"},

		// en tiers
		{"en tiny", 500, "en", "< 1,000 chars"},
		{"en small", 999, "en", "< 1,000 chars"},
		{"en medium-low", 1000, "en", "1,000-5,000 chars"},
		{"en medium", 3000, "en", "1,000-5,000 chars"},
		{"en medium-high", 5000, "en", "1,000-5,000 chars"},
		{"en large", 10000, "en", "5,000-15,000 chars"},
		{"en large boundary", 15000, "en", "5,000-15,000 chars"},
		{"en xlarge", 20000, "en", "> 15,000 chars"},

		// Unknown language defaults to en thresholds
		{"unknown lang", 500, "fr", "< 1,000 chars"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateTier(tt.count, tt.language)
			if got != tt.want {
				t.Errorf("CalculateTier(%d, %q) = %q, want %q", tt.count, tt.language, got, tt.want)
			}
		})
	}
}

func TestSubstituteVars(t *testing.T) {
	template := "Title: {{title}}, Channel: {{channel_name}}, Lang: {{language}}, " +
		"Date: {{upload_date}}, Duration: {{duration}}, Tags: {{tags}}, " +
		"Length: {{transcription_length}}, Tier: {{transcription_tier}}\n\n{{transcript}}"

	vars := PromptVars{
		Title:               "Test Video",
		ChannelName:         "Test Channel",
		Language:            "en",
		UploadDate:          "2026-03-22",
		Duration:            "10:30",
		Tags:                "go, testing",
		Transcript:          "This is the transcript.",
		TranscriptionLength: 3000,
	}

	result := SubstituteVars(template, vars)

	if !strings.Contains(result, "Title: Test Video") {
		t.Error("expected title substitution")
	}
	if !strings.Contains(result, "Channel: Test Channel") {
		t.Error("expected channel_name substitution")
	}
	if !strings.Contains(result, "Lang: en") {
		t.Error("expected language substitution")
	}
	if !strings.Contains(result, "Date: 2026-03-22") {
		t.Error("expected upload_date substitution")
	}
	if !strings.Contains(result, "Duration: 10:30") {
		t.Error("expected duration substitution")
	}
	if !strings.Contains(result, "Tags: go, testing") {
		t.Error("expected tags substitution")
	}
	if !strings.Contains(result, "Length: 3000") {
		t.Error("expected transcription_length substitution")
	}
	if !strings.Contains(result, "Tier: 1,000-5,000 chars") {
		t.Error("expected transcription_tier substitution")
	}
	if !strings.Contains(result, "This is the transcript.") {
		t.Error("expected transcript substitution")
	}
}

func TestSubstituteVars_InlineNoTranscript(t *testing.T) {
	// Inline prompt without {{transcript}} should append transcript
	template := "Summarize the following video: {{title}}"
	vars := PromptVars{
		Title:      "My Video",
		Transcript: "Full transcript text here.",
		Language:   "en",
	}

	result := SubstituteVars(template, vars)

	if !strings.Contains(result, "Summarize the following video: My Video") {
		t.Error("expected title substitution")
	}
	if !strings.HasSuffix(result, "Full transcript text here.") {
		t.Error("expected transcript appended at end")
	}
}

func TestParseKeywords(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "plain lines",
			input:    "keyword1\nkeyword2\nkeyword3",
			expected: []string{"keyword1", "keyword2", "keyword3"},
		},
		{
			name:     "dash bullets",
			input:    "- keyword1\n- keyword2\n- keyword3",
			expected: []string{"keyword1", "keyword2", "keyword3"},
		},
		{
			name:     "asterisk bullets",
			input:    "* keyword1\n* keyword2",
			expected: []string{"keyword1", "keyword2"},
		},
		{
			name:     "bullet character",
			input:    "\u2022 keyword1\n\u2022 keyword2",
			expected: []string{"keyword1", "keyword2"},
		},
		{
			name:     "numbered list with dot",
			input:    "1. keyword1\n2. keyword2\n10. keyword3",
			expected: []string{"keyword1", "keyword2", "keyword3"},
		},
		{
			name:     "numbered list with paren",
			input:    "1) keyword1\n2) keyword2",
			expected: []string{"keyword1", "keyword2"},
		},
		{
			name:     "empty lines filtered",
			input:    "keyword1\n\n\nkeyword2\n\n",
			expected: []string{"keyword1", "keyword2"},
		},
		{
			name:     "whitespace trimmed",
			input:    "  keyword1  \n  keyword2  ",
			expected: []string{"keyword1", "keyword2"},
		},
		{
			name:     "mixed formats",
			input:    "- keyword1\n* keyword2\n3. keyword3\nkeyword4\n\n",
			expected: []string{"keyword1", "keyword2", "keyword3", "keyword4"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseKeywords(tt.input)
			if len(got) != len(tt.expected) {
				t.Fatalf("ParseKeywords() got %d keywords, want %d: %v", len(got), len(tt.expected), got)
			}
			for i, kw := range got {
				if kw != tt.expected[i] {
					t.Errorf("keyword[%d] = %q, want %q", i, kw, tt.expected[i])
				}
			}
		})
	}
}

func TestValidateMermaid(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantValid bool
		wantHas   string // substring expected in result
	}{
		{
			name: "valid graph TD",
			input: "Here is the flowchart:\n```mermaid\ngraph TD\n    A[\"Start\"] --> B[\"End\"]\n```\n",
			wantValid: true,
			wantHas:   "graph TD",
		},
		{
			name: "valid flowchart",
			input: "```mermaid\nflowchart TD\n    A[\"Start\"] --> B[\"Middle\"] --> C[\"End\"]\n```",
			wantValid: true,
			wantHas:   "flowchart TD",
		},
		{
			name:      "raw mermaid without code block",
			input:     "graph TD\n    A --> B",
			wantValid: true,
			wantHas:   "graph TD",
		},
		{
			name:      "no closing fence fallback",
			input:     "```mermaid\ngraph TD\n    A --> B",
			wantValid: true,
			wantHas:   "graph TD",
		},
		{
			name:      "wrong diagram type",
			input:     "```mermaid\nsequenceDiagram\n    participant A\n```",
			wantValid: false,
		},
		{
			name:      "auto-fix wrong arrows",
			input:     "```mermaid\ngraph TD\nA ---> B\nB ==> C\n```",
			wantValid: true,
			wantHas:   "-->",
		},
		{
			name:      "auto-fix missing quotes",
			input:     "```mermaid\ngraph TD\nA[Start] --> B[End]\n```",
			wantValid: true,
			wantHas:   `A["Start"]`,
		},
		{
			name:      "auto-fix chinese brackets",
			input:     "```mermaid\ngraph TD\nA【開始】--> B【結束】\n```",
			wantValid: true,
			wantHas:   `A["開始"]`,
		},
		{
			name:      "no arrows",
			input:     "```mermaid\ngraph TD\n    A[\"Start\"]\n    B[\"End\"]\n```",
			wantValid: false,
		},
		{
			name:      "empty input",
			input:     "",
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, valid := ValidateMermaid(tt.input)
			if valid != tt.wantValid {
				t.Errorf("ValidateMermaid() valid = %v, want %v (got content: %q)", valid, tt.wantValid, got)
			}
			if tt.wantValid && tt.wantHas != "" && !strings.Contains(got, tt.wantHas) {
				t.Errorf("ValidateMermaid() result missing %q, got %q", tt.wantHas, got)
			}
		})
	}
}
