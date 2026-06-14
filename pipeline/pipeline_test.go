package pipeline

import (
	"fmt"
	"testing"

	"github.com/kouko/youtube-summarize-scraper/subtitle"
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
