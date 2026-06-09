package cmd

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/kouko/youtube-summarize-scraper/fetcher"
	"github.com/kouko/youtube-summarize-scraper/pipeline"
	"github.com/spf13/cobra"
)

// reVideoID matches an 11-character YouTube video ID (alphanumeric, hyphens, underscores).
var reVideoID = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

var videoCmd = &cobra.Command{
	Use:   "video [URL or VIDEO_ID]",
	Short: "Summarize a single video",
	Long:  "Download subtitles, transcribe if needed, and generate a summary for a single video.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		setupLogging(verbose)

		cfg := loadConfig(cfgFile)
		applyOverrides(cfg)

		input := args[0]
		videoURL := input
		if reVideoID.MatchString(input) {
			videoURL = fmt.Sprintf("https://www.youtube.com/watch?v=%s", input)
		}

		p, err := pipeline.NewPipeline(cfg, forceFlag, dryRun)
		if err != nil {
			return fmt.Errorf("initializing pipeline: %w", err)
		}

		// Extract video ID from URL for initial meta.
		videoID := extractVideoID(videoURL)
		meta := &fetcher.VideoMeta{
			ID:  videoID,
			URL: videoURL,
		}

		if err := p.ProcessVideo(meta, nil); err != nil {
			if pipeline.IsSkipped(err) {
				fmt.Printf("video %s: skipped\n", meta.ID)
				return nil
			}
			if pipeline.IsPartial(err) {
				// Transcription saved; only summarization failed (e.g. quota).
				// Not a hard failure — a later run resumes the summary.
				fmt.Printf("video %s: partial — transcription saved, summary failed (%v)\n", meta.ID, err)
				return nil
			}
			return fmt.Errorf("processing video: %w", err)
		}

		fmt.Printf("video %s: completed successfully\n", meta.ID)
		return nil
	},
}

// extractVideoID extracts the video ID from a YouTube URL or returns the input as-is.
func extractVideoID(input string) string {
	if reVideoID.MatchString(input) {
		return input
	}
	// Try to extract from ?v= parameter.
	if idx := strings.Index(input, "v="); idx >= 0 {
		id := input[idx+2:]
		if ampIdx := strings.Index(id, "&"); ampIdx >= 0 {
			id = id[:ampIdx]
		}
		return id
	}
	// Try youtu.be short URL.
	if strings.Contains(input, "youtu.be/") {
		parts := strings.SplitN(input, "youtu.be/", 2)
		if len(parts) == 2 {
			id := parts[1]
			if qIdx := strings.Index(id, "?"); qIdx >= 0 {
				id = id[:qIdx]
			}
			return id
		}
	}
	return input
}

func init() {
	rootCmd.AddCommand(videoCmd)
}
