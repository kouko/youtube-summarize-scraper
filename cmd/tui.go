package cmd

import (
	"io"
	"log/slog"

	tea "charm.land/bubbletea/v2"
	"github.com/kouko/youtube-summarize-scraper/tui"
	"github.com/spf13/cobra"
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Launch the interactive terminal UI",
	Long:  "Launch the interactive TUI: pick a config file, inspect its content, and run the pipeline while watching live status.",
	RunE: func(cmd *cobra.Command, args []string) error {
		state := tui.NewAppState()

		// Route slog output through the event bridge into the TUI's Recent
		// Events panel and live status. Nothing is written to stderr while the
		// alternate screen is active, so pipeline logs never garble the UI.
		bridge := tui.NewEventBridge(io.Discard, state, 100)
		slog.SetDefault(slog.New(slog.NewTextHandler(bridge, nil)))

		// The model owns the bridge and closes it on quit, so the bridge's
		// consumer goroutine never outlives the program.
		model := tui.NewModelWithBridge(state, bridge)
		_, err := tea.NewProgram(model).Run()
		return err
	},
}

func init() {
	rootCmd.AddCommand(tuiCmd)
}
