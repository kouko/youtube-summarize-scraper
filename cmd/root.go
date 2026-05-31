package cmd

import (
	"github.com/spf13/cobra"
)

var (
	cfgFile       string
	outputDir     string
	llmOverride   string
	cookieFile    string
	cookieBrowser string
	forceFlag     bool
	dryRun        bool
	verbose       bool
	watchFlag        bool
	intervalFlag     int
	fetchConcurrency int
)

var rootCmd = &cobra.Command{
	Use:   "ytss",
	Short: "YouTube Summarize Scraper",
	Long:  "A CLI tool that batch-processes YouTube channels to download subtitles, transcribe audio, and generate LLM-powered summaries.",
}

// SetVersion sets the version string displayed by --version.
func SetVersion(v string) {
	rootCmd.Version = v
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "./config.yaml", "config file path")
	rootCmd.PersistentFlags().StringVarP(&outputDir, "output", "o", "", "output directory (overrides config)")
	rootCmd.PersistentFlags().StringVar(&llmOverride, "llm", "", "override LLM backend (ollama/llamacpp/claude-api/claude-code/gemini-cli/antigravity-cli/qwen-code/openai-compat)")
	rootCmd.PersistentFlags().StringVar(&cookieFile, "cookie-file", "", "path to cookie.txt (Netscape format)")
	rootCmd.PersistentFlags().StringVar(&cookieBrowser, "cookie-browser", "", "auto-extract cookie from browser (chrome/firefox/safari/edge/brave)")
	rootCmd.PersistentFlags().BoolVar(&forceFlag, "force", false, "force re-process even if output already exists")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "list videos that would be processed without executing")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose logging")
	rootCmd.PersistentFlags().BoolVar(&watchFlag, "watch", false, "run in watch mode (check for new videos periodically)")
	rootCmd.PersistentFlags().IntVar(&intervalFlag, "interval", 0, "watch interval in minutes (overrides config, default: 10)")
	rootCmd.PersistentFlags().IntVar(&fetchConcurrency, "fetch-concurrency", 0, "max parallel yt-dlp list fetches (overrides config, default: 3)")
}
