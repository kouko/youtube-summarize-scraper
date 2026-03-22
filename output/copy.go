package output

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Regex patterns to match frontmatter fields that need updating when copying.
var (
	rePlaylistLine   = regexp.MustCompile(`(?m)^playlist: ".*"$`)
	rePlaylistIDLine = regexp.MustCompile(`(?m)^playlist_id: ".*"$`)
	reProcessedAt    = regexp.MustCompile(`(?m)^processed_at: ".*"$`)
)

// CopyVideoDir copies all files from srcDir to dstDir, updating frontmatter
// in .md files to reflect the new playlist context.
func CopyVideoDir(srcDir, dstDir string, playlist, playlistID string) error {
	if err := EnsureDir(dstDir); err != nil {
		return fmt.Errorf("creating destination directory: %w", err)
	}

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return fmt.Errorf("reading source directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		srcPath := filepath.Join(srcDir, entry.Name())
		dstPath := filepath.Join(dstDir, entry.Name())

		if strings.HasSuffix(entry.Name(), ".md") {
			// Read, update frontmatter, and write.
			data, err := os.ReadFile(srcPath)
			if err != nil {
				return fmt.Errorf("reading %s: %w", srcPath, err)
			}
			content := updateFrontmatter(string(data), playlist, playlistID)
			if err := os.WriteFile(dstPath, []byte(content), 0o644); err != nil {
				return fmt.Errorf("writing %s: %w", dstPath, err)
			}
		} else {
			// Binary copy for non-markdown files (.srt, etc.).
			if err := copyFile(srcPath, dstPath); err != nil {
				return fmt.Errorf("copying %s: %w", srcPath, err)
			}
		}
	}

	return nil
}

// updateFrontmatter replaces playlist, playlist_id, and processed_at fields
// in the YAML frontmatter of a markdown file.
func updateFrontmatter(content, playlist, playlistID string) string {
	now := time.Now().Format(time.RFC3339)
	content = rePlaylistLine.ReplaceAllString(content, fmt.Sprintf(`playlist: "%s"`, playlist))
	content = rePlaylistIDLine.ReplaceAllString(content, fmt.Sprintf(`playlist_id: "%s"`, playlistID))
	content = reProcessedAt.ReplaceAllString(content, fmt.Sprintf(`processed_at: "%s"`, now))
	return content
}

// copyFile copies a single file from src to dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
