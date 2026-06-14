package pipeline

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/kouko/youtube-summarize-scraper/config"
	"github.com/kouko/youtube-summarize-scraper/fetcher"
	"github.com/kouko/youtube-summarize-scraper/output"
	"github.com/kouko/youtube-summarize-scraper/subtitle"
	"github.com/kouko/youtube-summarize-scraper/transcriber"
)

// fakeSubtitleDownloader is a test double for the subtitleDownloader seam.
// Its Download method matches subtitle.Downloader.Download's exact signature.
type fakeSubtitleDownloader struct {
	result *subtitle.SubtitleResult
	err    error
	called bool
}

func (f *fakeSubtitleDownloader) Download(_ string, _ []string, _ string, _ string, _ []string) (*subtitle.SubtitleResult, error) {
	f.called = true
	return f.result, f.err
}

// TestPipeline_SubtitleSeam asserts the Pipeline.subtitle field is an
// interface seam, not the concrete *subtitle.Downloader. Before Task 4a's
// field-type change this test does NOT compile: assigning a test-local fake
// to a concrete *subtitle.Downloader field is rejected by the type checker
// (a compile-diagnostic RED). After the change the fake is assignable and the
// seam is usable — which is what makes the Whisper-gate branch (Task 4)
// unit-testable.
func TestPipeline_SubtitleSeam(t *testing.T) {
	fake := &fakeSubtitleDownloader{err: fmt.Errorf("no subtitles")}

	p := &Pipeline{subtitle: fake}

	res, err := p.subtitle.Download("https://example.test/v", []string{"en"}, t.TempDir(), "prefix", nil)
	if err == nil {
		t.Fatal("expected fake to return an error, got nil")
	}
	if res != nil {
		t.Fatalf("expected nil result, got %v", res)
	}
	if !fake.called {
		t.Fatal("expected the fake's Download to be invoked through the seam")
	}
}

// TestProcessVideo_WhisperGate_TooLong covers the Whisper duration gate in
// ProcessVideo's subtitle-download-failed branch. When no subtitles are
// available AND the video exceeds whisper.max_duration, the pipeline must
// write a terminal `.skipped` marker and return errSkipped INSTEAD of running
// Whisper transcription. The inverse case (under the cap) must NOT write a
// marker and must NOT short-circuit as skipped — it falls through toward the
// (concrete, un-fakeable) transcriber, which errors in test; we assert on the
// observables (marker absence + non-skipped error), not transcription success.
func TestProcessVideo_WhisperGate_TooLong(t *testing.T) {
	const maxDuration = 7200 // 2h cap

	tests := []struct {
		name        string
		duration    float64
		wantSkipped bool // gate fires: .skipped written + IsSkipped(err)
		wantMarker  bool
		videoID     string
	}{
		{
			name:        "over cap fires gate",
			duration:    10000,
			wantSkipped: true,
			wantMarker:  true,
			videoID:     "vidOverCap0",
		},
		{
			name:        "under cap does not fire gate",
			duration:    3600,
			wantSkipped: false,
			wantMarker:  false,
			videoID:     "vidUnderCap",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			outputDir := t.TempDir()
			cfg := &config.Config{
				OutputDir: outputDir,
				Whisper:   config.WhisperConfig{MaxDuration: maxDuration},
			}

			p := &Pipeline{
				config:      cfg,
				subtitle:    &fakeSubtitleDownloader{err: fmt.Errorf("no subtitles available")},
				transcriber: transcriber.NewTranscriber("", "", "", cfg.Whisper),
				index:       output.BuildIndex(outputDir),
				timezone:    time.UTC,
			}

			meta := &fetcher.VideoMeta{
				ID:         tc.videoID,
				Title:      "Test Video",
				Channel:    "@testchannel",
				UploadDate: "20240101",
				Duration:   tc.duration,
				Tags:       []string{"tag"}, // non-nil → skip full-metadata fetch
			}

			err := p.ProcessVideo(meta, nil)

			if got := IsSkipped(err); got != tc.wantSkipped {
				t.Errorf("IsSkipped(err) = %v, want %v (err=%v)", got, tc.wantSkipped, err)
			}

			channelDir := filepath.Join(outputDir, "@testchannel")
			skippedMatches, _ := filepath.Glob(filepath.Join(channelDir, "*", "*__.skipped"))
			if hasMarker := len(skippedMatches) > 0; hasMarker != tc.wantMarker {
				t.Errorf("found %d .skipped markers, want marker=%v", len(skippedMatches), tc.wantMarker)
			}

			// The gate must NOT have run transcription: no transcription.md.
			transMatches, _ := filepath.Glob(filepath.Join(channelDir, "*", "*__transcription.md"))
			if tc.wantSkipped && len(transMatches) > 0 {
				t.Errorf("gate fired but transcription.md was written (%d files) — transcriber path taken", len(transMatches))
			}
		})
	}
}

// TestProcessVideoInPlaylist_WhisperGate_TooLong mirrors
// TestProcessVideo_WhisperGate_TooLong for the playlist code path. The
// subtitle-failed → Whisper branch in processVideoInPlaylist must honor the
// same whisper.max_duration gate: when no subtitles exist AND the video
// exceeds the cap, write a terminal `.skipped` marker and return errSkipped
// instead of burning local Whisper. Under the cap the gate must not fire.
// The caller (processPlaylistVideos) registers the index entry via
// index.Add before invoking processVideoInPlaylist, so the test mirrors that
// precondition — the gate's index.AddFile depends on it.
func TestProcessVideoInPlaylist_WhisperGate_TooLong(t *testing.T) {
	const maxDuration = 7200 // 2h cap

	tests := []struct {
		name        string
		duration    float64
		wantSkipped bool
		wantMarker  bool
		videoID     string
	}{
		{
			name:        "over cap fires gate",
			duration:    10000,
			wantSkipped: true,
			wantMarker:  true,
			videoID:     "plVidOverCap",
		},
		{
			name:        "under cap does not fire gate",
			duration:    3600,
			wantSkipped: false,
			wantMarker:  false,
			videoID:     "plVidUnderCap",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			outputDir := t.TempDir()
			cfg := &config.Config{
				OutputDir: outputDir,
				Whisper:   config.WhisperConfig{MaxDuration: maxDuration},
			}

			p := &Pipeline{
				config:      cfg,
				subtitle:    &fakeSubtitleDownloader{err: fmt.Errorf("no subtitles available")},
				transcriber: transcriber.NewTranscriber("", "", "", cfg.Whisper),
				index:       output.BuildIndex(outputDir),
				timezone:    time.UTC,
			}

			meta := &fetcher.VideoMeta{
				ID:         tc.videoID,
				Title:      "Test Video",
				Channel:    "@testchannel",
				UploadDate: "20240101",
				Duration:   tc.duration,
				Tags:       []string{"tag"},
			}

			// Mirror processPlaylistVideos: create the target dir and register
			// the index entry before calling processVideoInPlaylist.
			videoDir := filepath.Join(outputDir, "playlist", "20240101__"+tc.videoID+"__Test-Video")
			if err := output.EnsureDir(videoDir); err != nil {
				t.Fatalf("EnsureDir: %v", err)
			}
			p.index.Add(meta.ID, videoDir)

			err := p.processVideoInPlaylist(meta, "@testchannel", videoDir, "My Playlist", "PL123", &config.PlaylistConfig{})

			if got := IsSkipped(err); got != tc.wantSkipped {
				t.Errorf("IsSkipped(err) = %v, want %v (err=%v)", got, tc.wantSkipped, err)
			}

			skippedMatches, _ := filepath.Glob(filepath.Join(videoDir, "*__.skipped"))
			if hasMarker := len(skippedMatches) > 0; hasMarker != tc.wantMarker {
				t.Errorf("found %d .skipped markers, want marker=%v", len(skippedMatches), tc.wantMarker)
			}

			// The gate must NOT have run transcription: no transcription.md.
			transMatches, _ := filepath.Glob(filepath.Join(videoDir, "*__transcription.md"))
			if tc.wantSkipped && len(transMatches) > 0 {
				t.Errorf("gate fired but transcription.md was written (%d files) — transcriber path taken", len(transMatches))
			}
		})
	}
}

// TestLoopPreChecks_SkipMarked verifies that the lightweight already-processed
// pre-check at the top of BOTH loops (processChannelVideos and
// processPlaylistVideos) treats a terminal `.skipped` index marker the same as
// summary.md: the video is counted as Skipped WITHOUT entering the per-video
// pipeline. Entering the per-video function would invoke the subtitle
// downloader (and, on its failure, real yt-dlp/Whisper), so we assert the
// dispatch never happened via the fake's `called` flag staying false — the
// observable that proves the pre-check short-circuited, not the full pipeline.
func TestLoopPreChecks_SkipMarked(t *testing.T) {
	tests := []struct {
		name string
		loop string // "channel" | "playlist"
	}{
		{name: "channel loop honors .skipped", loop: "channel"},
		{name: "playlist loop honors .skipped", loop: "playlist"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			outputDir := t.TempDir()
			cfg := &config.Config{OutputDir: outputDir}

			// An existing on-disk dir so FindVideoDir resolves non-empty.
			videoID := "skipMarkedVid"
			plDir := output.PlaylistDir(outputDir, "PL123", "My Playlist")
			videoDir := filepath.Join(plDir, "20240101__"+videoID+"__Test-Video")
			if err := output.EnsureDir(videoDir); err != nil {
				t.Fatalf("EnsureDir: %v", err)
			}

			idx := output.BuildIndex(outputDir)
			idx.Add(videoID, videoDir)
			idx.AddFile(videoID, ".skipped") // terminal marker, no summary.md

			// A fake whose .called flag flips iff the per-video pipeline runs.
			fake := &fakeSubtitleDownloader{err: fmt.Errorf("no subtitles")}
			p := &Pipeline{
				config:   cfg,
				subtitle: fake,
				index:    idx,
				timezone: time.UTC,
			}

			meta := fetcher.VideoMeta{
				ID:    videoID,
				Title: "Test Video",
			}

			var stats *Stats
			var err error
			switch tc.loop {
			case "channel":
				stats, err = p.processChannelVideos([]fetcher.VideoMeta{meta}, nil)
			case "playlist":
				stats, err = p.processPlaylistVideos([]fetcher.VideoMeta{meta}, "PL123", "My Playlist", plDir, &config.PlaylistConfig{})
			}
			if err != nil {
				t.Fatalf("loop returned error: %v", err)
			}

			if stats.Skipped != 1 {
				t.Errorf("stats.Skipped = %d, want 1 (.skipped marker should count as skipped)", stats.Skipped)
			}
			if fake.called {
				t.Error("per-video pipeline was dispatched (subtitle.Download invoked) — pre-check did not short-circuit on .skipped")
			}
		})
	}
}
