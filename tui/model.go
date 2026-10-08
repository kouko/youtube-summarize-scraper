package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/kouko/youtube-summarize-scraper/config"
	"github.com/kouko/youtube-summarize-scraper/pipeline"
)

// Panel indicates which panel currently has focus.
type FocusedPanel int

const (
	PanelFilePicker FocusedPanel = iota
	PanelConfig
	PanelStatus
	PanelEvents
)

// PanelCount is the number of panels.
const PanelCount = 4

// Model is the main TUI model.
type Model struct {
	// State shared with event bridge
	state *AppState

	// UI components
	filePicker *FilePickerModel
	configView *ConfigView

	// Layout
	width  int
	height int
	focus  FocusedPanel

	// Pipeline control
	ctx       context.Context
	cancel    context.CancelFunc
	isRunning bool

	// Styles
	styles Styles

	// Error message (for config parsing errors, etc.)
	err error
}

// Styles holds lipgloss styles for the TUI.
type Styles struct {
	PanelBorder    lipgloss.Style
	PanelTitle     lipgloss.Style
	FocusedBorder  lipgloss.Style
	FocusedTitle   lipgloss.Style
	ErrorStyle     lipgloss.Style
	KeyHintStyle   lipgloss.Style
	ErrorHighlight lipgloss.Style
	SuccessStyle   lipgloss.Style
	WarningStyle   lipgloss.Style
}

// DefaultStyles returns the default lipgloss styles.
func DefaultStyles() Styles {
	return Styles{
		PanelBorder: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")),
		PanelTitle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			Bold(true),
		FocusedBorder: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("33")).
			Bold(true),
		FocusedTitle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("33")).
			Bold(true),
		ErrorStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true),
		KeyHintStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")),
		ErrorHighlight: lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true),
		SuccessStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("46")),
		WarningStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("226")),
	}
}

// NewModel creates a new TUI model.
func NewModel(state *AppState) *Model {
	m := &Model{
		state:      state,
		filePicker: NewFilePickerModel(),
		focus:      PanelFilePicker,
		styles:     DefaultStyles(),
	}

	// Initialize config view with empty content
	m.configView = NewConfigView("")

	return m
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		m.filePicker.Init(),
		tickCmd(),
	)
}

// tickCmd returns a command that sends a TickMsg every 250ms.
func tickCmd() tea.Cmd {
	return tea.Every(250*time.Millisecond, func(t time.Time) tea.Msg {
		return TickMsg{}
	})
}

// Update handles messages.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case TickMsg:
		// Refresh state from AppState
		return m, nil

	case ConfigSelectedMsg:
		m.handleConfigSelected(msg.Path)
		return m, nil

	case StartRunMsg:
		if !m.isRunning && m.state.ConfigPath != "" {
			m.isRunning = true
			m.state.SetRunning(true)
			m.err = nil
			// Start pipeline in background
			m.ctx, m.cancel = context.WithCancel(context.Background())
			go func() {
				defer func() { m.cancel() }()
				// Run pipeline with selected config
				if err := runPipelineWithConfig(m.state.ConfigPath, m.state); err != nil {
					m.state.AddRecentEvent(fmt.Sprintf("ERROR: %v", err))
				}
				m.state.SetRunning(false)
			}()
		}
		return m, nil

	case StopRunMsg:
		if m.isRunning {
			m.cancel()
			m.isRunning = false
			m.state.SetRunning(false)
		}
		return m, nil

	case PauseRunMsg:
		// TODO: implement pause if needed
		return m, nil

	case QuitConfirmMsg:
		// User confirmed quit while pipeline is running: cancel and exit.
		if m.cancel != nil {
			m.cancel()
		}
		m.isRunning = false
		m.state.SetRunning(false)
		return m, tea.Quit
	}

	return m, nil
}

// handleKey handles keyboard input.
func (m *Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		if m.isRunning {
			// Return a message asking for confirmation
			return m, func() tea.Msg { return QuitConfirmMsg{} }
		}
		return m, tea.Quit

	case "r":
		if !m.isRunning && m.state.ConfigPath != "" {
			return m, func() tea.Msg { return StartRunMsg{} }
		}
		return m, nil

	case "p":
		if m.isRunning {
			return m, func() tea.Msg { return PauseRunMsg{} }
		}
		return m, nil

	case "c":
		if m.configView != nil {
			m.configView.Toggle()
			return m, nil
		}
		return m, nil

	case "tab":
		m.focus = (m.focus + 1) % PanelCount
		return m, nil

	case "shift+tab":
		m.focus = (m.focus - 1 + PanelCount) % PanelCount
		return m, nil

	case "up", "k":
		return m.handleUp(msg)
	case "down", "j":
		return m.handleDown(msg)
	}
	return m, nil
}

func (m *Model) handleUp(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch m.focus {
	case PanelFilePicker:
		return m.filePicker.Update(msg)
	case PanelConfig:
		// Scroll up in config view (if implemented)
		return m, nil
	case PanelEvents:
		// Scroll up in events
		return m, nil
	}
	return m, nil
}

func (m *Model) handleDown(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch m.focus {
	case PanelFilePicker:
		return m.filePicker.Update(msg)
	case PanelConfig:
		return m, nil
	case PanelEvents:
		return m, nil
	}
	return m, nil
}

// handleConfigSelected handles a config file selection.
func (m *Model) handleConfigSelected(path string) {
	content, err := os.ReadFile(path)
	if err != nil {
		m.configView = NewConfigView(fmt.Sprintf("Error reading file: %v", err))
		return
	}
	m.configView = NewConfigView(string(content))
	m.state.UpdateConfig(path, string(content))
}

// View renders the TUI.
func (m *Model) View() tea.View {
	if m.width == 0 || m.height == 0 {
		return tea.NewView("Loading...")
	}

	// Snapshot the shared state so rendering never races with the
	// event-bridge goroutine that updates it.
	s := m.state.Snapshot()

	// Calculate panel dimensions
	// Layout: 4 panels in 2x2 grid
	leftWidth := m.width / 2
	rightWidth := m.width - leftWidth
	topHeight := m.height / 2
	bottomHeight := m.height - topHeight - 1 // -1 for hint line

	// Render each panel
	filePickerView := m.renderFilePicker(leftWidth, topHeight)
	configView := m.renderConfig(leftWidth, bottomHeight)
	statusView := m.renderStatus(s, rightWidth, topHeight)
	eventsView := m.renderEvents(s, rightWidth, bottomHeight)

	// Combine into 2x2 grid
	topRow := lipgloss.JoinHorizontal(lipgloss.Top, filePickerView, statusView)
	bottomRow := lipgloss.JoinHorizontal(lipgloss.Top, configView, eventsView)
	mainView := lipgloss.JoinVertical(lipgloss.Left, topRow, bottomRow)

	// Add hint line at bottom
	hint := m.renderHintLine()
	v := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, mainView, hint))
	v.AltScreen = true
	return v
}

func (m *Model) renderFilePicker(w, h int) string {
	title := "Config File"
	if m.focus == PanelFilePicker {
		title = "▸ " + title
	}
	content := m.filePicker.View().Content
	return m.panelStyle(m.focus == PanelFilePicker, w, h).Render(
		m.styles.PanelTitle.Render(title) + "\n" + content,
	)
}

func (m *Model) renderConfig(w, h int) string {
	title := "Config"
	if m.focus == PanelConfig {
		title = "▸ " + title
	}
	var content string
	if m.configView == nil {
		content = "(no config selected)"
	} else {
		content = m.configView.Render()
	}
	return m.panelStyle(m.focus == PanelConfig, w, h).Render(
		m.styles.PanelTitle.Render(title) + "\n" + content,
	)
}

func (m *Model) renderStatus(s StateSnapshot, w, h int) string {
	title := "Execution Status"
	if m.focus == PanelStatus {
		title = "▸ " + title
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Watch Iteration: %d\n", s.WatchIter)
	fmt.Fprintf(&b, "Queue Length: %d\n", s.QueueLen)
	fmt.Fprintf(&b, "Processed: %d  Skipped: %d  Failed: %d  Partial: %d\n",
		s.Stats.Success, s.Stats.Skipped, s.Stats.Failed, s.Stats.Partial)
	if s.CurrentVideo != "" {
		fmt.Fprintf(&b, "Current: %s", s.CurrentVideo)
		if s.CurrentStage != "" {
			fmt.Fprintf(&b, " (%s)", s.CurrentStage)
		}
		b.WriteString("\n")
	}
	if s.IsRunning {
		b.WriteString("Status: Running\n")
	} else {
		b.WriteString("Status: Stopped\n")
	}

	return m.panelStyle(m.focus == PanelStatus, w, h).Render(
		m.styles.PanelTitle.Render(title) + "\n" + b.String(),
	)
}

func (m *Model) renderEvents(s StateSnapshot, w, h int) string {
	title := "Recent Events"
	if m.focus == PanelEvents {
		title = "▸ " + title
	}

	var b strings.Builder
	events := s.RecentEvents
	// Show last 10 events
	start := len(events) - 10
	if start < 0 {
		start = 0
	}
	for i := start; i < len(events); i++ {
		line := events[i]
		if strings.Contains(line, "level=ERROR") || strings.Contains(line, "ERROR") {
			b.WriteString(m.styles.ErrorHighlight.Render("[ERROR] "+line) + "\n")
		} else {
			b.WriteString(line + "\n")
		}
	}
	if len(events) == 0 {
		b.WriteString("(no events)")
	}

	return m.panelStyle(m.focus == PanelEvents, w, h).Render(
		m.styles.PanelTitle.Render(title) + "\n" + b.String(),
	)
}

// panelStyle returns the bordered style for a panel, focused or not.
func (m *Model) panelStyle(focused bool, w, h int) lipgloss.Style {
	st := m.styles.PanelBorder
	if focused {
		st = m.styles.FocusedBorder
	}
	return st.Width(w - 2).Height(h - 2)
}

func (m *Model) renderHintLine() string {
	hints := []string{
		"↑↓ Navigate",
		"Tab Switch panel",
		"c Toggle config view",
		"r Run",
		"p Pause",
		"q Quit",
	}
	return m.styles.KeyHintStyle.Render(strings.Join(hints, "  "))
}

// QuitConfirmMsg is sent when user confirms quitting while pipeline is running.
type QuitConfirmMsg struct{}

// StartRunMsg signals to start the pipeline.
type StartRunMsg struct{}

// StopRunMsg signals to stop the pipeline.
type StopRunMsg struct{}

// PauseRunMsg signals to pause the pipeline.
type PauseRunMsg struct{}

// TickMsg is sent periodically to trigger a re-render.
type TickMsg struct{}

// runPipelineWithConfig loads the selected config and runs ProcessBatchStreaming
// with its slog output captured by bridge, which feeds state.
func runPipelineWithConfig(configPath string, state *AppState) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	p, err := pipeline.NewPipeline(cfg, false, false)
	if err != nil {
		return fmt.Errorf("initializing pipeline: %w", err)
	}
	defer p.Shutdown()

	stats, err := p.ProcessBatchStreaming()
	if stats != nil {
		state.UpdateStats(*stats)
	}
	if err != nil {
		return fmt.Errorf("batch processing: %w", err)
	}
	state.AddRecentEvent(fmt.Sprintf(
		"batch complete: %d success, %d skipped, %d partial, %d failed",
		stats.Success, stats.Skipped, stats.Partial, stats.Failed,
	))
	return nil
}
