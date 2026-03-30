package pipeline

import (
	"testing"

	"github.com/kouko/youtube-summarize-scraper/config"
)

func TestPlaylistToChannelCfg_PropagatesFilter(t *testing.T) {
	minDur := 60
	pl := &config.PlaylistConfig{
		URL:   "https://www.youtube.com/playlist?list=PL1",
		Count: 5,
		Filter: &config.FilterOverride{
			MinDuration: &minDur,
		},
	}

	ch := playlistToChannelCfg(pl)

	if ch.Filter == nil {
		t.Fatal("Filter should be propagated, got nil")
	}
	if ch.Filter.MinDuration == nil || *ch.Filter.MinDuration != 60 {
		t.Errorf("Filter.MinDuration: got %v, want ptr(60)", ch.Filter.MinDuration)
	}
}

func TestPlaylistToChannelCfg_NilFilter(t *testing.T) {
	pl := &config.PlaylistConfig{
		URL: "https://www.youtube.com/playlist?list=PL2",
	}

	ch := playlistToChannelCfg(pl)

	if ch.Filter != nil {
		t.Errorf("Filter should be nil when playlist has no filter, got %+v", ch.Filter)
	}
}

func TestPlaylistToChannelCfg_Nil(t *testing.T) {
	ch := playlistToChannelCfg(nil)
	if ch != nil {
		t.Errorf("expected nil for nil playlist, got %+v", ch)
	}
}
