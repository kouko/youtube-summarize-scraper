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
// A video ID may have several candidate directories (cross-channel or playlist
// duplicates); each candidate keeps its own file marks.
type VideoIndex struct {
	entries   map[string][]*VideoEntry // videoID → candidate directories, ReadDir sort order
	lastAdded map[string]string        // videoID → dir registered by the most recent Add
}

// BuildIndex scans the output directory tree and builds an in-memory index
// of videoID → directory paths and known file suffixes.
// Directory structure: outputDir/{@channel|_playlist__*}/{DATE__videoID__title}/
func BuildIndex(outputDir string) *VideoIndex {
	idx := &VideoIndex{
		entries:   make(map[string][]*VideoEntry),
		lastAdded: make(map[string]string),
	}

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

			// Keep every parsed directory as a candidate: a single-winner overwrite
			// let one channel's duplicate shadow another's, and a stale empty
			// duplicate could steal the slot. Selection happens per query.
			idx.entries[videoID] = append(idx.entries[videoID], entry)
		}
	}

	return idx
}

// entryScore ranks a video directory so duplicate directories for the same
// video ID prefer the most complete one. 3 = summary.md (completed),
// 2 = .skipped (permanently skipped), 1 = any other known file, 0 = empty.
// summary.md must rank strictly above .skipped: HasFile(summary.md) must be
// true whenever a summary exists anywhere reachable, regardless of sort order.
// Ties keep the earlier-sorted entry (first occurrence wins on equal score).
func entryScore(entry *VideoEntry) int {
	if entry.Files["summary.md"] {
		return 3
	}
	if entry.Files[".skipped"] {
		return 2
	}
	if entry.Files["transcription.md"] || entry.Files["subtitle.srt"] {
		return 1
	}
	return 0
}

// extractVideoID extracts the video ID from a directory name formatted as DATE__videoID__title.
func extractVideoID(dirName string) string {
	// Directory format: DATE__videoID__title
	// videoID may itself contain "__", so we must find the first "__" (after DATE)
	// and the last "__" (before title), taking the middle segment.
	first := strings.Index(dirName, "__")
	if first == -1 {
		return ""
	}
	last := strings.LastIndex(dirName, "__")
	if last == -1 || last == first {
		return ""
	}
	// videoID starts after first "__", ends before last "__"
	start := first + 2
	end := last
	if start >= end {
		return ""
	}
	return dirName[start:end]
}

// bestEntry returns the highest-scoring candidate for a video ID, keeping the
// earlier-sorted candidate on ties (strictly-greater comparison first wins).
// Returns nil when the video is unknown. For queries with no channel
// parameter this is the single representative of the video.
func (idx *VideoIndex) bestEntry(videoID string) *VideoEntry {
	candidates := idx.entries[videoID]
	if len(candidates) == 0 {
		return nil
	}
	best := candidates[0]
	for _, c := range candidates[1:] {
		if entryScore(c) > entryScore(best) {
			best = c
		}
	}
	return best
}

// FindVideoDir returns the existing video directory path for a given video ID.
// Returns "" if not found or if the directory no longer exists on disk.
func (idx *VideoIndex) FindVideoDir(videoID string) string {
	candidates, ok := idx.entries[videoID]
	if !ok || len(candidates) == 0 {
		return ""
	}
	for {
		best := candidates[0]
		bestAt := 0
		for i, c := range candidates[1:] {
			if entryScore(c) > entryScore(best) {
				best = c
				bestAt = i + 1
			}
		}
		// Verify directory still exists (handles external deletion).
		if _, err := os.Stat(best.Dir); err == nil {
			idx.entries[videoID] = candidates
			return best.Dir
		}
		// The chosen candidate's directory is gone; drop it and fall back to
		// the next-best candidate instead of dropping the video entirely.
		candidates = append(candidates[:bestAt], candidates[bestAt+1:]...)
		if len(candidates) == 0 {
			delete(idx.entries, videoID)
			return ""
		}
	}
}

// HasFile checks whether a file with the given suffix exists for a video ID.
// The representative is the highest-scoring candidate, so summary.md (the
// strict top rank) is reported whenever any candidate directory has one.
func (idx *VideoIndex) HasFile(videoID, suffix string) bool {
	entry := idx.bestEntry(videoID)
	if entry == nil {
		return false
	}
	return entry.Files[suffix]
}

// IsProcessed checks whether a video is terminal — either fully processed
// (has summary.md) or permanently skipped (has a .skipped marker) — inside
// the expected channel directory.
func (idx *VideoIndex) IsProcessed(channelHandle, videoID string) bool {
	expectedPrefix := string(filepath.Separator) + "@" + channelHandle + string(filepath.Separator)
	for _, entry := range idx.entries[videoID] {
		if !entry.Files["summary.md"] && !entry.Files[".skipped"] {
			continue
		}
		// Verify the video is in the expected channel directory.
		if strings.Contains(entry.Dir, expectedPrefix) {
			return true
		}
	}
	return false
}

// StatCheck performs an os.Stat on the actual file to detect external changes.
// Use this as a final check before expensive operations (whisper, LLM).
func (idx *VideoIndex) StatCheck(videoID, suffix string) bool {
	entry := idx.bestEntry(videoID)
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

// Add registers a video directory in the index as a candidate. If a candidate
// with that exact directory already exists (e.g. discovered by BuildIndex at
// startup), its recorded file flags are preserved and only the working-dir
// pointer is updated — otherwise re-adding a known video before the resume
// check would wipe its transcription.md flag and silently defeat the resume
// path. Otherwise the new directory is appended to the video's candidate list.
func (idx *VideoIndex) Add(videoID, dir string) {
	idx.lastAdded[videoID] = dir
	for _, entry := range idx.entries[videoID] {
		if entry.Dir == dir {
			return
		}
	}
	idx.entries[videoID] = append(idx.entries[videoID], &VideoEntry{
		Dir:   dir,
		Files: make(map[string]bool),
	})
}

// AddFile registers a new file suffix for a video. AddFile carries no
// directory argument, so the mark lands on the candidate Add most recently
// registered as the working directory; when that directory is not among the
// candidates (e.g. an AddFile for a video known only to BuildIndex), the mark
// falls back to the highest-scoring candidate, preserving the previous
// single-entry semantics.
func (idx *VideoIndex) AddFile(videoID, suffix string) {
	if dir, ok := idx.lastAdded[videoID]; ok {
		for _, entry := range idx.entries[videoID] {
			if entry.Dir == dir {
				entry.Files[suffix] = true
				return
			}
		}
	}
	if entry := idx.bestEntry(videoID); entry != nil {
		entry.Files[suffix] = true
	}
}
