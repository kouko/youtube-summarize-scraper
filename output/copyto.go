package output

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/kouko/youtube-summarize-scraper/config"
)

// CopyToVars holds variables for path/filename template substitution.
type CopyToVars struct {
	UploadDate    string
	VideoID       string
	Title         string
	ChannelName   string
	ChannelHandle string
	PlaylistName  string
	PlaylistID    string
}

// NormalizeCopyToVars applies fallback defaults for empty fields to prevent
// malformed filenames (e.g. leading underscores or double underscores).
func NormalizeCopyToVars(vars CopyToVars) CopyToVars {
	if vars.UploadDate == "" {
		vars.UploadDate = "unknown-date"
	}
	if vars.Title == "" && vars.VideoID != "" {
		vars.Title = vars.VideoID
	}
	return vars
}

// fileTypeInfo maps a file type name to its glob suffix and extension.
type fileTypeInfo struct {
	suffix string // e.g. "summary.md"
	ext    string // e.g. "md"
}

var fileTypes = map[string]fileTypeInfo{
	"summary":       {suffix: "summary.md", ext: "md"},
	"transcription": {suffix: "transcription.md", ext: "md"},
	"subtitle":      {suffix: "subtitle.srt", ext: "srt"},
}

// maxSegmentBytes is the maximum byte length for a single path segment (filename
// or directory name) on macOS/Linux filesystems.
const maxSegmentBytes = 255

// truncateRunesToBytes shortens s to fit within maxBytes at rune boundaries,
// trimming trailing underscores after truncation.
func truncateRunesToBytes(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	runes := []rune(s)
	for len(runes) > 0 && len([]byte(string(runes))) > maxBytes {
		runes = runes[:len(runes)-1]
	}
	return strings.TrimRight(string(runes), "_")
}

// resolveTemplate replaces {variable} placeholders with values from vars.
// Text values are sanitized for safe filesystem use. The fileType parameter
// is used to resolve {type}. Each path segment is guaranteed to fit within
// 255 bytes by progressively shortening channel_name, playlist_name, then
// title, with a final hard truncate as fallback.
func resolveTemplate(template string, vars CopyToVars, fileType string) string {
	// Sanitize text fields with initial max lengths.
	channelName := SanitizeTitle(vars.ChannelName, 0)
	playlistName := SanitizeTitle(vars.PlaylistName, 0)
	title := SanitizeTitle(vars.Title, 0)

	resolve := func(cn, pn, t string) string {
		r := strings.NewReplacer(
			"{upload_date}", vars.UploadDate,
			"{video_id}", vars.VideoID,
			"{title}", t,
			"{channel_name}", cn,
			"{channel_handle}", vars.ChannelHandle,
			"{playlist_name}", pn,
			"{playlist_id}", vars.PlaylistID,
			"{type}", fileType,
		)
		return r.Replace(template)
	}

	result := resolve(channelName, playlistName, title)

	// Check if any path segment exceeds the filesystem limit.
	if !hasOversizedSegment(result) {
		return result
	}

	// Progressive shortening: channel_name → playlist_name → title.
	type field struct {
		raw *string // pointer to the sanitized value
		src string  // original raw value from vars
	}
	fields := []field{
		{&channelName, vars.ChannelName},
		{&playlistName, vars.PlaylistName},
		{&title, vars.Title},
	}
	for _, maxLen := range []int{40, 20} {
		for _, f := range fields {
			if len([]rune(*f.raw)) > maxLen {
				*f.raw = SanitizeTitle(f.src, maxLen)
				result = resolve(channelName, playlistName, title)
				if !hasOversizedSegment(result) {
					return result
				}
			}
		}
	}

	// Final fallback: hard truncate each oversized segment, preserving extension.
	parts := strings.Split(result, string(filepath.Separator))
	for i, part := range parts {
		if len(part) <= maxSegmentBytes {
			continue
		}
		ext := filepath.Ext(part)
		base := strings.TrimSuffix(part, ext)
		base = truncateRunesToBytes(base, maxSegmentBytes-len(ext))
		parts[i] = base + ext
		slog.Warn("copy_to: path segment truncated to fit filesystem limit",
			"original_bytes", len(part), "segment", parts[i])
	}
	return strings.Join(parts, string(filepath.Separator))
}

// hasOversizedSegment returns true if any path segment exceeds maxSegmentBytes.
func hasOversizedSegment(path string) bool {
	for _, part := range strings.Split(path, string(filepath.Separator)) {
		if len(part) > maxSegmentBytes {
			return true
		}
	}
	return false
}

// ExecuteCopyTo copies specified files from videoDir to the target path
// defined in the CopyToConfig. Files are matched by type (summary,
// transcription, subtitle) using glob patterns.
// expandHome replaces a leading "~/" or "~" with the user's home directory.
func expandHome(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}

func ExecuteCopyTo(cfg config.CopyToConfig, videoDir string, filePrefix string, vars CopyToVars) error {
	vars = NormalizeCopyToVars(vars)

	// Default files to ["summary"] if empty.
	files := cfg.Files
	if len(files) == 0 {
		files = []string{"summary"}
	}

	var errs []string
	for _, fileType := range files {
		info, ok := fileTypes[fileType]
		if !ok {
			slog.Warn("copy_to: unknown file type, skipping", "type", fileType)
			continue
		}

		// Find source file by glob.
		pattern := filepath.Join(videoDir, "*__"+info.suffix)
		matches, err := filepath.Glob(pattern)
		if err != nil {
			errs = append(errs, fmt.Sprintf("glob error for %s: %v", fileType, err))
			continue
		}
		if len(matches) == 0 {
			slog.Warn("copy_to: source file not found, skipping", "type", fileType, "pattern", pattern)
			continue
		}
		srcPath := matches[0]

		// Resolve target directory (expand ~ to home dir).
		targetDir := expandHome(resolveTemplate(cfg.Path, vars, fileType))

		// Create target directory.
		if err := os.MkdirAll(targetDir, 0o755); err != nil {
			errs = append(errs, fmt.Sprintf("creating target dir for %s: %v", fileType, err))
			continue
		}

		// Determine target filename.
		var targetName string
		if cfg.Filename != "" {
			targetName = resolveTemplate(cfg.Filename, vars, fileType)
		} else {
			targetName = filepath.Base(srcPath)
		}

		targetPath := filepath.Join(targetDir, targetName)

		// Check overwrite.
		if !cfg.Overwrite {
			if _, err := os.Stat(targetPath); err == nil {
				slog.Info("copy_to: target exists, skipping (overwrite=false)",
					"type", fileType, "target", targetPath)
				continue
			}
		}

		// Copy file.
		if err := copyFile(srcPath, targetPath); err != nil {
			errs = append(errs, fmt.Sprintf("copying %s: %v", fileType, err))
			continue
		}

		slog.Info("copy_to: file copied", "type", fileType, "target", targetPath)
	}

	if len(errs) > 0 {
		return fmt.Errorf("copy_to errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

