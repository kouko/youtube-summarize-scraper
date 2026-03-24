package output

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// reTagUnsafe matches characters not allowed in Obsidian tags.
// Allowed: letters (including CJK), numbers, hyphens, underscores.
var reTagUnsafe = regexp.MustCompile(`[^\p{L}\p{N}\-_]`)

// reMultiHyphen collapses consecutive hyphens.
var reMultiHyphen = regexp.MustCompile(`-+`)

// SanitizeTag converts a string into a valid Obsidian tag:
// lowercase, spaces → hyphens, remove special characters, collapse hyphens.
func SanitizeTag(tag string) string {
	tag = strings.TrimSpace(tag)
	tag = strings.ToLower(tag)
	tag = strings.TrimPrefix(tag, "@")
	tag = strings.ReplaceAll(tag, " ", "-")
	tag = reTagUnsafe.ReplaceAllString(tag, "")
	tag = reMultiHyphen.ReplaceAllString(tag, "-")
	tag = strings.Trim(tag, "-")
	return tag
}

// EnrichTagsForObsidian merges LLM-generated tags, a sanitized channel name,
// and autoTags into a single deduplicated list for Obsidian's tags field.
// All tags are sanitized for Obsidian compatibility (lowercase, no spaces, no special chars).
// YouTube native video tags are excluded (kept separately in video_tags).
func EnrichTagsForObsidian(llmTags []string, channelName string, autoTags []string) []string {
	seen := make(map[string]struct{})
	var result []string

	add := func(tag string) {
		tag = SanitizeTag(tag)
		if tag == "" {
			return
		}
		if _, ok := seen[tag]; ok {
			return
		}
		seen[tag] = struct{}{}
		result = append(result, tag)
	}

	for _, t := range llmTags {
		add(t)
	}

	add(channelName)

	for _, t := range autoTags {
		add(t)
	}

	return result
}

// GenerateChannelMOC creates or overwrites an _index.md file in the channel
// directory with a Dataview query listing all videos.
func GenerateChannelMOC(channelHandle string, outputDir string) error {
	channelDir := filepath.Join(outputDir, "@"+channelHandle)
	if err := EnsureDir(channelDir); err != nil {
		return fmt.Errorf("creating channel directory: %w", err)
	}

	// Use the channel directory name as the relative FROM path.
	fromPath := "@" + channelHandle

	content := fmt.Sprintf(`# @%s

`+"```dataview"+`
TABLE upload_date, duration, subtitle_type
FROM "%s"
WHERE video_id != null
SORT upload_date DESC
`+"```"+`
`, channelHandle, fromPath)

	mocPath := filepath.Join(channelDir, "_index.md")
	if err := os.WriteFile(mocPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing MOC file: %w", err)
	}

	return nil
}
