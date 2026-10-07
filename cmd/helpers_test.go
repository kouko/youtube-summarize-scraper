package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kouko/youtube-summarize-scraper/config"
	"github.com/kouko/youtube-summarize-scraper/fetcher"
	"github.com/spf13/cobra"
)

// stubKeychain swaps the preflight's OS and lookup for the test and records
// every keyring name looked up.
func stubKeychain(t *testing.T, code int) *[]string {
	t.Helper()
	var calls []string
	origGOOS, origLookup := keychainGOOS, keychainLookup
	keychainGOOS = "darwin"
	keychainLookup = func(name string) (int, error) {
		calls = append(calls, name)
		return code, nil
	}
	t.Cleanup(func() { keychainGOOS, keychainLookup = origGOOS, origLookup })
	return &calls
}

// A1 positive: run with a playlist reading Chrome cookies and a locked
// keychain stops with fetcher's one-line message before the pipeline is built.
func TestRunCmd_LockedPlaylistChromeCookie_StopsBeforePipeline(t *testing.T) {
	calls := stubKeychain(t, 36)
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	yaml := "output_dir: " + dir + "\nplaylists:\n  - url: https://www.youtube.com/playlist?list=PLx\n    cookie:\n      browser: chrome\n"
	if err := os.WriteFile(cfgPath, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	origCfg := cfgFile
	cfgFile = cfgPath
	t.Cleanup(func() { cfgFile = origCfg; runCmd.SilenceUsage = false })

	err := runCmd.RunE(runCmd, nil)

	want := fetcher.CheckBrowserCookieKeychain("darwin",
		func(string) (int, error) { return 36, nil },
		[]config.CookieConfig{{Browser: "chrome"}})
	if err == nil || err.Error() != want.Error() {
		t.Fatalf("err = %v, want %v", err, want)
	}
	if strings.Contains(err.Error(), "\n") {
		t.Errorf("error should be one line: %q", err)
	}
	if !runCmd.SilenceUsage {
		t.Error("SilenceUsage should be set on preflight failure")
	}
	if len(*calls) != 1 || (*calls)[0] != "Chrome" {
		t.Errorf("lookup calls = %v, want [Chrome]", *calls)
	}
}

// A1 negative: channel/video check only the global cookie, never a
// playlist-only browser setting.
func TestPreflight_GlobalOnly_IgnoresPlaylistBrowser(t *testing.T) {
	calls := stubKeychain(t, 36)
	cfg := config.DefaultConfig()
	cfg.Playlists = []config.PlaylistConfig{{URL: "u", Cookie: &config.CookieConfig{Browser: "chrome"}}}
	cfg.Channels = []config.ChannelConfig{{URL: "c", Cookie: &config.CookieConfig{Browser: "brave"}}}

	if err := preflightCookieKeychain(&cobra.Command{}, cfg, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(*calls) != 0 {
		t.Errorf("lookup calls = %v, want none", *calls)
	}
}

// A2 positive: a readable key lets the command proceed with no change.
func TestPreflight_Readable_Proceeds(t *testing.T) {
	calls := stubKeychain(t, 0)
	cfg := config.DefaultConfig()
	cfg.Cookie.Browser = "Chrome:Default"
	cmd := &cobra.Command{}

	if err := preflightCookieKeychain(cmd, cfg, true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.SilenceUsage {
		t.Error("SilenceUsage should stay unset when readable")
	}
	if len(*calls) != 1 {
		t.Errorf("lookup calls = %v, want one", *calls)
	}
}

// A3 positive: no cookie settings means no keychain lookup.
func TestPreflight_NoCookies_NoLookup(t *testing.T) {
	calls := stubKeychain(t, 36)
	if err := preflightCookieKeychain(&cobra.Command{}, config.DefaultConfig(), true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(*calls) != 0 {
		t.Errorf("lookup calls = %v, want none", *calls)
	}
}

// A3 negative: a cookie file wins over a browser, so no lookup runs.
func TestPreflight_CookieFileWins_NoLookup(t *testing.T) {
	calls := stubKeychain(t, 36)
	cfg := config.DefaultConfig()
	cfg.Cookie = config.CookieConfig{File: "/tmp/c.txt", Browser: "chrome"}
	cfg.Playlists = []config.PlaylistConfig{{URL: "u", Cookie: &config.CookieConfig{File: "/tmp/p.txt", Browser: "chrome"}}}

	if err := preflightCookieKeychain(&cobra.Command{}, cfg, true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(*calls) != 0 {
		t.Errorf("lookup calls = %v, want none", *calls)
	}
}
