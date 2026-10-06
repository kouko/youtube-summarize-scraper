package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"runtime"

	"github.com/kouko/youtube-summarize-scraper/config"
	"github.com/kouko/youtube-summarize-scraper/fetcher"
	"github.com/kouko/youtube-summarize-scraper/pipeline"
	"github.com/spf13/cobra"
)

// Injected into the startup cookie preflight; tests replace them.
var (
	keychainGOOS                          = runtime.GOOS
	keychainLookup fetcher.KeychainLookup = fetcher.LookupMacKeychain
)

// preflightCookieKeychain stops the command before any work when a configured
// Chrome-family browser's cookie key cannot be read from the macOS keychain.
// It checks the global cookie, plus every playlist's own cookie when
// includeEntries is set (run). Channel entries' cookies are not checked: no
// fetch consumes channels[].cookie, so checking it would block working runs. On failure it silences cobra's usage block
// so the error prints as a single line.
func preflightCookieKeychain(cmd *cobra.Command, cfg *config.Config, includeEntries bool) error {
	cookies := []config.CookieConfig{cfg.Cookie}
	if includeEntries {
		for _, pl := range cfg.Playlists {
			if pl.Cookie != nil {
				cookies = append(cookies, *pl.Cookie)
			}
		}
	}
	if err := fetcher.CheckBrowserCookieKeychain(keychainGOOS, keychainLookup, cookies); err != nil {
		cmd.SilenceUsage = true
		return err
	}
	return nil
}

// loadConfig tries to load the config file at the given path.
// If the file does not exist, it returns DefaultConfig.
func loadConfig(path string) *config.Config {
	path = config.ExpandHome(path)
	cfg, err := config.Load(path)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Debug("config file not found, using defaults", "path", path)
			return config.DefaultConfig()
		}
		slog.Warn("failed to load config, using defaults", "path", path, "error", err)
		return config.DefaultConfig()
	}
	return cfg
}

// applyOverrides applies CLI flag overrides to the config.
func applyOverrides(cfg *config.Config) {
	if outputDir != "" {
		cfg.OutputDir = config.ExpandHome(outputDir)
	}
	if llmOverride != "" {
		cfg.LLM.Provider.SetPrimary(llmOverride)
	}
	if cookieFile != "" {
		cfg.Cookie.File = config.ExpandHome(cookieFile)
	}
	if cookieBrowser != "" {
		cfg.Cookie.Browser = cookieBrowser
	}
	if watchFlag {
		cfg.Batch.Watch = true
	}
	if intervalFlag > 0 {
		cfg.Batch.WatchInterval = intervalFlag
	}
	if fetchConcurrency > 0 {
		cfg.Batch.FetchConcurrency = fetchConcurrency
	}
}

// setupLogging configures the default slog level.
func setupLogging(verboseFlag bool) {
	level := slog.LevelInfo
	if verboseFlag {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	})))
}

// printStats prints the completion summary from pipeline Stats.
func printStats(stats *pipeline.Stats) {
	fmt.Printf("completed: %d success, %d skipped, %d partial, %d failed\n",
		stats.Success, stats.Skipped, stats.Partial, stats.Failed)

	if verbose && len(stats.Errors) > 0 {
		fmt.Println("\nFailed videos:")
		for _, ve := range stats.Errors {
			fmt.Printf("  - %s (%s): %v\n", ve.VideoID, ve.Title, ve.Err)
		}
	}
}
