package pipeline

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
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

// callTrackingFetcher records FetchVideoMeta calls for the early-skip tests.
type callTrackingFetcher struct {
	calls int
	fn    func(url string) (*fetcher.VideoMeta, error)
}

func (f *callTrackingFetcher) FetchVideoMeta(url string) (*fetcher.VideoMeta, error) {
	f.calls++
	if f.fn != nil {
		return f.fn(url)
	}
	return nil, nil
}

func (f *callTrackingFetcher) FetchChannelTab(string, int) ([]fetcher.VideoMeta, error) {
	return nil, nil
}

func (f *callTrackingFetcher) FetchPlaylistVideos(string, int, []string) ([]fetcher.VideoMeta, string, error) {
	return nil, "", nil
}

// buildProcessedIndex creates outputDir/@testhandle/20240101__<videoID>__Test-Video
// with a summary file, then builds the index from disk. The file name must end
// with "__summary.md" for BuildIndex to register the summary flag.
func buildProcessedIndex(t *testing.T, outputDir, videoID string) *output.VideoIndex {
	t.Helper()
	videoDir := filepath.Join(outputDir, "@testhandle", "20240101__"+videoID+"__Test-Video")
	if err := output.EnsureDir(videoDir); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	summaryPath := filepath.Join(videoDir, "20240101__"+videoID+"__summary.md")
	if err := os.WriteFile(summaryPath, []byte("test"), 0644); err != nil {
		t.Fatalf("Write summary: %v", err)
	}
	return output.BuildIndex(outputDir)
}

// TestProcessVideo_EarlySkip verifies the two early-exit checks at the top of
// ProcessVideo: an already-processed video (index has summary.md/.skipped under
// the config URL's @handle) and dry-run mode both return errSkipped before any
// metadata fetch. Unindexed videos fetch exactly once; a handle mismatch falls
// through to the post-fetch check; --force bypasses the early check.
func TestProcessVideo_EarlySkip(t *testing.T) {
	tests := []struct {
		name             string
		dryRun           bool
		force            bool
		channelCfg       *config.ChannelConfig
		videoID          string
		alreadyProcessed bool
		fetchFullMeta    *fetcher.VideoMeta // returned by FetchVideoMeta; nil means error
		wantSkipped      bool
		wantFetchCalls   int
	}{
		{
			name:        "dry-run skips metadata fetch",
			dryRun:      true,
			videoID:     "earlySkipVid1",
			wantSkipped: true,
		},
		{
			name:             "already-processed skips metadata fetch",
			channelCfg:       &config.ChannelConfig{URL: "https://www.youtube.com/@testhandle"},
			videoID:          "earlySkipVid2",
			alreadyProcessed: true,
			wantSkipped:      true,
		},
		{
			name:       "unindexed video fetches exactly once",
			channelCfg: &config.ChannelConfig{URL: "https://www.youtube.com/@testhandle"},
			videoID:    "earlySkipVid3",
			// Full fetch fails and the partial meta has no channel info,
			// so ProcessVideo returns an error right after the fetch.
			wantSkipped:    false,
			wantFetchCalls: 1,
		},
		{
			name:             "renamed handle falls through then post-fetch skips",
			channelCfg:       &config.ChannelConfig{URL: "https://www.youtube.com/@renamedhandle"},
			videoID:          "earlySkipVid4",
			alreadyProcessed: true,
			// Full meta reports the original @testhandle channel, so the
			// post-fetch IsProcessed check still catches it.
			fetchFullMeta: &fetcher.VideoMeta{
				ID: "earlySkipVid4", Title: "Test Video", Channel: "@testhandle",
				UploadDate: "20240101", Tags: []string{"tag"},
			},
			wantSkipped:    true,
			wantFetchCalls: 1,
		},
		{
			name:             "force bypasses early processed check",
			force:            true,
			channelCfg:       &config.ChannelConfig{URL: "https://www.youtube.com/@testhandle"},
			videoID:          "earlySkipVid5",
			alreadyProcessed: true,
			// Fetch fails and partial meta has no channel → error, proving
			// the early skip did not fire.
			wantSkipped:    false,
			wantFetchCalls: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			outputDir := t.TempDir()

			var idx *output.VideoIndex
			if tc.alreadyProcessed {
				idx = buildProcessedIndex(t, outputDir, tc.videoID)
			} else {
				idx = output.BuildIndex(outputDir)
			}

			fetcherMock := &callTrackingFetcher{fn: func(string) (*fetcher.VideoMeta, error) {
				if tc.fetchFullMeta != nil {
					return tc.fetchFullMeta, nil
				}
				return nil, fmt.Errorf("fetch failed")
			}}

			p := &Pipeline{
				config:   &config.Config{OutputDir: outputDir},
				fetcher:  fetcherMock,
				index:    idx,
				timezone: time.UTC,
				force:    tc.force,
				dryRun:   tc.dryRun,
			}

			// Partial metadata (Tags nil) — without an early skip this
			// triggers the full-metadata fetch branch.
			meta := &fetcher.VideoMeta{
				ID:    tc.videoID,
				Title: "Test Video",
			}

			var channelCfg *config.ChannelConfig
			if tc.channelCfg != nil {
				channelCfg = tc.channelCfg
			}

			err := p.ProcessVideo(meta, channelCfg)

			if got := IsSkipped(err); got != tc.wantSkipped {
				t.Errorf("IsSkipped(err) = %v, want %v (err=%v)", got, tc.wantSkipped, err)
			}
			if fetcherMock.calls != tc.wantFetchCalls {
				t.Errorf("FetchVideoMeta calls = %d, want %d", fetcherMock.calls, tc.wantFetchCalls)
			}
		})
	}
}

// membersOnlyFetchErr returns an error in the shape FetchVideoMeta produces for
// a members-only video: the yt-dlp wrapper prefix followed by the observed
// stderr signature naming the membership tier.
func membersOnlyFetchErr() error {
	return fmt.Errorf("fetching video metadata: yt-dlp [--dump-json --no-download https://www.youtube.com/watch?v=eYOcdhR-72c]: exit status 1\nstderr: ERROR: [youtube] eYOcdhR-72c: This video is available to this channel's members on level: 小君 (or any higher level). Join this channel to get access to members-only content and other exclusive perks.")
}

// partialMeta builds a flat-playlist entry: no channel info and no upload date,
// so ProcessVideo enters the full-metadata fetch branch.
func partialMeta(videoID string) *fetcher.VideoMeta {
	return &fetcher.VideoMeta{ID: videoID, Title: "Test Video"}
}

// newMetadataFailurePipeline builds a Pipeline whose FetchVideoMeta always
// fails with fetchErr. The subtitle seam is a fake that would only be reached
// by a mistaken success path, so every members-only test asserts the fetch
// branch returned before it.
func newMetadataFailurePipeline(outputDir string, fetchErr error) (*Pipeline, *callTrackingFetcher) {
	fake := &callTrackingFetcher{fn: func(string) (*fetcher.VideoMeta, error) {
		return nil, fetchErr
	}}
	p := &Pipeline{
		config:   &config.Config{OutputDir: outputDir},
		fetcher:  fake,
		subtitle: &fakeSubtitleDownloader{err: fmt.Errorf("no subtitles")},
		index:    output.BuildIndex(outputDir),
		timezone: time.UTC,
	}
	return p, fake
}

// TestProcessVideo_MembersOnlyIsSkip covers Acceptance 1 at the single-video
// level: a metadata failure whose stderr carries a members-only signature, with
// no channel info from the flat playlist, is a skip for this scan, not a
// failure. The skip writes nothing under the output dir, so it leaves no marker
// for later scans (Acceptance 2).
func TestProcessVideo_MembersOnlyIsSkip(t *testing.T) {
	t.Run("membersOnlyIsSkip", func(t *testing.T) {
		outputDir := t.TempDir()
		p, fake := newMetadataFailurePipeline(outputDir, membersOnlyFetchErr())

		err := p.ProcessVideo(partialMeta("membersOnlyIsSkip"), nil)
		if !IsSkipped(err) {
			t.Fatalf("ProcessVideo err = %v, want errSkipped", err)
		}
		if fake.calls != 1 {
			t.Errorf("FetchVideoMeta calls = %d, want 1", fake.calls)
		}
	})

	t.Run("noMarkerFile", func(t *testing.T) {
		outputDir := t.TempDir()
		p, _ := newMetadataFailurePipeline(outputDir, membersOnlyFetchErr())

		if err := p.ProcessVideo(partialMeta("noMarkerFileVid"), nil); !IsSkipped(err) {
			t.Fatalf("ProcessVideo err = %v, want errSkipped", err)
		}

		var written []string
		walkErr := filepath.WalkDir(outputDir, func(path string, _ fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if path != outputDir {
				written = append(written, path)
			}
			return nil
		})
		if walkErr != nil {
			t.Fatalf("walk output dir: %v", walkErr)
		}
		if len(written) > 0 {
			t.Errorf("members-only skip wrote files/dirs under output dir: %v — no marker (or any state) may persist", written)
		}
	})
}

// TestProcessVideo_MembersOnlyReattempt covers Acceptance 2: nothing is
// persisted, so the next scan reaches the fetcher again instead of
// short-circuiting on a marker.
func TestProcessVideo_MembersOnlyReattempt(t *testing.T) {
	outputDir := t.TempDir()
	p, fake := newMetadataFailurePipeline(outputDir, membersOnlyFetchErr())
	meta := partialMeta("reattemptVid")

	for attempt := 1; attempt <= 2; attempt++ {
		if err := p.ProcessVideo(meta, nil); !IsSkipped(err) {
			t.Fatalf("attempt %d: ProcessVideo err = %v, want errSkipped", attempt, err)
		}
		if fake.calls != attempt {
			t.Fatalf("attempt %d: FetchVideoMeta calls = %d, want %d", attempt, fake.calls, attempt)
		}
	}
}

// TestProcessChannelVideos_MembersOnlyCountedFailed covers Acceptance 1's
// counting: a members-only metadata failure goes through processChannelVideos
// as a skip — Skipped increments, Failed stays 0, and no VideoError is
// recorded (that is what drives the ERROR log and the failed total).
func TestProcessChannelVideos_MembersOnlyCountedFailed(t *testing.T) {
	outputDir := t.TempDir()
	p, _ := newMetadataFailurePipeline(outputDir, membersOnlyFetchErr())

	stats, err := p.processChannelVideos([]fetcher.VideoMeta{
		{ID: "countedFailedVid", Title: "Test Video"},
	}, nil)
	if err != nil {
		t.Fatalf("processChannelVideos: %v", err)
	}
	if stats.Skipped != 1 {
		t.Errorf("stats.Skipped = %d, want 1", stats.Skipped)
	}
	if stats.Failed != 0 {
		t.Errorf("stats.Failed = %d, want 0", stats.Failed)
	}
	if len(stats.Errors) != 0 {
		t.Errorf("stats.Errors = %d entries, want 0: %v", len(stats.Errors), stats.Errors)
	}
}

// newRecoveringPipeline fails the first metadata fetch with a members-only
// error and succeeds with full metadata afterwards — the user's access appeared
// (token refresh or a new subscription). Used by
// TestProcessVideo_MembersOnlyAfterEarlier for Acceptance 3.
func newRecoveringPipeline(outputDir string) (*Pipeline, *callTrackingFetcher) {
	fake := &callTrackingFetcher{}
	fake.fn = func(string) (*fetcher.VideoMeta, error) {
		if fake.calls == 1 {
			return nil, membersOnlyFetchErr()
		}
		return &fetcher.VideoMeta{
			ID:         "recoveredVid",
			Title:      "Test Video",
			Channel:    "@testhandle",
			UploadDate: "20240101",
			Tags:       []string{"tag"},
		}, nil
	}
	p := &Pipeline{
		config:  &config.Config{OutputDir: outputDir},
		fetcher: fake,
		subtitle: &fakeSubtitleDownloader{result: &subtitle.SubtitleResult{
			Content:      "1\n00:00:00,000 --> 00:00:01,000\nhello world\n\n",
			Language:     "en",
			SubtitleType: "auto",
		}},
		index:    output.BuildIndex(outputDir),
		timezone: time.UTC,
	}
	return p, fake
}

// TestProcessVideo_MembersOnlyAfterEarlier covers Acceptance 3: the earlier
// skip persists nothing, so a later attempt that CAN fetch the metadata is
// processed through the normal success path with no members-only branch.
func TestProcessVideo_MembersOnlyAfterEarlier(t *testing.T) {
	t.Run("successAfterEarlierFailure", func(t *testing.T) {
		outputDir := t.TempDir()
		p, fake := newRecoveringPipeline(outputDir)
		meta := partialMeta("recoveredVid")

		if err := p.ProcessVideo(meta, nil); !IsSkipped(err) {
			t.Fatalf("first attempt: err = %v, want errSkipped (members-only)", err)
		}
		if err := p.ProcessVideo(meta, nil); err != nil {
			t.Fatalf("second attempt: err = %v, want nil (normal success path)", err)
		}
		if fake.calls != 2 {
			t.Errorf("FetchVideoMeta calls = %d, want 2", fake.calls)
		}
	})

	t.Run("noSpecialHandlingAfterSuccess", func(t *testing.T) {
		outputDir := t.TempDir()
		p, _ := newRecoveringPipeline(outputDir)
		meta := partialMeta("recoveredVid")

		if err := p.ProcessVideo(meta, nil); !IsSkipped(err) {
			t.Fatalf("first attempt: err = %v, want errSkipped", err)
		}
		if err := p.ProcessVideo(meta, nil); err != nil {
			t.Fatalf("second attempt: err = %v, want nil (normal success path)", err)
		}

		// The normal success path produced its artifacts — the same ones any
		// first-time success writes, with no members-only special handling.
		matches, _ := filepath.Glob(filepath.Join(outputDir, "@testhandle", "*", "*__transcription.md"))
		if len(matches) != 1 {
			t.Errorf("transcription.md files = %d, want 1: %v", len(matches), matches)
		}
		if !p.index.HasFile("recoveredVid", "transcription.md") {
			t.Error("index does not list transcription.md — normal success path did not complete")
		}
	})
}

// TestProcessVideo_OtherFailure covers Acceptance 4: any metadata failure that
// is NOT a members-only signature (a network error here) keeps the pre-change
// behaviour — a real error, classified as failed.
func TestProcessVideo_OtherFailure(t *testing.T) {
	t.Run("otherFailureStillFails", func(t *testing.T) {
		outputDir := t.TempDir()
		p, _ := newMetadataFailurePipeline(outputDir, fmt.Errorf("network timeout after 180s"))

		err := p.ProcessVideo(partialMeta("otherFailureVid"), nil)
		if err == nil {
			t.Fatal("ProcessVideo err = nil, want a non-nil failure error")
		}
		if IsSkipped(err) {
			t.Errorf("ProcessVideo err = %v, want NOT skipped", err)
		}
	})

	t.Run("otherFailureNotSkipped", func(t *testing.T) {
		outputDir := t.TempDir()
		p, _ := newMetadataFailurePipeline(outputDir, fmt.Errorf("network timeout after 180s"))

		err := p.ProcessVideo(partialMeta("otherFailureVid2"), nil)
		if bucket := classifyResult(err); bucket != bucketFailed {
			t.Errorf("classifyResult(err) = %v, want bucketFailed (err=%v)", bucket, err)
		}
	})
}

// TestIsMembersOnlyError_Signatures pins the recognition surface (plan Risk 1):
// both observed yt-dlp stderr signatures — the named-level annotation and the
// "Join this channel" form — classify as members-only in the wrapped form
// FetchVideoMeta produces, each half of the match is load-bearing, and matching
// is case-insensitive.
func TestIsMembersOnlyError_Signatures(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "named-tier signature, wrapped",
			err:  membersOnlyFetchErr(),
		},
		{
			name: "join-content signature",
			err:  errors.New("fetching video metadata: yt-dlp: exit status 1\nstderr: ERROR: [youtube] X: Join this channel to get access to members-only content like this video, and other exclusive perks."),
		},
		{
			name: "level fragment without the members-only hyphen",
			err:  errors.New("ERROR: [youtube] X: This video is available to this channel's members only"),
		},
		{
			name: "uppercase stderr",
			err:  errors.New("ERROR: [youtube] X: JOIN THIS CHANNEL TO GET ACCESS TO MEMBERS-ONLY CONTENT"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if !isMembersOnlyError(tc.err) {
				t.Errorf("isMembersOnlyError(%v) = false, want true", tc.err)
			}
		})
	}
}

// TestIsMembersOnlyError_NearMiss guards against over-classification: messages
// that merely mention "members" (channel member lists, memberships) and a nil
// error must NOT classify as members-only skips — those keep failing.
func TestIsMembersOnlyError_NearMiss(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "channel members list", err: errors.New("ERROR: [youtube] X: channel members list unavailable")},
		{name: "membership wording", err: errors.New("ERROR: [youtube] X: your membership expired")},
		{name: "nil error", err: nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if isMembersOnlyError(tc.err) {
				t.Errorf("isMembersOnlyError(%v) = true, want false", tc.err)
			}
		})
	}
}

// TestExtractHandleFromURL covers the URL forms accepted and rejected by the
// early-check handle parser.
func TestExtractHandleFromURL(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://www.youtube.com/@MacroMicro_tw", "MacroMicro_tw"},
		{"https://www.youtube.com/@MacroMicro_tw/videos", "MacroMicro_tw"},
		{"https://youtube.com/@somehandle/featured", "somehandle"},
		{"https://www.youtube.com/channel/UC1234567890abcdef", ""},
		{"https://www.youtube.com/c/SomeName", ""},
		{"https://www.youtube.com/user/SomeUser", ""},
		{"", ""},
	}
	for _, tc := range tests {
		if got := extractHandleFromURL(tc.url); got != tc.want {
			t.Errorf("extractHandleFromURL(%q) = %q, want %q", tc.url, got, tc.want)
		}
	}
}
