package pipeline

import (
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kouko/youtube-summarize-scraper/config"
	"github.com/kouko/youtube-summarize-scraper/embedded"
	"github.com/kouko/youtube-summarize-scraper/fetcher"
	"github.com/kouko/youtube-summarize-scraper/lang"
	"github.com/kouko/youtube-summarize-scraper/output"
	"github.com/kouko/youtube-summarize-scraper/subtitle"
	"github.com/kouko/youtube-summarize-scraper/summarizer"
	"github.com/kouko/youtube-summarize-scraper/transcriber"
)

// Pipeline wires together all processing modules.
type Pipeline struct {
	config      *config.Config
	binPaths    *embedded.BinPaths
	fetcher     *fetcher.Fetcher
	subtitle    *subtitle.Downloader
	transcriber *transcriber.Transcriber
	summarizer  summarizer.Summarizer
	force       bool
	dryRun      bool
}

// Stats aggregates processing results across videos.
type Stats struct {
	Success int
	Skipped int
	Failed  int
	Errors  []VideoError
}

// VideoError records a per-video processing failure.
type VideoError struct {
	VideoID string
	Title   string
	Err     error
}

// NewPipeline extracts embedded binaries and initializes all processing modules.
// If summarizer creation fails (e.g., bad provider), it logs a warning and continues
// without summarization support.
func NewPipeline(cfg *config.Config, force, dryRun bool) (*Pipeline, error) {
	binPaths, err := embedded.ExtractAll()
	if err != nil {
		return nil, fmt.Errorf("extracting embedded binaries: %w", err)
	}

	f := fetcher.NewFetcher(binPaths.YtDlp, cfg.Cookie)
	sub := subtitle.NewDownloader(binPaths.YtDlp, binPaths.FFmpeg)
	trans := transcriber.NewTranscriber(binPaths.WhisperCLI, binPaths.FFmpeg, binPaths.YtDlp, cfg.Whisper)

	var sum summarizer.Summarizer
	sum, err = summarizer.NewSummarizer(cfg.LLM)
	if err != nil {
		slog.Warn("summarizer initialization failed, summaries will be skipped", "error", err)
	}

	return &Pipeline{
		config:      cfg,
		binPaths:    binPaths,
		fetcher:     f,
		subtitle:    sub,
		transcriber: trans,
		summarizer:  sum,
		force:       force,
		dryRun:      dryRun,
	}, nil
}

// ProcessBatch iterates all playlists and channels from config, processes each, and aggregates stats.
// Playlists are processed first, then channels.
func (p *Pipeline) ProcessBatch() (*Stats, error) {
	total := &Stats{}

	// --- Process playlists first ---
	playlists := make([]config.PlaylistConfig, len(p.config.Playlists))
	copy(playlists, p.config.Playlists)

	if p.config.Batch.RandomOrder && len(playlists) > 1 {
		rand.Shuffle(len(playlists), func(i, j int) {
			playlists[i], playlists[j] = playlists[j], playlists[i]
		})
		slog.Info("shuffled playlist order")
	}

	for i, pl := range playlists {
		count := pl.Count
		if count <= 0 {
			count = p.config.DefaultCount
		}
		slog.Info("processing playlist", "url", pl.URL, "count", count)

		stats, err := p.ProcessPlaylist(pl.URL, count, &pl)
		if err != nil {
			slog.Error("playlist processing failed", "url", pl.URL, "error", err)
			continue
		}

		total.Success += stats.Success
		total.Skipped += stats.Skipped
		total.Failed += stats.Failed
		total.Errors = append(total.Errors, stats.Errors...)

		// Random delay between playlists (except after the last one before channels).
		hasMore := i < len(playlists)-1 || len(p.config.Channels) > 0
		if hasMore && p.config.Batch.DelayMax > 0 {
			p.randomDelay()
		}
	}

	// --- Process channels ---
	// Copy channels slice to avoid mutating config.
	channels := make([]config.ChannelConfig, len(p.config.Channels))
	copy(channels, p.config.Channels)

	// Shuffle channel order if configured.
	if p.config.Batch.RandomOrder && len(channels) > 1 {
		rand.Shuffle(len(channels), func(i, j int) {
			channels[i], channels[j] = channels[j], channels[i]
		})
		slog.Info("shuffled channel order")
	}

	for i, ch := range channels {
		count := p.config.EffectiveCount(ch)
		slog.Info("processing channel", "url", ch.URL, "count", count)

		stats, err := p.ProcessChannel(ch.URL, count, &ch)
		if err != nil {
			slog.Error("channel processing failed", "url", ch.URL, "error", err)
			continue
		}

		total.Success += stats.Success
		total.Skipped += stats.Skipped
		total.Failed += stats.Failed
		total.Errors = append(total.Errors, stats.Errors...)

		// Random delay between channels (except after the last one).
		if i < len(channels)-1 && p.config.Batch.DelayMax > 0 {
			p.randomDelay()
		}
	}

	// Generate Obsidian MOC files for each channel if enabled.
	if p.config.Obsidian.Enabled && p.config.Obsidian.GenerateMOC {
		for _, ch := range p.config.Channels {
			handle := strings.TrimPrefix(ch.URL, "https://www.youtube.com/")
			handle = strings.TrimPrefix(handle, "@")
			// Extract handle from URL variants (e.g., /channel/..., /@handle, etc.)
			if idx := strings.LastIndex(handle, "/"); idx >= 0 {
				handle = handle[idx+1:]
			}
			handle = strings.TrimPrefix(handle, "@")
			if handle != "" {
				if err := output.GenerateChannelMOC(handle, p.config.OutputDir); err != nil {
					slog.Warn("failed to generate MOC", "channel", handle, "error", err)
				} else {
					slog.Info("generated MOC", "channel", handle)
				}
			}
		}
	}

	slog.Info("batch complete",
		"success", total.Success,
		"skipped", total.Skipped,
		"failed", total.Failed,
	)

	return total, nil
}

// ProcessChannel fetches videos from a channel, applies filters, and processes each video.
func (p *Pipeline) ProcessChannel(channelURL string, count int, channelCfg *config.ChannelConfig) (*Stats, error) {
	// Determine which channel tabs to fetch based on filter types.
	filterCfg := p.config.EffectiveFilter(*channelCfg)
	tabSuffixes := fetcher.ChannelTabSuffixes(filterCfg.Types)

	// Small buffer for duration/other filters within the same type.
	fetchLimit := count + 5

	videos, err := p.fetcher.FetchChannelVideos(channelURL, fetchLimit, tabSuffixes)
	if err != nil {
		return nil, fmt.Errorf("fetching channel videos: %w", err)
	}

	slog.Info("fetched channel videos", "url", channelURL, "total", len(videos))

	// Apply filter (duration, etc. — type filtering already done via tab selection).
	filtered := fetcher.FilterVideos(videos, filterCfg)
	slog.Info("filtered videos", "before", len(videos), "after", len(filtered))

	// Take first N.
	if len(filtered) > count {
		filtered = filtered[:count]
	}

	stats := &Stats{}
	for i, meta := range filtered {
		// Smart skip check before fetching full metadata.
		if !p.force {
			existingDir := output.FindVideoDir(p.config.OutputDir, meta.ID)
			if existingDir != "" {
				// Determine expected target directory for this video in the channel context.
				channelHandle := deriveChannelHandle(&meta)
				targetDir := output.VideoDir(p.config.OutputDir, channelHandle, meta.UploadDate, meta.ID, meta.Title)
				if existingDir == targetDir {
					// Same dir: skip if complete, resume if partial.
					if output.HasFile(existingDir, "summary.md") {
						slog.Info(fmt.Sprintf("[%d/%d] %s - skipped (complete)", i+1, len(filtered), meta.ID),
							"title", meta.Title,
						)
						stats.Skipped++
						continue
					}
					slog.Info(fmt.Sprintf("[%d/%d] %s - resuming (missing summary)", i+1, len(filtered), meta.ID),
						"title", meta.Title,
					)
				} else if output.HasFile(existingDir, "summary.md") {
					// Cross-dir: reuse source subdirectory name (preserves date).
					srcSubDir := filepath.Base(existingDir)
					channelDir := filepath.Join(p.config.OutputDir, "@"+channelHandle)
					copyDst := filepath.Join(channelDir, srcSubDir)
					slog.Info(fmt.Sprintf("[%d/%d] %s - copying from existing dir", i+1, len(filtered), meta.ID),
						"title", meta.Title,
						"src", existingDir,
						"dst", copyDst,
					)
					if err := output.CopyVideoDir(existingDir, copyDst, "", ""); err != nil {
						slog.Warn("cross-dir copy failed, will reprocess",
							"video_id", meta.ID, "error", err)
					} else {
						stats.Skipped++
						continue
					}
				}
				// Cross-dir without summary: treat as new.
			}
		}

		slog.Info(fmt.Sprintf("[%d/%d] %s - processing", i+1, len(filtered), meta.ID),
			"title", meta.Title,
		)

		metaCopy := meta
		if err := p.ProcessVideo(&metaCopy, channelCfg); err != nil {
			if IsSkipped(err) {
				stats.Skipped++
			} else {
				slog.Error("video processing failed",
					"video_id", meta.ID,
					"title", meta.Title,
					"error", err,
				)
				stats.Failed++
				stats.Errors = append(stats.Errors, VideoError{
					VideoID: meta.ID,
					Title:   meta.Title,
					Err:     err,
				})
			}
		} else {
			stats.Success++
			// Post-processing: copy_to
			if channelCfg != nil && channelCfg.CopyTo != nil {
				channelHandle := deriveChannelHandle(&metaCopy)
				vDir := output.FindVideoDir(p.config.OutputDir, metaCopy.ID)
				if vDir == "" {
					vDir = output.VideoDir(p.config.OutputDir, channelHandle, metaCopy.UploadDate, metaCopy.ID, metaCopy.Title)
				}
				fp := output.VideoFilePrefix(metaCopy.UploadDate, metaCopy.ID)
				p.executeCopyTo(channelCfg.CopyTo, vDir, fp, &metaCopy, channelHandle, "", "")
			}
		}
	}

	return stats, nil
}

// ProcessVideo is the core pipeline for a single video.
func (p *Pipeline) ProcessVideo(meta *fetcher.VideoMeta, channelCfg *config.ChannelConfig) error {
	// 1. Fetch full metadata if we only have partial data (e.g., from ytss video command).
	if meta.Tags == nil {
		slog.Debug("fetching full metadata", "video_id", meta.ID)
		videoURL := fmt.Sprintf("https://www.youtube.com/watch?v=%s", meta.ID)
		fullMeta, err := p.fetcher.FetchVideoMeta(videoURL)
		if err != nil {
			slog.Warn("failed to fetch full metadata, continuing with partial data",
				"video_id", meta.ID, "error", err)
		} else {
			*meta = *fullMeta
		}
	}

	// 2. Determine channel handle (must be after metadata fetch).
	channelHandle := deriveChannelHandle(meta)

	// 3. Check if already processed (unless force).
	if !p.force {
		processed, err := output.IsProcessed(p.config.OutputDir, channelHandle, meta.ID)
		if err != nil {
			return fmt.Errorf("checking processed state: %w", err)
		}
		if processed {
			slog.Info("skipped (already processed)", "video_id", meta.ID)
			return errSkipped
		}
	}

	// 4. Dry run: log and return.
	if p.dryRun {
		slog.Info("would process (dry run)", "video_id", meta.ID, "title", meta.Title)
		return errSkipped
	}

	// 5. Check for resume: existing dir with transcription but no summary.
	videoDir := output.VideoDir(p.config.OutputDir, channelHandle, meta.UploadDate, meta.ID, meta.Title)
	existingDir := output.FindVideoDir(p.config.OutputDir, meta.ID)
	if existingDir != "" {
		videoDir = existingDir // reuse existing dir (may have different title sanitization)
	}

	if err := output.EnsureDir(videoDir); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	// 6. Build file prefix.
	filePrefix := output.VideoFilePrefix(meta.UploadDate, meta.ID)

	// 6.5. Resume path: if transcription exists but summary doesn't, skip to summarization.
	if existingDir != "" && output.HasFile(videoDir, "transcription.md") && !output.HasFile(videoDir, "summary.md") {
		slog.Info("resuming: reading existing transcription", "video_id", meta.ID)
		transcriptionFiles, _ := filepath.Glob(filepath.Join(videoDir, "*__transcription.md"))
		if len(transcriptionFiles) > 0 {
			data, err := os.ReadFile(transcriptionFiles[0])
			if err == nil {
				// Extract text after frontmatter (after second "---").
				content := string(data)
				if parts := strings.SplitN(content, "---\n", 3); len(parts) == 3 {
					transcriptText := strings.TrimSpace(parts[2])
					if p.summarizer != nil {
						subLang := resolveVideoLanguage(meta)
						if err := p.runSummarization(meta, channelCfg, channelHandle, videoDir, filePrefix, transcriptText, subLang, "resumed", processedAtNow()); err != nil {
							slog.Warn("summarization failed on resume", "video_id", meta.ID, "error", err)
						}
					}
					slog.Info("resume complete", "video_id", meta.ID)
					return nil
				}
			}
		}
		slog.Warn("resume failed, falling through to full processing", "video_id", meta.ID)
	}

	// 7. Build cookie args.
	cookieArgs := buildCookieArgs(p.config.Cookie)

	// 7.5. Resolve video language (4-tier fallback).
	resolvedLang := resolveVideoLanguage(meta)
	if resolvedLang != "" {
		slog.Debug("resolved video language", "video_id", meta.ID, "language", resolvedLang)
	}

	// 8. Attempt subtitle download (4-step cascade).
	var srtContent string
	var subLang string
	var subType string
	var whisperModel string

	subLangs := p.effectiveSubtitleLanguages(resolvedLang)
	subResult, err := p.subtitle.Download(
		videoURL(meta.ID),
		subLangs,
		videoDir,
		filePrefix,
		cookieArgs,
	)
	if err != nil {
		slog.Info("subtitle download failed, attempting whisper transcription",
			"video_id", meta.ID, "error", err)

		// 9. Attempt whisper transcription (use resolved language).
		transResult, transErr := p.transcriber.Transcribe(
			videoURL(meta.ID),
			resolvedLang,
			videoDir,
			filePrefix,
			cookieArgs,
		)
		if transErr != nil {
			return fmt.Errorf("both subtitle and transcription failed: subtitle=%w, transcription=%v", err, transErr)
		}

		srtContent = transResult.SRTContent
		subLang = transResult.Language
		subType = "whisper"
		whisperModel = transResult.ModelUsed
		slog.Info("whisper transcription succeeded",
			"video_id", meta.ID,
			"language", subLang,
			"model", transResult.ModelUsed,
		)
	} else {
		srtContent = subResult.Content
		subLang = subResult.Language
		subType = subResult.SubtitleType
		slog.Info("subtitle download succeeded",
			"video_id", meta.ID,
			"language", subLang,
			"type", subType,
		)
	}

	processedAt := time.Now().Format(time.RFC3339)

	// 10. Write subtitle.srt file.
	srtPath := filepath.Join(videoDir, filePrefix+"subtitle.srt")
	if err := os.WriteFile(srtPath, []byte(srtContent), 0o644); err != nil {
		return fmt.Errorf("writing SRT file: %w", err)
	}

	// 11. Convert SRT to text and write transcription.md.
	transcriptText := subtitle.SRTToText(srtContent)
	fmData := buildFrontmatterData(meta, channelHandle, subLang, subType, processedAt)
	fmData.WhisperModel = whisperModel

	// Enrich tags for Obsidian if enabled.
	if p.config.Obsidian.Enabled {
		fmData.Tags = output.EnrichTagsForObsidian(
			fmData.Tags, nil, meta.ChannelName, p.config.Obsidian.AutoTags,
		)
	}

	transcriptionFM := output.BuildTranscriptionFrontmatter(fmData)

	transcriptionPath := filepath.Join(videoDir, filePrefix+"transcription.md")
	transcriptionContent := transcriptionFM + "\n" + transcriptText + "\n"
	if err := os.WriteFile(transcriptionPath, []byte(transcriptionContent), 0o644); err != nil {
		return fmt.Errorf("writing transcription file: %w", err)
	}

	// 12. Summarization (if summarizer is available).
	if p.summarizer != nil {
		if err := p.runSummarization(meta, channelCfg, channelHandle, videoDir, filePrefix, transcriptText, subLang, subType, processedAt); err != nil {
			slog.Warn("summarization failed, transcription still produced",
				"video_id", meta.ID, "error", err)
		}
	}

	slog.Info("video processing complete", "video_id", meta.ID, "output_dir", videoDir)
	return nil
}

// runSummarization handles the multi-stage LLM summarization pipeline.
func (p *Pipeline) runSummarization(
	meta *fetcher.VideoMeta,
	channelCfg *config.ChannelConfig,
	channelHandle string,
	videoDir string,
	filePrefix string,
	transcriptText string,
	subLang string,
	subType string,
	processedAt string,
) error {
	// Resolve prompt template.
	promptTemplate, err := summarizer.ResolvePrompt(p.config.Summary, channelCfg)
	if err != nil {
		return fmt.Errorf("resolving prompt template: %w", err)
	}

	// Substitute variables.
	vars := summarizer.PromptVars{
		Title:               meta.Title,
		ChannelName:         meta.ChannelName,
		Language:            p.config.Summary.Language,
		UploadDate:          meta.UploadDate,
		Duration:            meta.DurationString,
		Tags:                strings.Join(meta.Tags, ", "),
		Transcript:          transcriptText,
		TranscriptionLength: len([]rune(transcriptText)),
	}

	prompt := summarizer.SubstituteVars(promptTemplate, vars)

	opts := summarizer.SummarizeOptions{
		Prompt:    prompt,
		MaxTokens: p.config.Summary.MaxTokens,
		Model:     p.llmModel(),
	}

	// Stage 1: Main summary.
	slog.Info("stage 1: generating summary", "video_id", meta.ID)
	summaryText, err := p.summarizer.Summarize(transcriptText, opts)
	if err != nil {
		return fmt.Errorf("stage 1 summarization: %w", err)
	}
	if strings.TrimSpace(summaryText) == "" {
		return fmt.Errorf("stage 1: LLM returned empty response — if using a thinking model (e.g., Qwen3.5), ensure think mode is disabled or increase max_tokens")
	}
	slog.Debug("stage 1 complete", "video_id", meta.ID, "response_length", len(summaryText))

	// Stage 2: Keywords (non-blocking).
	var keywords []string
	if p.config.Summary.Keywords.Enabled {
		slog.Info("stage 2: extracting keywords", "video_id", meta.ID)
		kwPrompt := summarizer.KeywordPrompt(
			summaryText,
			p.config.Summary.Keywords.Language,
			p.config.Summary.Keywords.Count,
		)
		kwOpts := summarizer.SummarizeOptions{
			Prompt:    kwPrompt,
			MaxTokens: p.config.Summary.MaxTokens,
			Model:     p.llmModel(),
		}
		kwResponse, kwErr := p.summarizer.Summarize(summaryText, kwOpts)
		if kwErr != nil {
			slog.Warn("stage 2 keyword extraction failed", "video_id", meta.ID, "error", kwErr)
		} else {
			keywords = summarizer.ParseKeywords(kwResponse)
		}
	}

	// Stage 3: Mermaid diagram (non-blocking).
	var mermaidBlock string
	if p.config.Summary.Mermaid.Enabled {
		slog.Info("stage 3: generating mermaid diagram", "video_id", meta.ID)
		mermaidPrompt := summarizer.MermaidPrompt(summaryText, p.config.Summary.Language)
		mermaidOpts := summarizer.SummarizeOptions{
			Prompt:    mermaidPrompt,
			MaxTokens: p.config.Summary.MaxTokens,
			Model:     p.llmModel(),
		}
		mermaidResponse, mermaidErr := p.summarizer.Summarize(summaryText, mermaidOpts)
		if mermaidErr != nil {
			slog.Warn("stage 3 mermaid generation failed", "video_id", meta.ID, "error", mermaidErr)
		} else {
			validated, ok := summarizer.ValidateMermaid(mermaidResponse)
			if ok {
				mermaidBlock = validated
			} else {
				slog.Warn("stage 3 mermaid validation failed", "video_id", meta.ID)
			}
		}
	}

	// Build summary frontmatter.
	fmData := buildFrontmatterData(meta, channelHandle, subLang, subType, processedAt)
	fmData.Keywords = keywords
	fmData.LLMProvider = p.config.LLM.Provider
	fmData.LLMModel = p.llmModel()

	// Enrich tags for Obsidian if enabled.
	if p.config.Obsidian.Enabled {
		fmData.Tags = output.EnrichTagsForObsidian(
			fmData.Tags, keywords, meta.ChannelName, p.config.Obsidian.AutoTags,
		)
	}

	summaryFM := output.BuildSummaryFrontmatter(fmData)

	// Assemble summary.md content.
	summaryBody := summaryText
	if mermaidBlock != "" {
		summaryBody = insertMermaidAfterFirstHeading(summaryBody, mermaidBlock)
	}

	summaryPath := filepath.Join(videoDir, filePrefix+"summary.md")
	summaryContent := summaryFM + "\n" + summaryBody + "\n"

	if err := os.WriteFile(summaryPath, []byte(summaryContent), 0o644); err != nil {
		return fmt.Errorf("writing summary file: %w", err)
	}

	slog.Info("summary written", "video_id", meta.ID, "path", summaryPath)
	return nil
}

// ProcessPlaylist fetches videos from a playlist, and processes each video.
// Output is organized under PlaylistDir with per-video subdirectories.
func (p *Pipeline) ProcessPlaylist(playlistURL string, count int, playlistCfg *config.PlaylistConfig) (*Stats, error) {
	// Extract playlist ID from URL.
	playlistID := extractPlaylistID(playlistURL)
	if playlistID == "" {
		return nil, fmt.Errorf("could not extract playlist ID from URL: %s", playlistURL)
	}

	// Build per-playlist cookie args.
	cookieArgs := p.resolveCookieArgs(playlistCfg.Cookie)

	videos, autoTitle, err := p.fetcher.FetchPlaylistVideos(playlistURL, count, cookieArgs)
	if err != nil {
		// Retry with global cookies if per-playlist cookies failed or were empty.
		globalArgs := p.globalCookieArgs()
		if len(globalArgs) > 0 && !cookieArgsEqual(cookieArgs, globalArgs) {
			slog.Info("retrying playlist fetch with global cookies", "url", playlistURL)
			videos, autoTitle, err = p.fetcher.FetchPlaylistVideos(playlistURL, count, globalArgs)
		}
		if err != nil {
			return nil, fmt.Errorf("fetching playlist videos: %w", err)
		}
	}

	// Resolve playlist name: config override > yt-dlp auto-detected > fallback.
	playlistName := playlistCfg.Name
	if playlistName == "" {
		playlistName = autoTitle
	}
	if playlistName == "" {
		playlistName = playlistID
	}

	slog.Info("fetched playlist videos",
		"url", playlistURL,
		"name", playlistName,
		"total", len(videos),
	)

	if len(videos) > count {
		videos = videos[:count]
	}

	// Determine playlist output directory.
	plDir := output.PlaylistDir(p.config.OutputDir, playlistID, playlistName)

	stats := &Stats{}
	for i, meta := range videos {
		// Build target dir: playlistDir/YYYY-MM-DD__videoID__title
		channelHandle := deriveChannelHandle(&meta)
		targetDir := filepath.Join(plDir, fmt.Sprintf("%s__%s__%s",
			formatDatePipeline(meta.UploadDate), meta.ID,
			output.SanitizeTitle(meta.Title, 0)))

		// Smart skip with cross-directory copy support.
		if !p.force {
			existingDir := output.FindVideoDir(p.config.OutputDir, meta.ID)
			if existingDir != "" {
				if existingDir == targetDir {
					if output.HasFile(existingDir, "summary.md") {
						slog.Info(fmt.Sprintf("[%d/%d] %s - skipped (complete)", i+1, len(videos), meta.ID),
							"title", meta.Title,
						)
						stats.Skipped++
						continue
					}
					slog.Info(fmt.Sprintf("[%d/%d] %s - resuming (missing summary)", i+1, len(videos), meta.ID),
						"title", meta.Title,
					)
				} else if output.HasFile(existingDir, "summary.md") {
					// Cross-dir: reuse source subdirectory name (preserves date from full metadata).
					srcSubDir := filepath.Base(existingDir)
					copyDst := filepath.Join(plDir, srcSubDir)
					slog.Info(fmt.Sprintf("[%d/%d] %s - copying from existing dir", i+1, len(videos), meta.ID),
						"title", meta.Title,
						"src", existingDir,
						"dst", copyDst,
					)
					if err := output.CopyVideoDir(existingDir, copyDst, playlistName, playlistID); err != nil {
						slog.Warn("cross-dir copy failed, will reprocess",
							"video_id", meta.ID, "error", err)
					} else {
						stats.Skipped++
						continue
					}
				}
				// Cross-dir without summary: treat as new.
			}
		}

		slog.Info(fmt.Sprintf("[%d/%d] %s - processing", i+1, len(videos), meta.ID),
			"title", meta.Title,
		)

		// Fetch full metadata.
		metaCopy := meta
		if metaCopy.Tags == nil {
			slog.Debug("fetching full metadata", "video_id", metaCopy.ID)
			fullMeta, err := p.fetcher.FetchVideoMeta(videoURL(metaCopy.ID))
			if err != nil {
				slog.Warn("failed to fetch full metadata, continuing with partial data",
					"video_id", metaCopy.ID, "error", err)
			} else {
				metaCopy = *fullMeta
			}
		}

		// Update channel handle after full metadata fetch.
		channelHandle = deriveChannelHandle(&metaCopy)

		// Recalculate targetDir with potentially updated metadata.
		targetDir = filepath.Join(plDir, fmt.Sprintf("%s__%s__%s",
			formatDatePipeline(metaCopy.UploadDate), metaCopy.ID,
			output.SanitizeTitle(metaCopy.Title, 0)))

		if err := output.EnsureDir(targetDir); err != nil {
			slog.Error("creating output directory failed", "error", err)
			stats.Failed++
			stats.Errors = append(stats.Errors, VideoError{
				VideoID: metaCopy.ID, Title: metaCopy.Title, Err: err,
			})
			continue
		}

		// Process the video using the playlist-aware wrapper.
		if err := p.processVideoInPlaylist(&metaCopy, channelHandle, targetDir, playlistName, playlistID, playlistCfg); err != nil {
			if IsSkipped(err) {
				stats.Skipped++
			} else {
				slog.Error("video processing failed",
					"video_id", metaCopy.ID,
					"title", metaCopy.Title,
					"error", err,
				)
				stats.Failed++
				stats.Errors = append(stats.Errors, VideoError{
					VideoID: metaCopy.ID,
					Title:   metaCopy.Title,
					Err:     err,
				})
			}
		} else {
			stats.Success++
			// Post-processing: copy_to
			if playlistCfg != nil && playlistCfg.CopyTo != nil {
				fp := output.VideoFilePrefix(metaCopy.UploadDate, metaCopy.ID)
				p.executeCopyTo(playlistCfg.CopyTo, targetDir, fp, &metaCopy, channelHandle, playlistName, playlistID)
			}
		}
	}

	return stats, nil
}

// processVideoInPlaylist runs the core video pipeline for a playlist context.
// It mirrors ProcessVideo but writes to the playlist-specific targetDir with
// playlist metadata in frontmatter.
func (p *Pipeline) processVideoInPlaylist(
	meta *fetcher.VideoMeta,
	channelHandle string,
	videoDir string,
	playlist string,
	playlistID string,
	playlistCfg *config.PlaylistConfig,
) error {
	// Dry run: log and return.
	if p.dryRun {
		slog.Info("would process (dry run)", "video_id", meta.ID, "title", meta.Title)
		return errSkipped
	}

	// Build file prefix.
	filePrefix := output.VideoFilePrefix(meta.UploadDate, meta.ID)

	// Resume path: if transcription exists but summary doesn't, skip to summarization.
	if output.HasFile(videoDir, "transcription.md") && !output.HasFile(videoDir, "summary.md") {
		slog.Info("resuming: reading existing transcription", "video_id", meta.ID)
		transcriptionFiles, _ := filepath.Glob(filepath.Join(videoDir, "*__transcription.md"))
		if len(transcriptionFiles) > 0 {
			data, err := os.ReadFile(transcriptionFiles[0])
			if err == nil {
				content := string(data)
				if parts := strings.SplitN(content, "---\n", 3); len(parts) == 3 {
					transcriptText := strings.TrimSpace(parts[2])
					if p.summarizer != nil {
						subLang := resolveVideoLanguage(meta)
						channelCfg := playlistToChannelCfg(playlistCfg)
						if err := p.runSummarizationPlaylist(meta, channelCfg, channelHandle, videoDir, filePrefix, transcriptText, subLang, "resumed", processedAtNow(), playlist, playlistID); err != nil {
							slog.Warn("summarization failed on resume", "video_id", meta.ID, "error", err)
						}
					}
					slog.Info("resume complete", "video_id", meta.ID)
					return nil
				}
			}
		}
		slog.Warn("resume failed, falling through to full processing", "video_id", meta.ID)
	}

	// Build cookie args: per-playlist > global.
	cookieArgs := p.resolveCookieArgs(playlistCfg.Cookie)
	if len(cookieArgs) == 0 {
		cookieArgs = p.globalCookieArgs()
	}

	// Resolve video language.
	resolvedLang := resolveVideoLanguage(meta)
	if resolvedLang != "" {
		slog.Debug("resolved video language", "video_id", meta.ID, "language", resolvedLang)
	}

	// Attempt subtitle download.
	var srtContent string
	var subLang string
	var subType string
	var whisperModel string

	subLangs := p.effectiveSubtitleLanguages(resolvedLang)
	subResult, err := p.subtitle.Download(
		videoURL(meta.ID),
		subLangs,
		videoDir,
		filePrefix,
		cookieArgs,
	)
	if err != nil {
		slog.Info("subtitle download failed, attempting whisper transcription",
			"video_id", meta.ID, "error", err)

		transResult, transErr := p.transcriber.Transcribe(
			videoURL(meta.ID),
			resolvedLang,
			videoDir,
			filePrefix,
			cookieArgs,
		)
		if transErr != nil {
			return fmt.Errorf("both subtitle and transcription failed: subtitle=%w, transcription=%v", err, transErr)
		}

		srtContent = transResult.SRTContent
		subLang = transResult.Language
		subType = "whisper"
		whisperModel = transResult.ModelUsed
		slog.Info("whisper transcription succeeded",
			"video_id", meta.ID,
			"language", subLang,
			"model", transResult.ModelUsed,
		)
	} else {
		srtContent = subResult.Content
		subLang = subResult.Language
		subType = subResult.SubtitleType
		slog.Info("subtitle download succeeded",
			"video_id", meta.ID,
			"language", subLang,
			"type", subType,
		)
	}

	processedAt := time.Now().Format(time.RFC3339)

	// Write subtitle.srt file.
	srtPath := filepath.Join(videoDir, filePrefix+"subtitle.srt")
	if err := os.WriteFile(srtPath, []byte(srtContent), 0o644); err != nil {
		return fmt.Errorf("writing SRT file: %w", err)
	}

	// Convert SRT to text and write transcription.md.
	transcriptText := subtitle.SRTToText(srtContent)
	fmData := buildFrontmatterData(meta, channelHandle, subLang, subType, processedAt)
	fmData.WhisperModel = whisperModel
	fmData.Playlist = playlist
	fmData.PlaylistID = playlistID

	if p.config.Obsidian.Enabled {
		fmData.Tags = output.EnrichTagsForObsidian(
			fmData.Tags, nil, meta.ChannelName, p.config.Obsidian.AutoTags,
		)
	}

	transcriptionFM := output.BuildTranscriptionFrontmatter(fmData)
	transcriptionPath := filepath.Join(videoDir, filePrefix+"transcription.md")
	transcriptionContent := transcriptionFM + "\n" + transcriptText + "\n"
	if err := os.WriteFile(transcriptionPath, []byte(transcriptionContent), 0o644); err != nil {
		return fmt.Errorf("writing transcription file: %w", err)
	}

	// Summarization.
	if p.summarizer != nil {
		channelCfg := playlistToChannelCfg(playlistCfg)
		if err := p.runSummarizationPlaylist(meta, channelCfg, channelHandle, videoDir, filePrefix, transcriptText, subLang, subType, processedAt, playlist, playlistID); err != nil {
			slog.Warn("summarization failed, transcription still produced",
				"video_id", meta.ID, "error", err)
		}
	}

	slog.Info("video processing complete", "video_id", meta.ID, "output_dir", videoDir)
	return nil
}

// runSummarizationPlaylist wraps runSummarization and injects playlist metadata
// into the summary frontmatter.
func (p *Pipeline) runSummarizationPlaylist(
	meta *fetcher.VideoMeta,
	channelCfg *config.ChannelConfig,
	channelHandle string,
	videoDir string,
	filePrefix string,
	transcriptText string,
	subLang string,
	subType string,
	processedAt string,
	playlist string,
	playlistID string,
) error {
	// Resolve prompt template.
	promptTemplate, err := summarizer.ResolvePrompt(p.config.Summary, channelCfg)
	if err != nil {
		return fmt.Errorf("resolving prompt template: %w", err)
	}

	vars := summarizer.PromptVars{
		Title:               meta.Title,
		ChannelName:         meta.ChannelName,
		Language:            p.config.Summary.Language,
		UploadDate:          meta.UploadDate,
		Duration:            meta.DurationString,
		Tags:                strings.Join(meta.Tags, ", "),
		Transcript:          transcriptText,
		TranscriptionLength: len([]rune(transcriptText)),
	}

	prompt := summarizer.SubstituteVars(promptTemplate, vars)

	opts := summarizer.SummarizeOptions{
		Prompt:    prompt,
		MaxTokens: p.config.Summary.MaxTokens,
		Model:     p.llmModel(),
	}

	slog.Info("stage 1: generating summary", "video_id", meta.ID)
	summaryText, err := p.summarizer.Summarize(transcriptText, opts)
	if err != nil {
		return fmt.Errorf("stage 1 summarization: %w", err)
	}
	if strings.TrimSpace(summaryText) == "" {
		return fmt.Errorf("stage 1: LLM returned empty response")
	}

	// Stage 2: Keywords.
	var keywords []string
	if p.config.Summary.Keywords.Enabled {
		slog.Info("stage 2: extracting keywords", "video_id", meta.ID)
		kwPrompt := summarizer.KeywordPrompt(
			summaryText,
			p.config.Summary.Keywords.Language,
			p.config.Summary.Keywords.Count,
		)
		kwOpts := summarizer.SummarizeOptions{
			Prompt:    kwPrompt,
			MaxTokens: p.config.Summary.MaxTokens,
			Model:     p.llmModel(),
		}
		kwResponse, kwErr := p.summarizer.Summarize(summaryText, kwOpts)
		if kwErr != nil {
			slog.Warn("stage 2 keyword extraction failed", "video_id", meta.ID, "error", kwErr)
		} else {
			keywords = summarizer.ParseKeywords(kwResponse)
		}
	}

	// Stage 3: Mermaid diagram.
	var mermaidBlock string
	if p.config.Summary.Mermaid.Enabled {
		slog.Info("stage 3: generating mermaid diagram", "video_id", meta.ID)
		mermaidPrompt := summarizer.MermaidPrompt(summaryText, p.config.Summary.Language)
		mermaidOpts := summarizer.SummarizeOptions{
			Prompt:    mermaidPrompt,
			MaxTokens: p.config.Summary.MaxTokens,
			Model:     p.llmModel(),
		}
		mermaidResponse, mermaidErr := p.summarizer.Summarize(summaryText, mermaidOpts)
		if mermaidErr != nil {
			slog.Warn("stage 3 mermaid generation failed", "video_id", meta.ID, "error", mermaidErr)
		} else {
			validated, ok := summarizer.ValidateMermaid(mermaidResponse)
			if ok {
				mermaidBlock = validated
			} else {
				slog.Warn("stage 3 mermaid validation failed", "video_id", meta.ID)
			}
		}
	}

	// Build summary frontmatter with playlist fields.
	fmData := buildFrontmatterData(meta, channelHandle, subLang, subType, processedAt)
	fmData.Keywords = keywords
	fmData.LLMProvider = p.config.LLM.Provider
	fmData.LLMModel = p.llmModel()
	fmData.Playlist = playlist
	fmData.PlaylistID = playlistID

	if p.config.Obsidian.Enabled {
		fmData.Tags = output.EnrichTagsForObsidian(
			fmData.Tags, keywords, meta.ChannelName, p.config.Obsidian.AutoTags,
		)
	}

	summaryFM := output.BuildSummaryFrontmatter(fmData)

	summaryBody := summaryText
	if mermaidBlock != "" {
		summaryBody = insertMermaidAfterFirstHeading(summaryBody, mermaidBlock)
	}

	summaryPath := filepath.Join(videoDir, filePrefix+"summary.md")
	summaryContent := summaryFM + "\n" + summaryBody + "\n"

	if err := os.WriteFile(summaryPath, []byte(summaryContent), 0o644); err != nil {
		return fmt.Errorf("writing summary file: %w", err)
	}

	slog.Info("summary written", "video_id", meta.ID, "path", summaryPath)
	return nil
}

// executeCopyTo runs the copy_to post-processing step for a successfully processed video.
func (p *Pipeline) executeCopyTo(copyTo *config.CopyToConfig, videoDir, filePrefix string, meta *fetcher.VideoMeta, channelHandle, playlist, playlistID string) {
	uploadDate := formatDatePipeline(meta.UploadDate)
	if uploadDate == "" {
		uploadDate = "unknown-date"
	}
	title := meta.Title
	if title == "" {
		title = meta.ID
	}
	vars := output.CopyToVars{
		UploadDate:    uploadDate,
		VideoID:       meta.ID,
		Title:         title,
		ChannelName:   meta.ChannelName,
		ChannelHandle: channelHandle,
		PlaylistName:  playlist,
		PlaylistID:    playlistID,
	}
	if err := output.ExecuteCopyTo(*copyTo, videoDir, filePrefix, vars); err != nil {
		slog.Warn("copy_to failed", "video_id", meta.ID, "error", err)
	}
}

// resolveCookieArgs returns cookie args from a per-source CookieConfig pointer.
// Returns nil if the pointer is nil or the config is empty.
func (p *Pipeline) resolveCookieArgs(perSource *config.CookieConfig) []string {
	if perSource != nil {
		return buildCookieArgs(*perSource)
	}
	return nil
}

// globalCookieArgs returns cookie args from the global config.
func (p *Pipeline) globalCookieArgs() []string {
	return buildCookieArgs(p.config.Cookie)
}

// randomDelay applies a random delay between batch items based on config.
func (p *Pipeline) randomDelay() {
	minDelay := p.config.Batch.DelayMin
	maxDelay := p.config.Batch.DelayMax
	if minDelay > maxDelay {
		minDelay = maxDelay
	}
	delay := minDelay + rand.IntN(maxDelay-minDelay+1)
	slog.Info("waiting before next item", "delay_seconds", delay)
	time.Sleep(time.Duration(delay) * time.Second)
}

// extractPlaylistID extracts the playlist ID (list= parameter) from a YouTube URL.
func extractPlaylistID(playlistURL string) string {
	u, err := url.Parse(playlistURL)
	if err != nil {
		return ""
	}
	return u.Query().Get("list")
}

// cookieArgsEqual compares two cookie arg slices for equality.
func cookieArgsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// playlistToChannelCfg converts a PlaylistConfig to a ChannelConfig for shared
// functions that accept ChannelConfig (e.g., prompt resolution).
func playlistToChannelCfg(pl *config.PlaylistConfig) *config.ChannelConfig {
	if pl == nil {
		return nil
	}
	return &config.ChannelConfig{
		URL:               pl.URL,
		Count:             pl.Count,
		SummaryPromptFile: pl.SummaryPromptFile,
		Cookie:            pl.Cookie,
		CopyTo:            pl.CopyTo,
	}
}

// formatDatePipeline converts YYYYMMDD to YYYY-MM-DD for directory naming.
func formatDatePipeline(date string) string {
	if len(date) == 8 {
		return date[:4] + "-" + date[4:6] + "-" + date[6:8]
	}
	return date
}

// errSkipped is a sentinel used to signal that a video was skipped (not an error).
var errSkipped = fmt.Errorf("skipped")

// IsSkipped returns true if the error is the sentinel skipped error.
func IsSkipped(err error) bool {
	return err == errSkipped
}

// deriveChannelHandle extracts the channel handle from VideoMeta.
// It prefers the Channel field (which is typically "@handle").
// If Channel starts with "@", the "@" is stripped (VideoDir adds it back).
func deriveChannelHandle(meta *fetcher.VideoMeta) string {
	ch := meta.Channel
	if ch == "" {
		ch = output.SanitizeTitle(meta.ChannelName, 40)
	}
	ch = strings.TrimPrefix(ch, "@")
	if ch == "" {
		ch = "unknown"
	}
	return ch
}

// videoURL constructs a YouTube watch URL from a video ID.
func videoURL(videoID string) string {
	return fmt.Sprintf("https://www.youtube.com/watch?v=%s", videoID)
}

// buildCookieArgs constructs yt-dlp cookie arguments from config.
// Chrome profile email is resolved to directory name automatically.
func buildCookieArgs(cookie config.CookieConfig) []string {
	if cookie.File != "" {
		return []string{"--cookies", cookie.File}
	}
	if cookie.Browser != "" {
		browser := cookie.Browser
		profile := fetcher.ResolveChromeProfile(cookie.ChromeProfile)
		if profile != "" {
			browser += ":" + profile
		}
		return []string{"--cookies-from-browser", browser}
	}
	return nil
}

// llmModel returns the model name for the configured LLM provider.
func (p *Pipeline) llmModel() string {
	switch p.config.LLM.Provider {
	case "ollama":
		return p.config.LLM.Ollama.Model
	case "llamacpp":
		return "llamacpp"
	case "claude-api":
		return p.config.LLM.ClaudeAPI.Model
	case "gemini-cli":
		return p.config.LLM.GeminiCLI.Model
	default:
		return p.config.LLM.Provider
	}
}

// buildFrontmatterData creates a FrontmatterData struct from video metadata.
func buildFrontmatterData(
	meta *fetcher.VideoMeta,
	channelHandle string,
	language string,
	subtitleType string,
	processedAt string,
) output.FrontmatterData {
	return output.FrontmatterData{
		Title:        meta.Title,
		VideoID:      meta.ID,
		URL:          meta.URL,
		Channel:      "@" + channelHandle,
		ChannelName:  meta.ChannelName,
		UploadDate:   meta.UploadDate,
		Duration:     meta.DurationString,
		Language:     language,
		Tags:         meta.Tags,
		Categories:   meta.Categories,
		SubtitleType: subtitleType,
		ProcessedAt:  processedAt,
	}
}

// resolveVideoLanguage applies a 4-tier fallback to determine the video language:
//  1. yt-dlp language field
//  2. First available subtitle language (from preferred languages config)
//  3. Detect from title + description text (Unicode character analysis)
//  4. "" (unknown — whisper will auto-detect)
func resolveVideoLanguage(meta *fetcher.VideoMeta) string {
	// Tier 1: yt-dlp language field.
	normalized := lang.NormalizeToISO639_1(meta.Language)
	if normalized != "" {
		return normalized
	}

	// Tier 2: skipped here — subtitle language is resolved during download.

	// Tier 3: detect from title + description.
	textForDetection := meta.Title + " " + meta.Description
	if detected := lang.DetectLanguageFromText(textForDetection); detected != "" {
		slog.Info("language detected from title/description", "language", detected)
		return detected
	}

	// Tier 4: unknown.
	return ""
}

// effectiveSubtitleLanguages returns the language list for subtitle download.
// Prioritizes the resolved video language (from metadata/title detection).
// Falls back to preferred_languages from config if video language is unknown.
func (p *Pipeline) effectiveSubtitleLanguages(resolvedLang string) []string {
	if resolvedLang != "" {
		return []string{resolvedLang}
	}
	if len(p.config.PreferredLanguages) > 0 {
		return p.config.PreferredLanguages
	}
	return nil
}

func processedAtNow() string {
	return time.Now().Format(time.RFC3339)
}

// insertMermaidAfterFirstHeading inserts a Mermaid code block after the first
// "### " heading in the summary text. If no heading is found, it prepends the block.
func insertMermaidAfterFirstHeading(summaryText, mermaidCode string) string {
	mermaidSection := "\n\n```mermaid\n" + mermaidCode + "\n```\n"

	lines := strings.Split(summaryText, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "### ") {
			// Insert after this line.
			before := strings.Join(lines[:i+1], "\n")
			after := strings.Join(lines[i+1:], "\n")
			return before + mermaidSection + after
		}
	}

	// No heading found — prepend.
	return mermaidSection + summaryText
}
