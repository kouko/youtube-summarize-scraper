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
)

var rootCmd = &cobra.Command{
	Use:   "ytss",
	Short: "YouTube Summarize Scraper",
	Long:  "A CLI tool that batch-processes YouTube channels to download subtitles, transcribe audio, and generate LLM-powered summaries.",
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "./config.yaml", "config file path")
	rootCmd.PersistentFlags().StringVarP(&outputDir, "output", "o", "", "output directory (overrides config)")
	rootCmd.PersistentFlags().StringVar(&llmOverride, "llm", "", "override LLM backend (ollama/llamacpp/claude-api/gemini-cli/openai-compat)")
	rootCmd.PersistentFlags().StringVar(&cookieFile, "cookie-file", "", "path to cookie.txt (Netscape format)")
	rootCmd.PersistentFlags().StringVar(&cookieBrowser, "cookie-browser", "", "auto-extract cookie from browser (chrome/firefox/safari/edge/brave)")
	rootCmd.PersistentFlags().BoolVar(&forceFlag, "force", false, "force re-process even if output already exists")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "list videos that would be processed without executing")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose logging")
}
