package pipeline

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kouko/youtube-summarize-scraper/config"
	"github.com/kouko/youtube-summarize-scraper/fetcher"
)

// mockFetcher is a test double for videoFetcher.
type mockFetcher struct {
	// tabResults maps tab URL suffix (e.g. "/videos") to its result.
	tabResults map[string]struct {
		videos []fetcher.VideoMeta
		err    error
	}
}

func (m *mockFetcher) FetchVideoMeta(string) (*fetcher.VideoMeta, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockFetcher) FetchChannelTab(tabURL string, _ int) ([]fetcher.VideoMeta, error) {
	for suffix, r := range m.tabResults {
		if strings.HasSuffix(tabURL, suffix) {
			return r.videos, r.err
		}
	}
	return nil, fmt.Errorf("unexpected tab URL: %s", tabURL)
}

func (m *mockFetcher) FetchPlaylistVideos(string, int, []string) ([]fetcher.VideoMeta, string, error) {
	return nil, "", fmt.Errorf("not implemented")
}

func newFetchTestPipeline(f videoFetcher) *Pipeline {
	return &Pipeline{
		config:  &config.Config{},
		fetcher: f,
	}
}

func TestFetchAllTabs_AllSucceed(t *testing.T) {
	m := &mockFetcher{
		tabResults: map[string]struct {
			videos []fetcher.VideoMeta
			err    error
		}{
			"/videos":  {videos: []fetcher.VideoMeta{{ID: "v1"}, {ID: "v2"}}},
			"/streams": {videos: []fetcher.VideoMeta{{ID: "s1"}}},
		},
	}
	p := newFetchTestPipeline(m)
	filterCfg := config.FilterConfig{Types: []string{"video", "live"}}

	videos, err := p.fetchAllTabs("https://www.youtube.com/@ch", 10, filterCfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(videos) != 3 {
		t.Fatalf("expected 3 videos, got %d", len(videos))
	}
}

func TestFetchAllTabs_OneTabFails(t *testing.T) {
	m := &mockFetcher{
		tabResults: map[string]struct {
			videos []fetcher.VideoMeta
			err    error
		}{
			"/videos":  {videos: []fetcher.VideoMeta{{ID: "v1"}, {ID: "v2"}}},
			"/streams": {err: fmt.Errorf("tab not found")},
		},
	}
	p := newFetchTestPipeline(m)
	filterCfg := config.FilterConfig{Types: []string{"video", "live"}}

	videos, err := p.fetchAllTabs("https://www.youtube.com/@ch", 10, filterCfg)
	if err != nil {
		t.Fatalf("should succeed when at least one tab works, got: %v", err)
	}
	if len(videos) != 2 {
		t.Fatalf("expected 2 videos from /videos tab, got %d", len(videos))
	}
}

func TestFetchAllTabs_AllTabsFail(t *testing.T) {
	m := &mockFetcher{
		tabResults: map[string]struct {
			videos []fetcher.VideoMeta
			err    error
		}{
			"/videos":  {err: fmt.Errorf("network error")},
			"/streams": {err: fmt.Errorf("tab not found")},
		},
	}
	p := newFetchTestPipeline(m)
	filterCfg := config.FilterConfig{Types: []string{"video", "live"}}

	_, err := p.fetchAllTabs("https://www.youtube.com/@ch", 10, filterCfg)
	if err == nil {
		t.Fatal("expected error when all tabs fail")
	}
	if !strings.Contains(err.Error(), "all channel tabs failed") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestFetchAllTabs_EmptyChannelNoError(t *testing.T) {
	m := &mockFetcher{
		tabResults: map[string]struct {
			videos []fetcher.VideoMeta
			err    error
		}{
			"/videos": {videos: []fetcher.VideoMeta{}},
		},
	}
	p := newFetchTestPipeline(m)
	filterCfg := config.FilterConfig{Types: []string{"video"}}

	videos, err := p.fetchAllTabs("https://www.youtube.com/@ch", 10, filterCfg)
	if err != nil {
		t.Fatalf("empty channel should not error: %v", err)
	}
	if len(videos) != 0 {
		t.Fatalf("expected 0 videos, got %d", len(videos))
	}
}

func TestFetchAllTabs_RespectsCountLimit(t *testing.T) {
	vids := make([]fetcher.VideoMeta, 20)
	for i := range vids {
		vids[i] = fetcher.VideoMeta{ID: fmt.Sprintf("v%d", i)}
	}
	m := &mockFetcher{
		tabResults: map[string]struct {
			videos []fetcher.VideoMeta
			err    error
		}{
			"/videos": {videos: vids},
		},
	}
	p := newFetchTestPipeline(m)
	filterCfg := config.FilterConfig{Types: []string{"video"}}

	videos, err := p.fetchAllTabs("https://www.youtube.com/@ch", 5, filterCfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(videos) != 5 {
		t.Fatalf("expected 5 videos (count limit), got %d", len(videos))
	}
}
