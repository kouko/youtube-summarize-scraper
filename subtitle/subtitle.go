package subtitle

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/kouko/youtube-summarize-scraper/lang"
)

// DefaultStderr receives yt-dlp's raw stdout/stderr. It defaults to os.Stderr
// (CLI behavior unchanged); an embedding UI such as the TUI redirects it to
// io.Discard so raw progress lines never overwrite the rendered screen.
var DefaultStderr io.Writer = os.Stderr

// SubtitleResult holds the result of a subtitle download operation.
type SubtitleResult struct {
	Content      string // SRT content
	Language     string // Normalized ISO 639-1 language code
	SubtitleType string // "manual", "auto", or "whisper"
	FilePath     string // Path to the downloaded .srt file
}

// Downloader downloads subtitles using yt-dlp.
type Downloader struct {
	ytdlpPath  string
	ffmpegPath string
}

// NewDownloader creates a new Downloader with the given tool paths.
func NewDownloader(ytdlpPath, ffmpegPath string) *Downloader {
	return &Downloader{
		ytdlpPath:  ytdlpPath,
		ffmpegPath: ffmpegPath,
	}
}

// Download attempts to download subtitles for a video using a 4-step cascade:
//  1. Manual subs in target languages
//  2. Auto subs in target languages
//  3. Manual subs in any language
//  4. Auto subs in any language
//
// It returns the first successful result or an error if all steps fail.
func (d *Downloader) Download(videoURL string, languages []string, outputDir string, filePrefix string, cookieArgs []string) (*SubtitleResult, error) {
	langArg := strings.Join(languages, ",")
	outputTemplate := filepath.Join(outputDir, filePrefix)

	type step struct {
		name         string
		subtitleType string
		args         []string
	}

	steps := []step{
		{
			name:         "manual subs (target languages)",
			subtitleType: "manual",
			args:         d.buildArgs("--write-subs", langArg, outputTemplate, cookieArgs),
		},
		{
			name:         "auto subs (target languages)",
			subtitleType: "auto",
			args:         d.buildArgs("--write-auto-subs", langArg, outputTemplate, cookieArgs),
		},
		{
			name:         "manual subs (any language)",
			subtitleType: "manual",
			args:         d.buildArgs("--write-subs", "", outputTemplate, cookieArgs),
		},
		{
			name:         "auto subs (any language)",
			subtitleType: "auto",
			args:         d.buildArgs("--write-auto-subs", "", outputTemplate, cookieArgs),
		},
	}

	var lastErr error
	for _, s := range steps {
		args := append(s.args, videoURL)
		cmd := exec.Command(d.ytdlpPath, args...)
		cmd.Stdout = DefaultStderr
		cmd.Stderr = DefaultStderr

		_ = cmd.Run() // yt-dlp may return non-zero even when subs are written

		result, err := d.findSRTFile(outputDir, filePrefix, s.subtitleType)
		if err == nil {
			return result, nil
		}
		lastErr = fmt.Errorf("step %q: %w", s.name, err)
	}

	return nil, fmt.Errorf("all subtitle download steps failed: %w", lastErr)
}

// buildArgs constructs yt-dlp arguments for a download step.
func (d *Downloader) buildArgs(subFlag, langArg, outputTemplate string, cookieArgs []string) []string {
	args := []string{
		subFlag,
		"--skip-download",
		"--convert-subs", "srt",
		"-o", outputTemplate,
	}
	if langArg != "" {
		args = append(args, "--sub-lang", langArg)
	}
	if d.ffmpegPath != "" {
		args = append(args, "--ffmpeg-location", d.ffmpegPath)
	}
	args = append(args, cookieArgs...)
	return args
}

// findSRTFile looks for .srt files matching the prefix in outputDir.
func (d *Downloader) findSRTFile(outputDir, filePrefix, subtitleType string) (*SubtitleResult, error) {
	pattern := filepath.Join(outputDir, filePrefix+"*.srt")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob error: %w", err)
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("no .srt files found matching %s", pattern)
	}

	// Use the first match.
	srtPath := matches[0]

	content, err := os.ReadFile(srtPath)
	if err != nil {
		return nil, fmt.Errorf("reading srt file: %w", err)
	}

	// Extract language from filename: filePrefix.LANG.srt
	baseName := filepath.Base(srtPath)
	langCode := extractLangFromFilename(baseName, filePrefix)

	return &SubtitleResult{
		Content:      string(content),
		Language:     lang.NormalizeToISO639_1(langCode),
		SubtitleType: subtitleType,
		FilePath:     srtPath,
	}, nil
}

// extractLangFromFilename extracts the language code from a subtitle filename.
// Expected format: prefix.LANG.srt
func extractLangFromFilename(filename, prefix string) string {
	// Remove the prefix and .srt extension
	name := strings.TrimSuffix(filename, ".srt")
	if strings.HasPrefix(name, prefix) {
		rest := strings.TrimPrefix(name, prefix)
		rest = strings.TrimPrefix(rest, ".")
		if rest != "" {
			return rest
		}
	}
	return ""
}

// srtTimestampLine matches SRT timestamp lines like "00:00:00,000 --> 00:00:05,000"
var srtTimestampLine = regexp.MustCompile(`^\d{2}:\d{2}:\d{2},\d{3}\s*-->\s*\d{2}:\d{2}:\d{2},\d{3}`)

// srtSequenceNumber matches SRT sequence numbers (standalone digits on a line)
var srtSequenceNumber = regexp.MustCompile(`^\d+$`)

// SRTToText strips SRT formatting and returns deduplicated plain text.
// It removes sequence numbers, timestamps, and blank lines, then deduplicates
// consecutive identical lines caused by YouTube's rolling auto-subtitle format.
func SRTToText(srtContent string) string {
	lines := strings.Split(srtContent, "\n")
	var textLines []string

	// Extract text lines, skipping SRT metadata.
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if srtSequenceNumber.MatchString(trimmed) {
			continue
		}
		if srtTimestampLine.MatchString(trimmed) {
			continue
		}
		textLines = append(textLines, trimmed)
	}

	// Deduplicate consecutive identical lines.
	// YouTube auto-subs produce rolling blocks where each line appears 2-3 times.
	var deduped []string
	prevLine := ""
	for _, line := range textLines {
		if line != prevLine {
			deduped = append(deduped, line)
			prevLine = line
		}
	}

	return strings.Join(deduped, "\n")
}
