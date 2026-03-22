package output

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// reUnsafe matches characters that are not alphanumeric, CJK, or underscores.
var reUnsafe = regexp.MustCompile(`[^\p{L}\p{N}_\s]`)

// reMultiUnderscore collapses consecutive underscores.
var reMultiUnderscore = regexp.MustCompile(`_+`)

// SanitizeTitle removes special characters (keeping alphanumeric, CJK chars,
// underscores), replaces spaces with underscores, and limits length to maxLen.
// If maxLen is 0, it defaults to 80. Leading/trailing underscores are stripped.
func SanitizeTitle(title string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = 80
	}

	// Remove unsafe characters (keep letters, numbers, underscores, spaces).
	s := reUnsafe.ReplaceAllString(title, "")

	// Replace whitespace with underscores.
	s = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return '_'
		}
		return r
	}, s)

	// Collapse consecutive underscores.
	s = reMultiUnderscore.ReplaceAllString(s, "_")

	// Trim leading/trailing underscores.
	s = strings.Trim(s, "_")

	// Truncate to maxLen (rune-aware to preserve CJK characters).
	runes := []rune(s)
	if len(runes) > maxLen {
		runes = runes[:maxLen]
	}
	s = string(runes)

	// Trim trailing underscores after truncation.
	s = strings.TrimRight(s, "_")

	return s
}

// formatDate converts a yt-dlp date string (YYYYMMDD) to YYYY-MM-DD.
// If the input is not exactly 8 characters, it is returned as-is.
func formatDate(date string) string {
	if len(date) == 8 {
		return date[:4] + "-" + date[4:6] + "-" + date[6:8]
	}
	return date
}

// VideoDir returns the output directory path for a single video:
//
//	outputDir/@channelHandle/YYYY-MM-DD__videoID__sanitizedTitle
func VideoDir(outputDir, channelHandle, uploadDate, videoID, title string) string {
	sanitized := SanitizeTitle(title, 0)
	formattedDate := formatDate(uploadDate)
	dirName := fmt.Sprintf("%s__%s__%s", formattedDate, videoID, sanitized)
	return filepath.Join(outputDir, "@"+channelHandle, dirName)
}

// VideoFilePrefix returns the file name prefix for video output files:
//
//	YYYY-MM-DD__videoID__
func VideoFilePrefix(uploadDate, videoID string) string {
	return fmt.Sprintf("%s__%s__", formatDate(uploadDate), videoID)
}

// IsProcessed checks whether a video is fully processed (has summary.md)
// inside the channel directory.
func IsProcessed(outputDir, channelHandle, videoID string) (bool, error) {
	channelDir := filepath.Join(outputDir, "@"+channelHandle)
	pattern := filepath.Join(channelDir, fmt.Sprintf("*__%s__*", videoID), "*__summary.md")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return false, fmt.Errorf("glob pattern error: %w", err)
	}
	return len(matches) > 0, nil
}

// IsProcessedGlobal checks whether a video is fully processed (has summary.md)
// in any channel directory. Used when channel handle is unknown (flat-playlist).
func IsProcessedGlobal(outputDir, videoID string) (bool, error) {
	pattern := filepath.Join(outputDir, "*", fmt.Sprintf("*__%s__*", videoID), "*__summary.md")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return false, fmt.Errorf("glob pattern error: %w", err)
	}
	return len(matches) > 0, nil
}

// FindVideoDir returns the existing video directory path for a given video ID
// by searching across all channel directories. Returns "" if not found.
func FindVideoDir(outputDir, videoID string) string {
	pattern := filepath.Join(outputDir, "*", fmt.Sprintf("*__%s__*", videoID))
	matches, _ := filepath.Glob(pattern)
	if len(matches) > 0 {
		return matches[0]
	}
	return ""
}

// HasFile checks whether a file matching the given suffix exists in videoDir.
// Suffix examples: "summary.md", "transcription.md", "subtitle.srt"
func HasFile(videoDir, suffix string) bool {
	pattern := filepath.Join(videoDir, "*__"+suffix)
	matches, _ := filepath.Glob(pattern)
	return len(matches) > 0
}

// EnsureDir creates the directory (and all parents) if it does not exist.
func EnsureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}
