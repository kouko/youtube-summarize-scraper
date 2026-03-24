package cmd

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/kouko/youtube-summarize-scraper/config"
	"github.com/kouko/youtube-summarize-scraper/pipeline"
)

// loadConfig tries to load the config file at the given path.
// If the file does not exist, it returns DefaultConfig.
func loadConfig(path string) *config.Config {
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
		cfg.OutputDir = outputDir
	}
	if llmOverride != "" {
		cfg.LLM.Provider = llmOverride
	}
	if cookieFile != "" {
		cfg.Cookie.File = cookieFile
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
	fmt.Printf("completed: %d success, %d skipped, %d failed\n",
		stats.Success, stats.Skipped, stats.Failed)

	if verbose && len(stats.Errors) > 0 {
		fmt.Println("\nFailed videos:")
		for _, ve := range stats.Errors {
			fmt.Printf("  - %s (%s): %v\n", ve.VideoID, ve.Title, ve.Err)
		}
	}
}
