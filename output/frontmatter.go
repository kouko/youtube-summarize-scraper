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
	UploadDate   string // YYYYMMDD from yt-dlp
	Duration     string
	Language     string
	Tags         []string
	Categories   []string
	SubtitleType string
	WhisperModel string // whisper model used (empty if subtitle download)
	ProcessedAt  string

	// Summary-only fields
	Keywords    []string
	LLMProvider string
	LLMModel    string
}

// BuildTranscriptionFrontmatter generates YAML frontmatter for a transcription
// markdown file.
func BuildTranscriptionFrontmatter(data FrontmatterData) string {
	formattedDate := formatDate(data.UploadDate)
	fmTitle := fmt.Sprintf("%s %s (transcription)", formattedDate, data.Title)

	var b strings.Builder
	b.WriteString("---\n")
	writeLine(&b, "title", fmTitle)
	writeLine(&b, "video_id", data.VideoID)
	writeLine(&b, "url", data.URL)
	writeLine(&b, "channel", data.Channel)
	writeLine(&b, "channel_name", data.ChannelName)
	writeLine(&b, "upload_date", formattedDate)
	writeLine(&b, "duration", data.Duration)
	writeLine(&b, "language", data.Language)
	writeList(&b, "tags", data.Tags)
	writeList(&b, "categories", data.Categories)
	writeLine(&b, "subtitle_type", data.SubtitleType)
	writeLine(&b, "whisper_model", data.WhisperModel)
	writeLine(&b, "processed_at", data.ProcessedAt)
	b.WriteString("---\n")

	return b.String()
}

// BuildSummaryFrontmatter generates YAML frontmatter for a summary markdown
// file. It includes additional fields: keywords, llm_provider, llm_model.
func BuildSummaryFrontmatter(data FrontmatterData) string {
	formattedDate := formatDate(data.UploadDate)
	fmTitle := fmt.Sprintf("%s %s (summary)", formattedDate, data.Title)

	var b strings.Builder
	b.WriteString("---\n")
	writeLine(&b, "title", fmTitle)
	writeLine(&b, "video_id", data.VideoID)
	writeLine(&b, "url", data.URL)
	writeLine(&b, "channel", data.Channel)
	writeLine(&b, "channel_name", data.ChannelName)
	writeLine(&b, "upload_date", formattedDate)
	writeLine(&b, "duration", data.Duration)
	writeLine(&b, "language", data.Language)
	writeList(&b, "tags", data.Tags)
	writeList(&b, "categories", data.Categories)
	writeLine(&b, "subtitle_type", data.SubtitleType)
	writeLine(&b, "processed_at", data.ProcessedAt)
	writeList(&b, "keywords", data.Keywords)
	writeLine(&b, "llm_provider", data.LLMProvider)
	writeLine(&b, "llm_model", data.LLMModel)
	b.WriteString("---\n")

	return b.String()
}

// writeLine writes a single key-value line in YAML format.
func writeLine(b *strings.Builder, key, value string) {
	fmt.Fprintf(b, "%s: \"%s\"\n", key, value)
}

// writeList writes a YAML list. If the slice is empty, it writes [].
func writeList(b *strings.Builder, key string, values []string) {
	if len(values) == 0 {
		fmt.Fprintf(b, "%s: []\n", key)
		return
	}
	fmt.Fprintf(b, "%s:\n", key)
	for _, v := range values {
		fmt.Fprintf(b, "  - \"%s\"\n", v)
	}
}
