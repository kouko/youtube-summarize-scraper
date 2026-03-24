package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kouko/youtube-summarize-scraper/pipeline"
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Batch process all channels from config",
	Long:  "Read config.yaml and process the latest N videos from each configured channel.",
	RunE: func(cmd *cobra.Command, args []string) error {
		setupLogging(verbose)

		cfg := loadConfig(cfgFile)
		applyOverrides(cfg)

		if len(cfg.Channels) == 0 && len(cfg.Playlists) == 0 {
			return fmt.Errorf("no channels or playlists configured in %s", cfgFile)
		}

		p, err := pipeline.NewPipeline(cfg, forceFlag, dryRun)
		if err != nil {
			return fmt.Errorf("initializing pipeline: %w", err)
		}

		if !cfg.Batch.Watch {
			// Single run mode.
			stats, err := p.ProcessBatch()
			if err != nil {
				return fmt.Errorf("batch processing: %w", err)
			}
			printStats(stats)
			return nil
		}

		// Watch mode: loop until signal received.
		return runWatch(p, cfgFile, cfg.Batch.WatchInterval)
	},
}

// runWatch runs ProcessBatch in a loop with the given interval (minutes).
// It handles SIGINT/SIGTERM for graceful shutdown between iterations,
// and also signals the pipeline to stop mid-batch if interrupted.
func runWatch(p *pipeline.Pipeline, cfgPath string, intervalMin int) error {
	interval := time.Duration(intervalMin) * time.Minute

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	slog.Info("watch mode started", "interval", interval)

	iteration := 0
	for {
		iteration++
		slog.Info(fmt.Sprintf("watch: iteration %d starting", iteration))

		p.ResetContext()
		p.ReloadConfig(cfgPath)
		p.RebuildIndex()

		// Run ProcessBatch in a goroutine so we can listen for signals concurrently.
		doneCh := make(chan batchResult, 1)
		go func() {
			stats, err := p.ProcessBatch()
			doneCh <- batchResult{stats: stats, err: err}
		}()

		// Wait for either batch completion or signal.
		var result batchResult
		select {
		case result = <-doneCh:
			// Batch completed normally.
		case sig := <-sigCh:
			slog.Info("watch: received signal, stopping current batch", "signal", sig)
			p.Shutdown()
			result = <-doneCh // Wait for batch to finish current video.
			printStats(result.stats)
			slog.Info("watch: shutdown complete")
			return nil
		}

		if result.err != nil {
			slog.Error("watch: batch processing failed", "iteration", iteration, "error", result.err)
		} else {
			printStats(result.stats)
		}

		slog.Info(fmt.Sprintf("watch: iteration %d complete, sleeping %s", iteration, interval))

		select {
		case sig := <-sigCh:
			slog.Info("watch: received signal, shutting down", "signal", sig)
			return nil
		case <-time.After(interval):
			continue
		}
	}
}

type batchResult struct {
	stats *pipeline.Stats
	err   error
}

func init() {
	rootCmd.AddCommand(runCmd)
}
