package output

import (
	"os"
	"path/filepath"
	"strings"
)

// VideoEntry holds cached directory info for a single video.
type VideoEntry struct {
	Dir   string          // absolute path to the video directory
	Files map[string]bool // suffix → exists ("summary.md", "transcription.md", "subtitle.srt", ".skipped")
}

// VideoIndex is an in-memory index of video IDs to their output directories and files.
// Built once at pipeline startup, updated dynamically via Add/AddFile during processing.
type VideoIndex struct {
	entries map[string]*VideoEntry // videoID → entry
}

// BuildIndex scans the output directory tree and builds an in-memory index
// of videoID → directory path and known file suffixes.
// Directory structure: outputDir/{@channel|_playlist__*}/{DATE__videoID__title}/
func BuildIndex(outputDir string) *VideoIndex {
	idx := &VideoIndex{entries: make(map[string]*VideoEntry)}

	// Level 1: channel/playlist directories.
	topEntries, err := os.ReadDir(outputDir)
	if err != nil {
		return idx
	}

	for _, topEntry := range topEntries {
		if !topEntry.IsDir() {
			continue
		}
		topPath := filepath.Join(outputDir, topEntry.Name())

		// Level 2: video directories.
		videoEntries, err := os.ReadDir(topPath)
		if err != nil {
			continue
		}

		for _, videoEntry := range videoEntries {
			if !videoEntry.IsDir() {
				continue
			}

			videoID := extractVideoID(videoEntry.Name())
			if videoID == "" {
				continue
			}

			videoPath := filepath.Join(topPath, videoEntry.Name())
			entry := &VideoEntry{
				Dir:   videoPath,
				Files: make(map[string]bool),
			}

			// Level 3: files inside video directory.
			fileEntries, err := os.ReadDir(videoPath)
			if err != nil {
				continue
			}

			for _, fileEntry := range fileEntries {
				if fileEntry.IsDir() {
					continue
				}
				name := fileEntry.Name()
				for _, suffix := range []string{"summary.md", "transcription.md", "subtitle.srt", ".skipped"} {
					if strings.HasSuffix(name, "__"+suffix) {
						entry.Files[suffix] = true
					}
				}
			}

			idx.entries[videoID] = entry
		}
	}

	return idx
}

// extractVideoID extracts the video ID from a directory name formatted as DATE__videoID__title.
func extractVideoID(dirName string) string {
	parts := strings.SplitN(dirName, "__", 3)
	if len(parts) >= 2 {
		return parts[1]
	}
	return ""
}

// FindVideoDir returns the existing video directory path for a given video ID.
// Returns "" if not found or if the directory no longer exists on disk.
func (idx *VideoIndex) FindVideoDir(videoID string) string {
	entry := idx.entries[videoID]
	if entry == nil {
		return ""
	}
	// Verify directory still exists (handles external deletion).
	if _, err := os.Stat(entry.Dir); err != nil {
		delete(idx.entries, videoID)
		return ""
	}
	return entry.Dir
}

// HasFile checks whether a file with the given suffix exists for a video ID.
func (idx *VideoIndex) HasFile(videoID, suffix string) bool {
	entry := idx.entries[videoID]
	if entry == nil {
		return false
	}
	return entry.Files[suffix]
}

// IsProcessed checks whether a video is fully processed (has summary.md)
// inside the expected channel directory.
func (idx *VideoIndex) IsProcessed(channelHandle, videoID string) bool {
	entry := idx.entries[videoID]
	if entry == nil {
		return false
	}
	if !entry.Files["summary.md"] {
		return false
	}
	// Verify the video is in the expected channel directory.
	expectedPrefix := string(filepath.Separator) + "@" + channelHandle + string(filepath.Separator)
	return strings.Contains(entry.Dir, expectedPrefix)
}

// StatCheck performs an os.Stat on the actual file to detect external changes.
// Use this as a final check before expensive operations (whisper, LLM).
func (idx *VideoIndex) StatCheck(videoID, suffix string) bool {
	entry := idx.entries[videoID]
	if entry == nil {
		return false
	}

	// Scan the video directory for a file matching the suffix.
	fileEntries, err := os.ReadDir(entry.Dir)
	if err != nil {
		return false
	}
	for _, f := range fileEntries {
		if !f.IsDir() && strings.HasSuffix(f.Name(), "__"+suffix) {
			// Update the index to reflect the discovered file.
			entry.Files[suffix] = true
			return true
		}
	}
	return false
}

// Add registers a video directory in the index. If the video is already
// indexed (e.g. discovered by BuildIndex at startup), its recorded file flags
// are preserved and only the directory is updated — otherwise re-adding a
// known video before the resume check would wipe its transcription.md flag and
// silently defeat the resume path.
func (idx *VideoIndex) Add(videoID, dir string) {
	if entry, ok := idx.entries[videoID]; ok {
		entry.Dir = dir
		return
	}
	idx.entries[videoID] = &VideoEntry{
		Dir:   dir,
		Files: make(map[string]bool),
	}
}

// AddFile registers a new file suffix for an existing video in the index.
func (idx *VideoIndex) AddFile(videoID, suffix string) {
	entry := idx.entries[videoID]
	if entry == nil {
		return
	}
	entry.Files[suffix] = true
}
