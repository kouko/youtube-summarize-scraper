package subtitle

import "testing"

func TestSRTToText(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "basic SRT content",
			input: `1
00:00:00,000 --> 00:00:05,000
Hello world

2
00:00:05,000 --> 00:00:10,000
This is a test
`,
			expected: "Hello world\nThis is a test",
		},
		{
			name:     "empty input",
			input:    "",
			expected: "",
		},
		{
			name: "multi-line subtitle entry",
			input: `1
00:00:00,000 --> 00:00:05,000
Line one
Line two

2
00:00:05,000 --> 00:00:10,000
Another entry
`,
			expected: "Line one\nLine two\nAnother entry",
		},
		{
			name: "SRT with extra blank lines",
			input: `1
00:00:00,000 --> 00:00:05,000
Hello world


2
00:00:05,000 --> 00:00:10,000
This is a test

`,
			expected: "Hello world\nThis is a test",
		},
		{
			name:     "SRT with CRLF line endings",
			input:    "1\r\n00:00:00,000 --> 00:00:05,000\r\nHello world\r\n\r\n2\r\n00:00:05,000 --> 00:00:10,000\r\nThis is a test\r\n",
			expected: "Hello world\nThis is a test",
		},
		{
			name: "YouTube rolling auto-subtitle dedup",
			input: `1
00:00:00,160 --> 00:00:02,070

Superpowers plugin for Claude Code is

2
00:00:02,070 --> 00:00:02,080
Superpowers plugin for Claude Code is


3
00:00:02,080 --> 00:00:03,669
Superpowers plugin for Claude Code is
getting a lot of hype right now. But

4
00:00:03,669 --> 00:00:03,679
getting a lot of hype right now. But


5
00:00:03,679 --> 00:00:05,269
getting a lot of hype right now. But
does it actually make a difference? In

6
00:00:05,269 --> 00:00:05,279
does it actually make a difference? In


7
00:00:05,279 --> 00:00:06,470
does it actually make a difference? In
this video, I'm going to build the same
`,
			expected: "Superpowers plugin for Claude Code is\ngetting a lot of hype right now. But\ndoes it actually make a difference? In\nthis video, I'm going to build the same",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SRTToText(tt.input)
			if got != tt.expected {
				t.Errorf("SRTToText() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestExtractLangFromFilename(t *testing.T) {
	tests := []struct {
		filename string
		prefix   string
		expected string
	}{
		{"video.ja.srt", "video", "ja"},
		{"video.en-US.srt", "video", "en-US"},
		{"video.zh-Hant.srt", "video", "zh-Hant"},
		{"video.en-orig.srt", "video", "en-orig"},
		{"video.srt", "video", ""},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := extractLangFromFilename(tt.filename, tt.prefix)
			if got != tt.expected {
				t.Errorf("extractLangFromFilename(%q, %q) = %q, want %q",
					tt.filename, tt.prefix, got, tt.expected)
			}
		})
	}
}
