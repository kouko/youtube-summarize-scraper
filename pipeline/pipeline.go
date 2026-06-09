package pipeline

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
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

// videoFetcher abstracts fetcher operations for testability.
type videoFetcher interface {
	FetchVideoMeta(videoURL string) (*fetcher.VideoMeta, error)
	FetchChannelTab(tabURL string, limit int) ([]fetcher.VideoMeta, error)
	FetchPlaylistVideos(playlistURL string, limit int, cookieArgs []string) ([]fetcher.VideoMeta, string, error)
}

// Pipeline wires together all processing modules.
type Pipeline struct {
	config      *config.Config
	binPaths    *embedded.BinPaths
	fetcher     videoFetcher
	subtitle    *subtitle.Downloader
	transcriber *transcriber.Transcriber
	summarizer  summarizer.Summarizer
	index       *output.VideoIndex
	timezone    *time.Location
	ctx         context.Context
	cancel      context.CancelFunc
	force       bool
	dryRun      bool
}

// Stats aggregates processing results across videos.
type Stats struct {
	Success int
	Skipped int
	// Partial counts videos whose transcription was produced but whose
	// summarization failed (e.g. all LLM providers out of quota). These are
	// not lost: a later run resumes summarization. Kept separate from Success
	// (the summary is missing) and from Failed (the transcript is usable).
	Partial int
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

	// Build in-memory index of processed videos for fast skip detection.
	start := time.Now()
	idx := output.BuildIndex(cfg.OutputDir)
	slog.Info("built video index", "duration", time.Since(start))

	ctx, cancel := context.WithCancel(context.Background())

	return &Pipeline{
		config:      cfg,
		binPaths:    binPaths,
		fetcher:     f,
		subtitle:    sub,
		transcriber: trans,
		summarizer:  sum,
		index:       idx,
		timezone:    config.LoadTimezone(cfg.Timezone),
		ctx:         ctx,
		cancel:      cancel,
		force:       force,
		dryRun:      dryRun,
	}, nil
}

// Shutdown signals the pipeline to stop processing after the current video completes.
func (p *Pipeline) Shutdown() {
	p.cancel()
}

// stopped returns true if a shutdown has been requested.
func (p *Pipeline) stopped() bool {
	select {
	case <-p.ctx.Done():
		return true
	default:
		return false
	}
}

// ResetContext creates a fresh context for a new watch iteration.
func (p *Pipeline) ResetContext() {
	p.ctx, p.cancel = context.WithCancel(context.Background())
}

// ReloadConfig re-reads the config file and updates runtime-safe fields
// (channels, playlists, filter, batch, etc.). LLM/whisper/cookie are preserved.
func (p *Pipeline) ReloadConfig(cfgPath string) {
	if err := p.config.ReloadPartial(cfgPath); err != nil {
		slog.Warn("config reload failed, keeping previous config", "error", err)
		return
	}
	slog.Info("config reloaded")
}

// RebuildIndex rebuilds the in-memory video index from the output directory.
// Call this at the start of each watch iteration to detect external changes.
func (p *Pipeline) RebuildIndex() {
	start := time.Now()
	p.index = output.BuildIndex(p.config.OutputDir)
	slog.Info("rebuilt video index", "duration", time.Since(start))
}

// ProcessBatch iterates all playlists and channels from config, processes each, and aggregates stats.
// Playlists are processed first, then channels.
// batchSource holds the result of a parallel video-list fetch for one
// playlist or channel, along with the config needed for Phase 2 processing.
type batchSource struct {
	// Shared fields.
	videos []fetcher.VideoMeta
	err    error

	// Channel source (nil for playlists).
	channelURL string
	channelCfg *config.ChannelConfig

	// Playlist source (nil for channels).
	playlistURL  string
	playlistCfg  *config.PlaylistConfig
	playlistID   string
	playlistName string
}

func (s *batchSource) isPlaylist() bool { return s.playlistCfg != nil }

func (p *Pipeline) ProcessBatch() (*Stats, error) {
	total := &Stats{}

	// --- Build source list (playlists + channels) ---
	playlists := make([]config.PlaylistConfig, len(p.config.Playlists))
	copy(playlists, p.config.Playlists)
	channels := make([]config.ChannelConfig, len(p.config.Channels))
	copy(channels, p.config.Channels)

	if p.config.Batch.RandomOrder {
		if len(playlists) > 1 {
			rand.Shuffle(len(playlists), func(i, j int) {
				playlists[i], playlists[j] = playlists[j], playlists[i]
			})
			slog.Info("shuffled playlist order")
		}
		if len(channels) > 1 {
			rand.Shuffle(len(channels), func(i, j int) {
				channels[i], channels[j] = channels[j], channels[i]
			})
			slog.Info("shuffled channel order")
		}
	}

	// Allocate result slots: playlists first, then channels.
	sources := make([]batchSource, len(playlists)+len(channels))
	for i := range playlists {
		sources[i] = batchSource{
			playlistURL: playlists[i].URL,
			playlistCfg: &playlists[i],
		}
	}
	for i := range channels {
		sources[len(playlists)+i] = batchSource{
			channelURL: channels[i].URL,
			channelCfg: &channels[i],
		}
	}

	// --- Phase 1: Parallel video-list fetching ---
	concurrency := p.config.Batch.FetchConcurrency
	if concurrency <= 0 {
		concurrency = 3
	}
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	slog.Info("phase 1: fetching video lists in parallel",
		"playlists", len(playlists),
		"channels", len(channels),
		"concurrency", concurrency,
	)

	totalSources := len(sources)
	var fetchedCount atomic.Int32

	for idx := range sources {
		if p.stopped() {
			break
		}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if p.stopped() {
				return
			}

			s := &sources[i]
			if s.isPlaylist() {
				p.fetchPlaylistList(s)
			} else {
				p.fetchChannelList(s)
			}

			n := fetchedCount.Add(1)
			if s.err == nil {
				if s.isPlaylist() {
					slog.Info(fmt.Sprintf("[%d/%d] fetched playlist", n, totalSources),
						"url", s.playlistURL, "name", s.playlistName, "videos", len(s.videos))
				} else {
					slog.Info(fmt.Sprintf("[%d/%d] fetched channel", n, totalSources),
						"url", s.channelURL, "videos", len(s.videos))
				}
			}
		}(idx)
	}
	wg.Wait()

	if p.stopped() {
		slog.Info("batch interrupted by shutdown signal")
		return total, nil
	}

	// --- Phase 2: Sequential video processing ---
	slog.Info("phase 2: processing videos sequentially")

	for i := range sources {
		if p.stopped() {
			slog.Info("batch interrupted by shutdown signal")
			return total, nil
		}

		s := &sources[i]
		if s.err != nil {
			if s.isPlaylist() {
				slog.Error("playlist fetch failed", "url", s.playlistURL, "error", s.err)
			} else {
				slog.Error("channel fetch failed", "url", s.channelURL, "error", s.err)
			}
			continue
		}

		var stats *Stats
		var err error
		if s.isPlaylist() {
			plDir := output.PlaylistDir(p.config.OutputDir, s.playlistID, s.playlistName)
			slog.Info("processing playlist", "url", s.playlistURL, "name", s.playlistName, "videos", len(s.videos))
			stats, err = p.processPlaylistVideos(s.videos, s.playlistID, s.playlistName, plDir, s.playlistCfg)
		} else {
			slog.Info("processing channel", "url", s.channelURL, "videos", len(s.videos))
			stats, err = p.processChannelVideos(s.videos, s.channelCfg)
		}
		if err != nil {
			if s.isPlaylist() {
				slog.Error("playlist processing failed", "url", s.playlistURL, "error", err)
			} else {
				slog.Error("channel processing failed", "url", s.channelURL, "error", err)
			}
			continue
		}

		total.Success += stats.Success
		total.Skipped += stats.Skipped
		total.Partial += stats.Partial
		total.Failed += stats.Failed
		total.Errors = append(total.Errors, stats.Errors...)

		// Random delay between sources (except after the last one).
		if i < len(sources)-1 && p.config.Batch.DelayMax > 0 {
			p.randomDelay()
		}
	}

	// Generate Obsidian MOC files for each channel if enabled.
	if p.config.Obsidian.Enabled && p.config.Obsidian.GenerateMOC {
		for _, ch := range p.config.Channels {
			handle := strings.TrimPrefix(ch.URL, "https://www.youtube.com/")
			handle = strings.TrimPrefix(handle, "@")
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
		"partial", total.Partial,
		"failed", total.Failed,
	)

	return total, nil
}

// fetchPlaylistList fetches the video list for a playlist source (Phase 1).
func (p *Pipeline) fetchPlaylistList(s *batchSource) {
	count := s.playlistCfg.Count
	if count <= 0 {
		count = p.config.DefaultCount
	}

	cookieArgs := p.resolveCookieArgs(s.playlistCfg.Cookie)
	videos, autoTitle, err := p.fetcher.FetchPlaylistVideos(s.playlistURL, count, cookieArgs)
	if err != nil {
		// Retry with global cookies.
		globalArgs := p.globalCookieArgs()
		if len(globalArgs) > 0 && !cookieArgsEqual(cookieArgs, globalArgs) {
			slog.Info("retrying playlist fetch with global cookies", "url", s.playlistURL)
			videos, autoTitle, err = p.fetcher.FetchPlaylistVideos(s.playlistURL, count, globalArgs)
		}
		if err != nil {
			s.err = fmt.Errorf("fetching playlist videos: %w", err)
			return
		}
	}

	// Resolve playlist name.
	name := s.playlistCfg.Name
	if name == "" {
		name = autoTitle
	}
	playlistID := extractPlaylistID(s.playlistURL)
	if name == "" {
		name = playlistID
	}

	// Apply effective filter (global merged with per-playlist override).
	videos = fetcher.FilterVideos(videos, p.config.EffectivePlaylistFilter(*s.playlistCfg))
	if len(videos) > count {
		videos = videos[:count]
	}

	s.videos = videos
	s.playlistID = playlistID
	s.playlistName = name
}

// fetchAllTabs fetches and filters videos from all configured channel tabs.
// Individual tab failures are logged as warnings and skipped; an error is
// returned only when all tabs fail.
func (p *Pipeline) fetchAllTabs(channelURL string, count int, filterCfg config.FilterConfig) ([]fetcher.VideoMeta, error) {
	tabSuffixes := fetcher.ChannelTabSuffixes(filterCfg.Types)

	var allVideos []fetcher.VideoMeta
	var tabErrors []error
	for _, suffix := range tabSuffixes {
		tabURL := channelURL + suffix
		videos, err := p.fetcher.FetchChannelTab(tabURL, count)
		if err != nil {
			slog.Warn("failed to fetch channel tab, skipping", "tab", suffix, "url", tabURL, "error", err)
			tabErrors = append(tabErrors, err)
			continue
		}
		slog.Info("fetched channel tab", "tab", suffix, "url", tabURL, "fetched", len(videos), "limit", count)

		tabFiltered := fetcher.FilterVideos(videos, filterCfg)
		if len(tabFiltered) > count {
			tabFiltered = tabFiltered[:count]
		}
		allVideos = append(allVideos, tabFiltered...)
	}

	if len(allVideos) == 0 && len(tabErrors) > 0 {
		return nil, fmt.Errorf("all channel tabs failed for %s", channelURL)
	}
	return allVideos, nil
}

// fetchChannelList fetches the video list for a channel source (Phase 1).
func (p *Pipeline) fetchChannelList(s *batchSource) {
	count := p.config.EffectiveCount(*s.channelCfg)
	filterCfg := p.config.EffectiveFilter(*s.channelCfg)
	videos, err := p.fetchAllTabs(s.channelURL, count, filterCfg)
	if err != nil {
		s.err = err
		return
	}
	s.videos = videos
}

// ProcessChannel fetches videos from a channel, applies filters, and processes each video.
func (p *Pipeline) ProcessChannel(channelURL string, count int, channelCfg *config.ChannelConfig) (*Stats, error) {
	filterCfg := p.config.EffectiveFilter(*channelCfg)
	filtered, err := p.fetchAllTabs(channelURL, count, filterCfg)
	if err != nil {
		return nil, err
	}
	slog.Info("total filtered videos across tabs", "count", len(filtered))
	return p.processChannelVideos(filtered, channelCfg)
}

// processChannelVideos processes pre-fetched channel videos sequentially.
func (p *Pipeline) processChannelVideos(videos []fetcher.VideoMeta, channelCfg *config.ChannelConfig) (*Stats, error) {
	stats := &Stats{}
	for i, meta := range videos {
		if !p.force {
			existingDir := p.index.FindVideoDir(meta.ID)
			if existingDir != "" && p.index.HasFile(meta.ID, "summary.md") {
				slog.Info(fmt.Sprintf("[%d/%d] %s - skipped (already processed)", i+1, len(videos), meta.ID),
					"title", meta.Title,
				)
				stats.Skipped++
				continue
			}
		}

		slog.Info(fmt.Sprintf("[%d/%d] %s - processing", i+1, len(videos), meta.ID),
			"title", meta.Title,
		)

		metaCopy := meta
		err := p.ProcessVideo(&metaCopy, channelCfg)
		switch classifyResult(err) {
		case bucketSkipped:
			stats.Skipped++
		case bucketPartial:
			stats.Partial++
		case bucketFailed:
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
		case bucketSuccess:
			stats.Success++
			// Post-processing: copy_to
			if channelCfg != nil && channelCfg.CopyTo != nil {
				channelHandle := deriveChannelHandle(&metaCopy)
				vDir := p.index.FindVideoDir(metaCopy.ID)
				if vDir == "" {
					convertedDate := output.ConvertUploadDate(metaCopy.UploadDate, metaCopy.Timestamp, p.timezone)
					vDir = output.VideoDir(p.config.OutputDir, channelHandle, convertedDate, metaCopy.ID, metaCopy.Title)
				}
				fp := output.VideoFilePrefix(output.ConvertUploadDate(metaCopy.UploadDate, metaCopy.Timestamp, p.timezone), metaCopy.ID)
				p.executeCopyTo(channelCfg.CopyTo, vDir, fp, &metaCopy, channelHandle, "", "")
			}
		}
	}

	return stats, nil
}

// ProcessVideo is the core pipeline for a single video.
func (p *Pipeline) ProcessVideo(meta *fetcher.VideoMeta, channelCfg *config.ChannelConfig) error {
	// 1. Fetch full metadata if we only have partial data (e.g., from ytss video command).
	// Preserve approximate date from flat-playlist as fallback.
	approxUploadDate := meta.UploadDate
	approxTimestamp := meta.Timestamp

	if meta.Tags == nil || meta.UploadDate == "" {
		slog.Debug("fetching full metadata", "video_id", meta.ID)
		videoURL := fmt.Sprintf("https://www.youtube.com/watch?v=%s", meta.ID)
		fullMeta, err := p.fetcher.FetchVideoMeta(videoURL)
		if err != nil {
			slog.Warn("failed to fetch full metadata",
				"video_id", meta.ID, "error", err)
			// Restore approximate date if available.
			meta.UploadDate = approxUploadDate
			meta.Timestamp = approxTimestamp
			// Without channel info we cannot create proper directory — skip.
			if meta.Channel == "" && meta.ChannelName == "" {
				return fmt.Errorf("cannot process %s: full metadata fetch failed and no channel info available", meta.ID)
			}
		} else {
			*meta = *fullMeta
		}
	}

	// 2. Determine channel handle (must be after metadata fetch).
	channelHandle := deriveChannelHandle(meta)

	// 3. Check if already processed (unless force).
	if !p.force {
		if p.index.IsProcessed(channelHandle, meta.ID) {
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
	convertedDate := output.ConvertUploadDate(meta.UploadDate, meta.Timestamp, p.timezone)
	videoDir := output.VideoDir(p.config.OutputDir, channelHandle, convertedDate, meta.ID, meta.Title)
	existingDir := p.index.FindVideoDir(meta.ID)
	if existingDir != "" {
		videoDir = existingDir // reuse existing dir (may have different title sanitization)
	}

	if err := output.EnsureDir(videoDir); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}
	p.index.Add(meta.ID, videoDir)

	// 6. Build file prefix.
	filePrefix := output.VideoFilePrefix(convertedDate, meta.ID)

	// 6.5. Resume path: if transcription exists but summary doesn't, skip to summarization.
	if existingDir != "" && p.index.HasFile(meta.ID, "transcription.md") && !p.index.HasFile(meta.ID, "summary.md") {
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
							return fmt.Errorf("%w: %v", errPartial, err)
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
	p.index.AddFile(meta.ID, "subtitle.srt")

	// 11. Convert SRT to text and write transcription.md.
	transcriptText := subtitle.SRTToText(srtContent)
	fmData := buildFrontmatterData(meta, channelHandle, subLang, subType, processedAt, p.timezone)
	fmData.WhisperModel = whisperModel

	transcriptionFM := output.BuildTranscriptionFrontmatter(fmData)

	transcriptionPath := filepath.Join(videoDir, filePrefix+"transcription.md")
	embedEnabled := channelCfg == nil && p.config.VideoEmbed || channelCfg != nil && p.config.EffectiveVideoEmbed(*channelCfg)
	var videoEmbed string
	if embedEnabled {
		videoEmbed = "![](" + meta.URL + ")\n\n"
	}
	transcriptionContent := transcriptionFM + videoEmbed + transcriptText + "\n"
	if err := os.WriteFile(transcriptionPath, []byte(transcriptionContent), 0o644); err != nil {
		return fmt.Errorf("writing transcription file: %w", err)
	}
	p.index.AddFile(meta.ID, "transcription.md")

	// 12. Summarization (if summarizer is available).
	if p.summarizer != nil {
		if err := p.runSummarization(meta, channelCfg, channelHandle, videoDir, filePrefix, transcriptText, subLang, subType, processedAt); err != nil {
			slog.Warn("summarization failed, transcription still produced",
				"video_id", meta.ID, "error", err)
			return fmt.Errorf("%w: %v", errPartial, err)
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
		UploadDate:          output.ConvertUploadDate(meta.UploadDate, meta.Timestamp, p.timezone),
		Duration:            meta.DurationString,
		Tags:                strings.Join(meta.Tags, ", "),
		Transcript:          transcriptText,
		TranscriptionLength: len([]rune(transcriptText)),
	}

	prompt := summarizer.SubstituteVars(promptTemplate, vars)

	opts := summarizer.SummarizeOptions{
		Prompt:    prompt,
		MaxTokens: p.config.Summary.MaxTokens,
		Model:     "", // let each provider use its own configured model
	}

	// Stage 1: Main summary.
	slog.Info("stage 1: generating summary", "video_id", meta.ID)
	summaryResult, err := p.summarizer.Summarize(transcriptText, opts)
	if err != nil {
		return fmt.Errorf("stage 1 summarization: %w", err)
	}
	summaryText := summaryResult.Text
	if strings.TrimSpace(summaryText) == "" {
		return emptyResponseError("stage 1", summaryResult.Provider, summaryResult.Model)
	}
	slog.Info("stage 1 complete", "video_id", meta.ID, "provider", summaryResult.Provider, "model", summaryResult.Model, "response_length", len(summaryText))

	// Stage 2: Keywords (non-blocking).
	var keywords []string
	if p.config.Summary.Keywords.Enabled {
		slog.Info("stage 2: extracting keywords", "video_id", meta.ID)
		kwPrompt, kwPromptErr := summarizer.KeywordPrompt(
			summaryText,
			p.config.Summary.Keywords.Language,
			p.config.Summary.Keywords.Count,
		)
		if kwPromptErr != nil {
			slog.Warn("stage 2 keyword prompt loading failed", "video_id", meta.ID, "error", kwPromptErr)
		}
		kwOpts := summarizer.SummarizeOptions{
			Prompt:    kwPrompt,
			MaxTokens: p.config.Summary.MaxTokens,
		}
		kwResult, kwErr := p.summarizer.Summarize(summaryText, kwOpts)
		if kwErr != nil {
			slog.Warn("stage 2 keyword extraction failed", "video_id", meta.ID, "error", kwErr)
		} else {
			keywords = summarizer.ParseKeywords(kwResult.Text)
		}
	}

	// Stage 3: Mermaid diagrams (non-blocking).
	var mermaidBlocks []summarizer.MermaidBlock
	if p.config.Summary.Mermaid.Enabled {
		slog.Info("stage 3: generating mermaid diagrams", "video_id", meta.ID)
		mermaidPrompt, mermaidPromptErr := summarizer.MermaidPrompt(summaryText, p.config.Summary.Language)
		if mermaidPromptErr != nil {
			slog.Warn("stage 3 mermaid prompt loading failed", "video_id", meta.ID, "error", mermaidPromptErr)
		}
		mermaidOpts := summarizer.SummarizeOptions{
			Prompt:    mermaidPrompt,
			MaxTokens: p.config.Summary.MaxTokens,
		}
		mermaidResult, mermaidErr := p.summarizer.Summarize(summaryText, mermaidOpts)
		if mermaidErr != nil {
			slog.Warn("stage 3 mermaid generation failed", "video_id", meta.ID, "error", mermaidErr)
		} else {
			mermaidBlocks = summarizer.ValidateMermaidBlocks(mermaidResult.Text)
			if len(mermaidBlocks) == 0 {
				slog.Warn("stage 3 mermaid validation failed", "video_id", meta.ID)
			} else {
				slog.Debug("stage 3 complete", "video_id", meta.ID, "diagram_count", len(mermaidBlocks))
			}
		}
	}

	// Build summary frontmatter using actual provider/model from Stage 1.
	fmData := buildFrontmatterData(meta, channelHandle, subLang, subType, processedAt, p.timezone)
	fmData.Tags = keywords
	fmData.LLMProvider = summaryResult.Provider
	fmData.LLMModel = summaryResult.Model

	// Enrich tags for Obsidian if enabled (LLM tags + channel + auto).
	if p.config.Obsidian.Enabled {
		fmData.Tags = output.EnrichTagsForObsidian(
			fmData.Tags, meta.ChannelName, p.config.Obsidian.AutoTags,
		)
	}

	summaryFM := output.BuildSummaryFrontmatter(fmData)

	// Assemble summary.md content.
	summaryBody := summaryText
	if len(mermaidBlocks) > 0 {
		summaryBody = insertMermaidBlocksAfterFirstHeading(summaryBody, mermaidBlocks)
	}

	summaryPath := filepath.Join(videoDir, filePrefix+"summary.md")
	embedEnabled := channelCfg == nil && p.config.VideoEmbed || channelCfg != nil && p.config.EffectiveVideoEmbed(*channelCfg)
	var videoEmbed string
	if embedEnabled {
		videoEmbed = "![](" + meta.URL + ")\n\n"
	}
	summaryContent := summaryFM + videoEmbed + summaryBody + "\n"

	if err := os.WriteFile(summaryPath, []byte(summaryContent), 0o644); err != nil {
		return fmt.Errorf("writing summary file: %w", err)
	}
	p.index.AddFile(meta.ID, "summary.md")

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

	// Apply effective filter (global merged with per-playlist override).
	videos = fetcher.FilterVideos(videos, p.config.EffectivePlaylistFilter(*playlistCfg))

	if len(videos) > count {
		videos = videos[:count]
	}

	// Determine playlist output directory.
	plDir := output.PlaylistDir(p.config.OutputDir, playlistID, playlistName)

	return p.processPlaylistVideos(videos, playlistID, playlistName, plDir, playlistCfg)
}

// processPlaylistVideos processes pre-fetched playlist videos sequentially.
func (p *Pipeline) processPlaylistVideos(videos []fetcher.VideoMeta, playlistID, playlistName, plDir string, playlistCfg *config.PlaylistConfig) (*Stats, error) {
	stats := &Stats{}
	for i, meta := range videos {
		// Smart skip with cross-directory copy support.
		if !p.force {
			existingDir := p.index.FindVideoDir(meta.ID)
			if existingDir != "" && p.index.HasFile(meta.ID, "summary.md") {
				if strings.HasPrefix(existingDir, plDir+string(filepath.Separator)) {
					slog.Info(fmt.Sprintf("[%d/%d] %s - skipped (complete)", i+1, len(videos), meta.ID),
						"title", meta.Title,
					)
					stats.Skipped++
					continue
				}
				// Cross-dir: copy to playlist dir using existing subdirectory name.
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
		}

		slog.Info(fmt.Sprintf("[%d/%d] %s - processing", i+1, len(videos), meta.ID),
			"title", meta.Title,
		)

		// Fetch full metadata.
		metaCopy := meta
		approxUploadDate := metaCopy.UploadDate
		approxTimestamp := metaCopy.Timestamp

		if metaCopy.Tags == nil || metaCopy.UploadDate == "" {
			slog.Debug("fetching full metadata", "video_id", metaCopy.ID)
			fullMeta, err := p.fetcher.FetchVideoMeta(videoURL(metaCopy.ID))
			if err != nil {
				slog.Warn("failed to fetch full metadata, continuing with partial data",
					"video_id", metaCopy.ID, "error", err)
				metaCopy.UploadDate = approxUploadDate
				metaCopy.Timestamp = approxTimestamp
			} else {
				metaCopy = *fullMeta
			}
		}

		channelHandle := deriveChannelHandle(&metaCopy)

		convertedDate := output.ConvertUploadDate(metaCopy.UploadDate, metaCopy.Timestamp, p.timezone)
		if convertedDate == "" {
			convertedDate = "unknown-date"
		}
		targetDir := filepath.Join(plDir, fmt.Sprintf("%s__%s__%s",
			convertedDate, metaCopy.ID,
			output.SanitizeTitle(metaCopy.Title, 0)))

		if err := output.EnsureDir(targetDir); err != nil {
			slog.Error("creating output directory failed", "error", err)
			stats.Failed++
			stats.Errors = append(stats.Errors, VideoError{
				VideoID: metaCopy.ID, Title: metaCopy.Title, Err: err,
			})
			continue
		}
		p.index.Add(metaCopy.ID, targetDir)

		err := p.processVideoInPlaylist(&metaCopy, channelHandle, targetDir, playlistName, playlistID, playlistCfg)
		switch classifyResult(err) {
		case bucketSkipped:
			stats.Skipped++
		case bucketPartial:
			stats.Partial++
		case bucketFailed:
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
		case bucketSuccess:
			stats.Success++
			if playlistCfg != nil && playlistCfg.CopyTo != nil {
				fp := output.VideoFilePrefix(convertedDate, metaCopy.ID)
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
	convertedDate := output.ConvertUploadDate(meta.UploadDate, meta.Timestamp, p.timezone)
	filePrefix := output.VideoFilePrefix(convertedDate, meta.ID)

	// Resume path: if transcription exists but summary doesn't, skip to summarization.
	if p.index.HasFile(meta.ID, "transcription.md") && !p.index.HasFile(meta.ID, "summary.md") {
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
							return fmt.Errorf("%w: %v", errPartial, err)
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
	p.index.AddFile(meta.ID, "subtitle.srt")

	// Convert SRT to text and write transcription.md.
	transcriptText := subtitle.SRTToText(srtContent)
	fmData := buildFrontmatterData(meta, channelHandle, subLang, subType, processedAt, p.timezone)
	fmData.WhisperModel = whisperModel
	fmData.Playlist = playlist
	fmData.PlaylistID = playlistID

	transcriptionFM := output.BuildTranscriptionFrontmatter(fmData)
	transcriptionPath := filepath.Join(videoDir, filePrefix+"transcription.md")
	var videoEmbed string
	if p.config.EffectivePlaylistVideoEmbed(*playlistCfg) {
		videoEmbed = "![](" + meta.URL + ")\n\n"
	}
	transcriptionContent := transcriptionFM + videoEmbed + transcriptText + "\n"
	if err := os.WriteFile(transcriptionPath, []byte(transcriptionContent), 0o644); err != nil {
		return fmt.Errorf("writing transcription file: %w", err)
	}
	p.index.AddFile(meta.ID, "transcription.md")

	// Summarization.
	if p.summarizer != nil {
		channelCfg := playlistToChannelCfg(playlistCfg)
		if err := p.runSummarizationPlaylist(meta, channelCfg, channelHandle, videoDir, filePrefix, transcriptText, subLang, subType, processedAt, playlist, playlistID); err != nil {
			slog.Warn("summarization failed, transcription still produced",
				"video_id", meta.ID, "error", err)
			return fmt.Errorf("%w: %v", errPartial, err)
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
		UploadDate:          output.ConvertUploadDate(meta.UploadDate, meta.Timestamp, p.timezone),
		Duration:            meta.DurationString,
		Tags:                strings.Join(meta.Tags, ", "),
		Transcript:          transcriptText,
		TranscriptionLength: len([]rune(transcriptText)),
	}

	prompt := summarizer.SubstituteVars(promptTemplate, vars)

	opts := summarizer.SummarizeOptions{
		Prompt:    prompt,
		MaxTokens: p.config.Summary.MaxTokens,
		Model:     "", // let each provider use its own configured model
	}

	slog.Info("stage 1: generating summary", "video_id", meta.ID)
	summaryResult, err := p.summarizer.Summarize(transcriptText, opts)
	if err != nil {
		return fmt.Errorf("stage 1 summarization: %w", err)
	}
	summaryText := summaryResult.Text
	if strings.TrimSpace(summaryText) == "" {
		return emptyResponseError("stage 1", summaryResult.Provider, summaryResult.Model)
	}
	slog.Info("stage 1 complete", "video_id", meta.ID, "provider", summaryResult.Provider, "model", summaryResult.Model, "response_length", len(summaryText))

	// Stage 2: Keywords.
	var keywords []string
	if p.config.Summary.Keywords.Enabled {
		slog.Info("stage 2: extracting keywords", "video_id", meta.ID)
		kwPrompt, kwPromptErr := summarizer.KeywordPrompt(
			summaryText,
			p.config.Summary.Keywords.Language,
			p.config.Summary.Keywords.Count,
		)
		if kwPromptErr != nil {
			slog.Warn("stage 2 keyword prompt loading failed", "video_id", meta.ID, "error", kwPromptErr)
		}
		kwOpts := summarizer.SummarizeOptions{
			Prompt:    kwPrompt,
			MaxTokens: p.config.Summary.MaxTokens,
		}
		kwResult, kwErr := p.summarizer.Summarize(summaryText, kwOpts)
		if kwErr != nil {
			slog.Warn("stage 2 keyword extraction failed", "video_id", meta.ID, "error", kwErr)
		} else {
			keywords = summarizer.ParseKeywords(kwResult.Text)
		}
	}

	// Stage 3: Mermaid diagrams.
	var mermaidBlocks []summarizer.MermaidBlock
	if p.config.Summary.Mermaid.Enabled {
		slog.Info("stage 3: generating mermaid diagrams", "video_id", meta.ID)
		mermaidPrompt, mermaidPromptErr := summarizer.MermaidPrompt(summaryText, p.config.Summary.Language)
		if mermaidPromptErr != nil {
			slog.Warn("stage 3 mermaid prompt loading failed", "video_id", meta.ID, "error", mermaidPromptErr)
		}
		mermaidOpts := summarizer.SummarizeOptions{
			Prompt:    mermaidPrompt,
			MaxTokens: p.config.Summary.MaxTokens,
		}
		mermaidResult, mermaidErr := p.summarizer.Summarize(summaryText, mermaidOpts)
		if mermaidErr != nil {
			slog.Warn("stage 3 mermaid generation failed", "video_id", meta.ID, "error", mermaidErr)
		} else {
			mermaidBlocks = summarizer.ValidateMermaidBlocks(mermaidResult.Text)
			if len(mermaidBlocks) == 0 {
				slog.Warn("stage 3 mermaid validation failed", "video_id", meta.ID)
			} else {
				slog.Debug("stage 3 complete", "video_id", meta.ID, "diagram_count", len(mermaidBlocks))
			}
		}
	}

	// Build summary frontmatter with playlist fields.
	fmData := buildFrontmatterData(meta, channelHandle, subLang, subType, processedAt, p.timezone)
	fmData.Tags = keywords
	fmData.LLMProvider = summaryResult.Provider
	fmData.LLMModel = summaryResult.Model
	fmData.Playlist = playlist
	fmData.PlaylistID = playlistID

	// Enrich tags for Obsidian if enabled (LLM tags + channel + auto).
	if p.config.Obsidian.Enabled {
		fmData.Tags = output.EnrichTagsForObsidian(
			fmData.Tags, meta.ChannelName, p.config.Obsidian.AutoTags,
		)
	}

	summaryFM := output.BuildSummaryFrontmatter(fmData)

	summaryBody := summaryText
	if len(mermaidBlocks) > 0 {
		summaryBody = insertMermaidBlocksAfterFirstHeading(summaryBody, mermaidBlocks)
	}

	summaryPath := filepath.Join(videoDir, filePrefix+"summary.md")
	embedEnabled := channelCfg == nil && p.config.VideoEmbed || channelCfg != nil && p.config.EffectiveVideoEmbed(*channelCfg)
	var videoEmbed string
	if embedEnabled {
		videoEmbed = "![](" + meta.URL + ")\n\n"
	}
	summaryContent := summaryFM + videoEmbed + summaryBody + "\n"

	if err := os.WriteFile(summaryPath, []byte(summaryContent), 0o644); err != nil {
		return fmt.Errorf("writing summary file: %w", err)
	}
	p.index.AddFile(meta.ID, "summary.md")

	slog.Info("summary written", "video_id", meta.ID, "path", summaryPath)
	return nil
}

// executeCopyTo runs the copy_to post-processing step for a successfully processed video.
func (p *Pipeline) executeCopyTo(copyTo *config.CopyToConfig, videoDir, filePrefix string, meta *fetcher.VideoMeta, channelHandle, playlist, playlistID string) {
	uploadDate := output.ConvertUploadDate(meta.UploadDate, meta.Timestamp, p.timezone)
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
		VideoEmbed:        pl.VideoEmbed,
		Filter:            pl.Filter,
		Cookie:            pl.Cookie,
		CopyTo:            pl.CopyTo,
	}
}

// errSkipped is a sentinel used to signal that a video was skipped (not an error).
var errSkipped = fmt.Errorf("skipped")

// IsSkipped returns true if the error is the sentinel skipped error.
func IsSkipped(err error) bool {
	return err == errSkipped
}

// emptyResponseError builds the stage-1 "empty response" error, naming the
// provider/model that returned empty text so the failure log identifies the
// actual LLM backend used (an empty response counts as success to the
// fallback chain, so summaryResult still carries the provider that answered).
func emptyResponseError(stage, provider, model string) error {
	return fmt.Errorf("%s: LLM returned empty response from provider %q (model %q) — if using a thinking model (e.g., Qwen3.5), ensure think mode is disabled or increase max_tokens", stage, provider, model)
}

// errPartial is a sentinel signaling that a video was partially processed:
// its transcription was written but summarization failed (e.g. all LLM
// providers out of quota). It is wrapped with the underlying cause via %w,
// so detection uses errors.Is rather than equality.
var errPartial = errors.New("partial: transcription produced but summarization failed")

// IsPartial returns true if err is (or wraps) the partial sentinel.
func IsPartial(err error) bool {
	return errors.Is(err, errPartial)
}

// resultBucket classifies a single video's processing outcome for Stats.
type resultBucket int

const (
	bucketSuccess resultBucket = iota
	bucketSkipped
	bucketPartial
	bucketFailed
)

// classifyResult maps a ProcessVideo return value to its stats bucket.
// Order matters: skipped and partial are sentinels checked before the
// catch-all failed bucket.
func classifyResult(err error) resultBucket {
	switch {
	case err == nil:
		return bucketSuccess
	case IsSkipped(err):
		return bucketSkipped
	case IsPartial(err):
		return bucketPartial
	default:
		return bucketFailed
	}
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

// buildFrontmatterData creates a FrontmatterData struct from video metadata.
func buildFrontmatterData(
	meta *fetcher.VideoMeta,
	channelHandle string,
	language string,
	subtitleType string,
	processedAt string,
	loc *time.Location,
) output.FrontmatterData {
	return output.FrontmatterData{
		Title:        meta.Title,
		VideoID:      meta.ID,
		URL:          meta.URL,
		Channel:      "@" + channelHandle,
		ChannelName:  meta.ChannelName,
		UploadDate:   output.ConvertUploadDate(meta.UploadDate, meta.Timestamp, loc),
		UploadTime:   output.FormatUploadTime(meta.Timestamp, loc),
		Timezone:     loc.String(),
		Duration:     meta.DurationString,
		Language:     language,
		VideoTags:    meta.Tags,
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

// insertMermaidBlocksAfterFirstHeading inserts Mermaid code blocks into the summary.
// Blocks whose Title exactly matches a #### heading are inserted after that section's content.
// Unmatched blocks fall back to being inserted after the overview (before the second ### heading).
// normalizeHeading strips leading # markers and whitespace from a markdown heading.
// "#### 章節標題" → "章節標題", "### 章節標題" → "章節標題"
func normalizeHeading(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimLeft(s, "#")
	return strings.TrimSpace(s)
}

func insertMermaidBlocksAfterFirstHeading(summaryText string, blocks []summarizer.MermaidBlock) string {
	lines := strings.Split(summaryText, "\n")

	// Build a map of normalized heading text → insertion line index.
	// Normalized = strip leading # markers so "### Title" and "#### Title" both key as "Title".
	// This handles LLMs shifting heading levels (e.g., using ### instead of ####).
	type sectionPos struct {
		headingLine int // line index of the heading
		insertLine  int // line index to insert before (next heading or EOF)
	}
	sections := map[string]sectionPos{}
	var headingIndices []int
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "### ") || strings.HasPrefix(trimmed, "#### ") {
			headingIndices = append(headingIndices, i)
		}
	}
	for idx, hi := range headingIndices {
		trimmed := strings.TrimSpace(lines[hi])
		if !strings.HasPrefix(trimmed, "### ") && !strings.HasPrefix(trimmed, "#### ") {
			continue
		}
		// Find the next heading (any level ### or ####) after this one.
		insertAt := len(lines) // default: end of file
		if idx+1 < len(headingIndices) {
			insertAt = headingIndices[idx+1]
		}
		key := normalizeHeading(trimmed)
		// Prefer #### over ### when both exist with the same text.
		if existing, ok := sections[key]; ok {
			existingTrimmed := strings.TrimSpace(lines[existing.headingLine])
			if strings.HasPrefix(existingTrimmed, "#### ") {
				continue // keep existing #### entry
			}
		}
		sections[key] = sectionPos{headingLine: hi, insertLine: insertAt}
	}

	// Classify blocks into matched (with insertion line) and unmatched.
	type insertion struct {
		line  int
		block summarizer.MermaidBlock
	}
	var matched []insertion
	var unmatched []summarizer.MermaidBlock
	for _, b := range blocks {
		key := normalizeHeading(b.Title)
		if pos, ok := sections[key]; ok {
			matched = append(matched, insertion{line: pos.insertLine, block: b})
		} else {
			unmatched = append(unmatched, b)
		}
	}

	// Sort matched insertions by line descending so we can insert back-to-front
	// without shifting indices.
	for i := 0; i < len(matched); i++ {
		for j := i + 1; j < len(matched); j++ {
			if matched[j].line > matched[i].line {
				matched[i], matched[j] = matched[j], matched[i]
			}
		}
	}

	// Insert matched blocks (back-to-front) without title (already under the matching heading).
	for _, ins := range matched {
		snippet := formatMermaidSnippet(ins.block, false)
		before := lines[:ins.line]
		after := lines[ins.line:]
		lines = make([]string, 0, len(before)+1+len(after))
		lines = append(lines, before...)
		lines = append(lines, snippet)
		lines = append(lines, after...)
	}

	result := strings.Join(lines, "\n")

	// Insert unmatched blocks after overview (before the second ### heading).
	if len(unmatched) > 0 {
		var sb strings.Builder
		for _, b := range unmatched {
			sb.WriteString(formatMermaidSnippet(b, true))
		}
		fallback := sb.String()

		resultLines := strings.Split(result, "\n")
		h3Count := 0
		for i, line := range resultLines {
			if strings.HasPrefix(strings.TrimSpace(line), "### ") {
				h3Count++
				if h3Count == 2 {
					before := strings.Join(resultLines[:i], "\n")
					after := strings.Join(resultLines[i:], "\n")
					return before + fallback + "\n" + after
				}
			}
		}
		// Fewer than 2 ### headings — append at end.
		return result + fallback
	}

	return result
}

// formatMermaidSnippet formats a single MermaidBlock as a markdown snippet.
// When includeTitle is false, the title is omitted (used for matched insertions
// where the diagram is already under the corresponding section heading).
func formatMermaidSnippet(b summarizer.MermaidBlock, includeTitle bool) string {
	var sb strings.Builder
	sb.WriteString("\n")
	if includeTitle && b.Title != "" {
		sb.WriteString(b.Title)
		sb.WriteString("\n")
	}
	sb.WriteString("```mermaid\n")
	sb.WriteString(b.Code)
	sb.WriteString("\n```\n")
	return sb.String()
}
