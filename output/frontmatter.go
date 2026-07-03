package output

import (
	"fmt"
	"strings"
)

// FrontmatterData holds all metadata fields used to generate YAML frontmatter
// for transcription and summary markdown files.
type FrontmatterData struct {
	Title        string
	VideoID      string
	URL          string
	Channel      string // @handle
	ChannelName  string
	UploadDate   string // YYYY-MM-DD (timezone-converted)
	UploadTime   string // RFC3339 in configured timezone
	Timezone     string // IANA timezone name used for conversion
	Duration     string
	Language     string
	VideoTags    []string // YouTube native tags
	Categories   []string
	Playlist     string // playlist name (empty for channel videos)
	PlaylistID   string // playlist ID (empty for channel videos)
	SubtitleType string
	WhisperModel string // whisper model used (empty if subtitle download)
	ProcessedAt  string

	// Summary-only fields
	Tags        []string // LLM-generated tags (formerly keywords)
	LLMProvider string
	LLMModel    string
}

// BuildTranscriptionFrontmatter generates YAML frontmatter for a transcription
// markdown file.
func BuildTranscriptionFrontmatter(data FrontmatterData) string {
	fmTitle := fmt.Sprintf("%s %s (transcription)", data.UploadDate, data.Title)

	var b strings.Builder
	b.WriteString("---\n")
	writeLine(&b, "title", fmTitle)
	writeLine(&b, "video_id", data.VideoID)
	writeLine(&b, "url", data.URL)
	writeLine(&b, "channel", data.Channel)
	writeLine(&b, "channel_name", data.ChannelName)
	writeLine(&b, "playlist", data.Playlist)
	writeLine(&b, "playlist_id", data.PlaylistID)
	writeLine(&b, "upload_date", data.UploadDate)
	writeLine(&b, "upload_time", data.UploadTime)
	writeLine(&b, "timezone", data.Timezone)
	writeLine(&b, "duration", data.Duration)
	writeLine(&b, "language", data.Language)
	writeList(&b, "video_tags", data.VideoTags)
	writeList(&b, "categories", data.Categories)
	writeLine(&b, "subtitle_type", data.SubtitleType)
	writeLine(&b, "whisper_model", data.WhisperModel)
	writeLine(&b, "processed_at", data.ProcessedAt)
	b.WriteString("---\n")

	return b.String()
}

// BuildSummaryFrontmatter generates YAML frontmatter for a summary markdown
// file. It includes additional fields: tags (LLM-generated), llm_provider, llm_model.
func BuildSummaryFrontmatter(data FrontmatterData) string {
	fmTitle := fmt.Sprintf("%s %s (summary)", data.UploadDate, data.Title)

	var b strings.Builder
	b.WriteString("---\n")
	writeLine(&b, "title", fmTitle)
	writeLine(&b, "video_id", data.VideoID)
	writeLine(&b, "url", data.URL)
	writeLine(&b, "channel", data.Channel)
	writeLine(&b, "channel_name", data.ChannelName)
	writeLine(&b, "playlist", data.Playlist)
	writeLine(&b, "playlist_id", data.PlaylistID)
	writeLine(&b, "upload_date", data.UploadDate)
	writeLine(&b, "upload_time", data.UploadTime)
	writeLine(&b, "timezone", data.Timezone)
	writeLine(&b, "duration", data.Duration)
	writeLine(&b, "language", data.Language)
	writeList(&b, "video_tags", data.VideoTags)
	writeList(&b, "categories", data.Categories)
	writeLine(&b, "subtitle_type", data.SubtitleType)
	writeLine(&b, "processed_at", data.ProcessedAt)
	writeList(&b, "tags", data.Tags)
	writeLine(&b, "llm_provider", data.LLMProvider)
	writeLine(&b, "llm_model", data.LLMModel)
	b.WriteString("---\n")

	return b.String()
}

// yamlEscape escapes a string for embedding inside a YAML double-quoted
// scalar, so values carrying " \ or control characters (e.g. raw YouTube
// titles) produce valid, parseable frontmatter. Every C0 control character
// (and DEL) must be escaped — a bare control byte inside "..." is rejected by
// YAML parsers ("control characters are not allowed") — so the common ones use
// their named escape and the rest fall back to \xNN.
func yamlEscape(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, `\x%02X`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

// writeLine writes a single key-value line in YAML format.
func writeLine(b *strings.Builder, key, value string) {
	fmt.Fprintf(b, "%s: \"%s\"\n", key, yamlEscape(value))
}

// writeList writes a YAML list. If the slice is empty, it writes [].
func writeList(b *strings.Builder, key string, values []string) {
	if len(values) == 0 {
		fmt.Fprintf(b, "%s: []\n", key)
		return
	}
	fmt.Fprintf(b, "%s:\n", key)
	for _, v := range values {
		fmt.Fprintf(b, "  - \"%s\"\n", yamlEscape(v))
	}
}
