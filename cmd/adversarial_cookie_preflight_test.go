package cmd

// concern: false startup stop — the preflight checks a cookie setting that no yt-dlp call ever reads, killing an unattended run that would have succeeded.

import (
	"testing"

	"github.com/kouko/youtube-summarize-scraper/config"
	"github.com/spf13/cobra"
)

// TestPreflight_ChannelOnlyBrowserCookie_NoLookup pins REQ-3 / Acceptance 3:
// a channel entry's own cookie block is never consumed by the pipeline (only
// the global cookie and playlist cookies reach yt-dlp), so a run whose only
// browser setting sits on a channel entry reads no browser cookies and must
// not be stopped by a locked keychain.
func TestPreflight_ChannelOnlyBrowserCookie_NoLookup(t *testing.T) {
	calls := stubKeychain(t, 36)
	cfg := config.DefaultConfig()
	cfg.Channels = []config.ChannelConfig{{URL: "https://www.youtube.com/@x", Cookie: &config.CookieConfig{Browser: "chrome"}}}

	if err := preflightCookieKeychain(&cobra.Command{}, cfg, true); err != nil {
		t.Errorf("run stopped for a cookie setting nothing reads: %v", err)
	}
	if len(*calls) != 0 {
		t.Errorf("lookup calls = %v, want none", *calls)
	}
}
