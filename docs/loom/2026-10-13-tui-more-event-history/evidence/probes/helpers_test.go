package probes

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kouko/youtube-summarize-scraper/tui"
)

// pressKey creates a key press message.
func pressKey(s string) tea.KeyPressMsg {
	switch s {
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEsc}
	default:
		r := []rune(s)
		if len(r) == 1 {
			return tea.KeyPressMsg{Code: r[0], Text: s}
		}
		return tea.KeyPressMsg{Text: s}
	}
}

// viewContent renders the view and returns the content string.
func viewContent(t *testing.T, m *tui.Model) string {
	t.Helper()
	v := m.View()
	if v.Content == "" {
		t.Fatal("View() returned empty content")
	}
	return v.Content
}

// cycleToEventsPanel tabs from the initial PanelFilePicker (0) to PanelEvents (3).
func cycleToEventsPanel(m *tui.Model) {
	m.Update(pressKey("tab")) // -> PanelConfig (1)
	m.Update(pressKey("tab")) // -> PanelStatus (2)
	m.Update(pressKey("tab")) // -> PanelEvents (3)
}

// newSizedEventsModelWithState returns a model focused on the events panel
// built over the given state (so the caller can add events directly).
func newSizedEventsModelWithState(state *tui.AppState) *tui.Model {
	m := tui.NewModel(state)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	cycleToEventsPanel(m)
	return m
}
