package fetcher

import (
	"testing"

	"github.com/kouko/youtube-summarize-scraper/config"
)

func TestFilterVideos_NoFilter(t *testing.T) {
	videos := []VideoMeta{
		{ID: "a", Duration: 100},
		{ID: "b", Duration: 500},
		{ID: "c", Duration: 1000},
	}
	got := FilterVideos(videos, config.FilterConfig{MinDuration: 0, MaxDuration: 0})
	if len(got) != 3 {
		t.Errorf("NoFilter: got %d videos, want 3", len(got))
	}
}

func TestFilterVideos_MinDuration(t *testing.T) {
	videos := []VideoMeta{
		{ID: "short", Duration: 120},
		{ID: "medium", Duration: 300},
		{ID: "long", Duration: 600},
	}
	got := FilterVideos(videos, config.FilterConfig{MinDuration: 300})
	if len(got) != 2 {
		t.Errorf("MinDuration: got %d videos, want 2", len(got))
	}
	for _, v := range got {
		if v.Duration < 300 {
			t.Errorf("MinDuration: video %q has duration %v, should be >= 300", v.ID, v.Duration)
		}
	}
}

func TestFilterVideos_MaxDuration(t *testing.T) {
	videos := []VideoMeta{
		{ID: "short", Duration: 120},
		{ID: "medium", Duration: 500},
		{ID: "long", Duration: 900},
	}
	got := FilterVideos(videos, config.FilterConfig{MaxDuration: 600})
	if len(got) != 2 {
		t.Errorf("MaxDuration: got %d videos, want 2", len(got))
	}
	for _, v := range got {
		if v.Duration > 600 {
			t.Errorf("MaxDuration: video %q has duration %v, should be <= 600", v.ID, v.Duration)
		}
	}
}

func TestFilterVideos_MinAndMax(t *testing.T) {
	videos := []VideoMeta{
		{ID: "too-short", Duration: 100},
		{ID: "in-range", Duration: 400},
		{ID: "also-in-range", Duration: 600},
		{ID: "too-long", Duration: 900},
	}
	got := FilterVideos(videos, config.FilterConfig{MinDuration: 300, MaxDuration: 700})
	if len(got) != 2 {
		t.Errorf("MinAndMax: got %d videos, want 2", len(got))
	}
	for _, v := range got {
		if v.ID != "in-range" && v.ID != "also-in-range" {
			t.Errorf("MinAndMax: unexpected video %q", v.ID)
		}
	}
}

func TestFilterVideos_FloatDuration(t *testing.T) {
	videos := []VideoMeta{
		{ID: "a", Duration: 1434.0},
		{ID: "b", Duration: 299.9},
	}
	got := FilterVideos(videos, config.FilterConfig{MinDuration: 300})
	if len(got) != 1 {
		t.Errorf("FloatDuration: got %d videos, want 1", len(got))
	}
	if got[0].ID != "a" {
		t.Errorf("FloatDuration: expected video 'a', got %q", got[0].ID)
	}
}

func TestFilterVideos_LiveStatus(t *testing.T) {
	videos := []VideoMeta{
		{ID: "upcoming", LiveStatus: "is_upcoming", Duration: 0},
		{ID: "live", LiveStatus: "is_live", Duration: 0},
		{ID: "was-live", LiveStatus: "was_live", Duration: 1800},
		{ID: "normal", LiveStatus: "", Duration: 600},
	}
	got := FilterVideos(videos, config.FilterConfig{})
	if len(got) != 2 {
		t.Errorf("LiveStatus: got %d videos, want 2", len(got))
	}
	ids := map[string]bool{}
	for _, v := range got {
		ids[v.ID] = true
	}
	if !ids["was-live"] {
		t.Error("LiveStatus: expected 'was-live' to be included")
	}
	if !ids["normal"] {
		t.Error("LiveStatus: expected 'normal' to be included")
	}
	if ids["upcoming"] {
		t.Error("LiveStatus: 'upcoming' should be filtered out")
	}
	if ids["live"] {
		t.Error("LiveStatus: 'live' should be filtered out")
	}
}
