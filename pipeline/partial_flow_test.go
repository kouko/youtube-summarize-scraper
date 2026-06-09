package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kouko/youtube-summarize-scraper/config"
	"github.com/kouko/youtube-summarize-scraper/fetcher"
	"github.com/kouko/youtube-summarize-scraper/output"
	"github.com/kouko/youtube-summarize-scraper/summarizer"
)

// quotaAlwaysSummarizer is a fake Summarizer that always reports quota
// exhaustion — standing in for "every provider is out of quota".
type quotaAlwaysSummarizer struct{ calls int }

func (q *quotaAlwaysSummarizer) Summarize(text string, opts summarizer.SummarizeOptions) (summarizer.SummarizeResult, error) {
	q.calls++
	return summarizer.SummarizeResult{}, &summarizer.QuotaError{
		Provider: "fake",
		Err:      fmt.Errorf("429 all providers out of quota"),
		Kind:     summarizer.KindExhausted,
	}
}

// TestProcessVideo_SummarizationQuotaFailure_IsPartialNoSummary drives the full
// ProcessVideo path (via the resume seam, which skips subtitle/transcriber) and
// confirms that when summarization fails for quota reasons:
//   - ProcessVideo returns a Partial error (not nil, not a hard failure), and
//   - no summary.md is written (so a later run re-resumes).
//
// This is the end-to-end wiring that unit tests of classifyResult cannot reach.
func TestProcessVideo_SummarizationQuotaFailure_IsPartialNoSummary(t *testing.T) {
	outDir := t.TempDir()
	videoID := "vid12345678"
	handle := "testchannel"

	// Seed an existing video dir with a transcription but no summary, so
	// ProcessVideo takes the resume path (no subtitle/transcriber needed).
	vdir := filepath.Join(outDir, "@"+handle, "2026-01-01__"+videoID+"__Sample")
	if err := os.MkdirAll(vdir, 0o755); err != nil {
		t.Fatal(err)
	}
	transcriptionPath := filepath.Join(vdir, "2026-01-01__"+videoID+"__transcription.md")
	transcriptionBody := "---\ntitle: Sample\n---\nThis is the transcript body to summarize.\n"
	if err := os.WriteFile(transcriptionPath, []byte(transcriptionBody), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.DefaultConfig()
	cfg.OutputDir = outDir

	// Index built from the seeded layout: the video has transcription.md and no
	// summary.md, so ProcessVideo takes the resume guard.
	idx := output.BuildIndex(outDir)

	fake := &quotaAlwaysSummarizer{}
	p := &Pipeline{
		config:     cfg,
		summarizer: fake,
		index:      idx,
		timezone:   time.UTC,
		ctx:        context.Background(),
	}

	meta := &fetcher.VideoMeta{
		ID:         videoID,
		Channel:    "@" + handle,
		Title:      "Sample",
		UploadDate: "20260101",
		Tags:       []string{}, // non-nil → ProcessVideo skips the metadata fetch
	}

	err := p.ProcessVideo(meta, nil)

	if !IsPartial(err) {
		t.Fatalf("quota summarization failure should yield a Partial error, got: %v", err)
	}
	if fake.calls == 0 {
		t.Error("the fake summarizer was never called — resume path did not run")
	}
	if matches, _ := filepath.Glob(filepath.Join(vdir, "*__summary.md")); len(matches) > 0 {
		t.Errorf("summary.md must not be written when summarization fails, found: %v", matches)
	}
}

// TestProcessVideo_SummarizationSucceeds_NotPartial is the positive control:
// the same path with a working summarizer completes (nil error) and writes
// summary.md — proving the Partial result above is caused by the quota failure,
// not by the resume harness itself.
func TestProcessVideo_SummarizationSucceeds_NotPartial(t *testing.T) {
	outDir := t.TempDir()
	videoID := "vidOK456789"
	handle := "testchannel"

	vdir := filepath.Join(outDir, "@"+handle, "2026-01-01__"+videoID+"__Sample")
	if err := os.MkdirAll(vdir, 0o755); err != nil {
		t.Fatal(err)
	}
	transcriptionPath := filepath.Join(vdir, "2026-01-01__"+videoID+"__transcription.md")
	if err := os.WriteFile(transcriptionPath, []byte("---\ntitle: Sample\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.DefaultConfig()
	cfg.OutputDir = outDir
	// Keep the summary single-stage: no keywords / mermaid follow-ups.
	cfg.Summary.Keywords.Enabled = false
	cfg.Summary.Mermaid.Enabled = false

	idx := output.BuildIndex(outDir)

	p := &Pipeline{
		config:     cfg,
		summarizer: okSummarizer{},
		index:      idx,
		timezone:   time.UTC,
		ctx:        context.Background(),
	}

	meta := &fetcher.VideoMeta{
		ID: videoID, Channel: "@" + handle, Title: "Sample",
		UploadDate: "20260101", Tags: []string{},
	}

	err := p.ProcessVideo(meta, nil)
	if err != nil {
		t.Fatalf("successful summarization should return nil, got: %v", err)
	}
	if IsPartial(err) {
		t.Error("a completed video must not be reported as Partial")
	}
	if matches, _ := filepath.Glob(filepath.Join(vdir, "*__summary.md")); len(matches) == 0 {
		t.Error("summary.md should be written on success")
	}
}

// okSummarizer returns a usable summary.
type okSummarizer struct{}

func (okSummarizer) Summarize(text string, opts summarizer.SummarizeOptions) (summarizer.SummarizeResult, error) {
	return summarizer.SummarizeResult{Text: "## Summary\n\nA fine summary.", Provider: "fake", Model: "m"}, nil
}
