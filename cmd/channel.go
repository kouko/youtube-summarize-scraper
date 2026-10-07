package cmd

import (
	"fmt"
	"strings"

	"github.com/kouko/youtube-summarize-scraper/config"
	"github.com/kouko/youtube-summarize-scraper/pipeline"
	"github.com/spf13/cobra"
)

var (
	channelCount int
)

var channelCmd = &cobra.Command{
	Use:   "channel [URL or @handle]",
	Short: "Summarize latest N videos from a channel",
	Long:  "Fetch the latest N videos from a YouTube channel and generate summaries for each.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		setupLogging(verbose)

		cfg := loadConfig(cfgFile)
		applyOverrides(cfg)
		if err := preflightCookieKeychain(cmd, cfg, false); err != nil {
			return err
		}

		input := args[0]
		channelURL := input
		if strings.HasPrefix(input, "@") {
			channelURL = "https://www.youtube.com/" + input
		}

		p, err := pipeline.NewPipeline(cfg, forceFlag, dryRun)
		if err != nil {
			return fmt.Errorf("initializing pipeline: %w", err)
		}

		chCfg := &config.ChannelConfig{
			URL:   channelURL,
			Count: channelCount,
		}

		stats, err := p.ProcessChannel(channelURL, channelCount, chCfg)
		if err != nil {
			return fmt.Errorf("processing channel: %w", err)
		}

		printStats(stats)
		return nil
	},
}

func init() {
	channelCmd.Flags().IntVarP(&channelCount, "count", "n", 5, "number of latest videos to process")
	rootCmd.AddCommand(channelCmd)
}
