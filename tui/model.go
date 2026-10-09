package tui

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	// bubbletea v2.0.10 (charm.land/bubbletea/v2): a Model is Init()/Update()/
	// View(); View returns tea.View (a struct, not a string); full-screen is
	// enabled by setting View.AltScreen = true (there is no WithAltScreen
	// option in v2); key events arrive as tea.KeyPressMsg; tea.Every drives
	// the 250ms re-render tick; tea.Batch groups init commands.
	tea "charm.land/bubbletea/v2"
	// lipgloss v2.0.6: Style.Border(b Border, sides ...bool) takes a Border
	// value first (v1's Border(cond, ...) does not compile); JoinHorizontal/
	// JoinVertical lay the four panels out.
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

	// bridge tees slog output into state; closed on quit so its consumer
	// goroutine does not leak.
	bridge *EventBridge

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

	// confirmPending: a first q while running armed the quit confirmation.
	// The next q confirms (QuitConfirmMsg); any other key disarms it.
	confirmPending bool

	// pickerOpen: the file-picker popup overlay is showing (opened by Enter
	// on the focused config card). While open it owns all keys.
	pickerOpen bool

	// configScroll / eventsScroll: scroll offsets for the bottom panels
	// (0 = newest / top). ↑ increases the offset (older), ↓ decreases.
	configScroll int
	eventsScroll int

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
	return NewModelWithBridge(state, nil)
}

// NewModelWithBridge creates a TUI model that owns bridge (may be nil) and
// closes it when the program quits, so the bridge consumer never leaks.
func NewModelWithBridge(state *AppState, bridge *EventBridge) *Model {
	m := &Model{
		state:      state,
		bridge:     bridge,
		filePicker: NewFilePickerModel(),
		focus:      PanelFilePicker,
		styles:     DefaultStyles(),
	}

	// Initialize config view with empty content
	m.configView = NewConfigView("")

	return m
}

// closeBridge shuts down the event-bridge consumer, if one is wired.
func (m *Model) closeBridge() {
	if m.bridge != nil {
		m.bridge.Close()
	}
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd {
	m.applyPanelHeights()
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
	var cmds []tea.Cmd

	// The file picker owns messages the main model does not handle itself:
	// readDirMsg (the directory listing from its Init command), WindowSizeMsg
	// (AutoHeight), and its key navigation while the popup is open or the
	// picker panel is focused — except Enter, which on the focused card opens
	// the popup instead of navigating the (hidden) listing.
	isKey := false
	keyStr := ""
	if k, ok := msg.(tea.KeyPressMsg); ok {
		isKey = true
		keyStr = k.String()
	}
	if !isKey || m.pickerOpen || (m.focus == PanelFilePicker && keyStr != "enter") {
		if picked, cmd := m.filePicker.Update(msg); cmd != nil {
			m.filePicker = picked.(*FilePickerModel)
			cmds = append(cmds, cmd)
		} else {
			m.filePicker = picked.(*FilePickerModel)
		}
	}

	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.applyPanelHeights()

	case tea.KeyPressMsg:
		_, cmd = m.handleKey(msg)

	case TickMsg:
		// Refresh happens on every render; nothing to do here.

	case ConfigSelectedMsg:
		m.pickerOpen = false
		m.handleConfigSelected(msg.Path)

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
				if err := runPipelineWithConfig(m.state.ConfigPath, m.state, m.ctx); err != nil {
					m.state.AddRecentEvent(fmt.Sprintf("ERROR: %v", err))
				}
				m.state.SetRunning(false)
			}()
		}

	case StopRunMsg:
		if m.isRunning {
			m.cancel()
			m.isRunning = false
			m.state.SetRunning(false)
		}

	case QuitConfirmMsg:
		// User confirmed quit while pipeline is running: cancel and exit.
		if m.cancel != nil {
			m.cancel()
		}
		m.isRunning = false
		m.state.SetRunning(false)
		m.closeBridge()
		cmd = tea.Quit
	}

	if cmd != nil {
		cmds = append(cmds, cmd)
	}
	if len(cmds) > 0 {
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

// handleKey handles keyboard input.
func (m *Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// While the picker popup is open it owns navigation; Esc closes it and
	// q is not a global quit. (Other keys are forwarded by Update.)
	if m.pickerOpen {
		if key == "esc" {
			m.pickerOpen = false
		}
		return m, nil
	}

	// Any key other than q/ctrl+c cancels a pending quit confirmation.
	if key != "q" && key != "ctrl+c" {
		m.confirmPending = false
	}
	switch key {
	case "enter":
		// Enter on the focused config card opens the file-picker popup.
		if m.focus == PanelFilePicker {
			m.pickerOpen = true
		}
		return m, nil

	case "ctrl+c", "q":
		if m.isRunning {
			// First q arms the confirmation; the second q confirms it. Any
			// other key (handled below) disarms. This is deliberately not a
			// QuitConfirmMsg on the first press — Update treats that msg as
			// the confirmed quit and would cancel the run immediately.
			if !m.confirmPending {
				m.confirmPending = true
				return m, nil
			}
			m.confirmPending = false
			return m, func() tea.Msg { return QuitConfirmMsg{} }
		}
		m.closeBridge()
		return m, tea.Quit

	case "r":
		if !m.isRunning && m.state.ConfigPath != "" {
			return m, func() tea.Msg { return StartRunMsg{} }
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
	case "pgup":
		return m.pageFocused(-5)
	case "pgdown":
		return m.pageFocused(5)
	}
	return m, nil
}

// pageFocused pages the focused bottom panel by delta rows.
func (m *Model) pageFocused(delta int) (tea.Model, tea.Cmd) {
	switch m.focus {
	case PanelConfig:
		m.clampConfigScroll(m.configScroll + delta)
	case PanelEvents:
		m.clampEventsScroll(m.eventsScroll + delta)
	}
	return m, nil
}

// applyPanelHeights sizes the file picker to the top-left panel's inner
// rows so its listing scrolls inside the band instead of overflowing.
func (m *Model) applyPanelHeights() {
	if m.width == 0 || m.height == 0 {
		return
	}
	// The picker lives in the popup (3/4 of the terminal, borders + title
	// leave 4 rows), so size it to that overlay, not to the top panel.
	popupH := m.height * 3 / 4
	if popupH < 10 {
		popupH = 10
	}
	m.filePicker.SetHeight(popupH - 4)
}

func (m *Model) handleUp(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch m.focus {
	case PanelConfig:
		// ↑ reveals earlier config rows.
		m.clampConfigScroll(m.configScroll - 1)
	case PanelEvents:
		// ↑ reveals older events.
		m.clampEventsScroll(m.eventsScroll + 1)
	}
	// File-picker navigation is forwarded by Update; bottom panels scroll
	// above. Returning m keeps the main model as the program model.
	return m, nil
}

func (m *Model) handleDown(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch m.focus {
	case PanelConfig:
		// ↓ reveals later config rows.
		m.clampConfigScroll(m.configScroll + 1)
	case PanelEvents:
		// ↓ reveals newer events.
		m.clampEventsScroll(m.eventsScroll - 1)
	}
	return m, nil
}

// clampConfigScroll keeps the config offset within the rendered line count.
func (m *Model) clampConfigScroll(v int) {
	if m.configView == nil {
		m.configScroll = 0
		return
	}
	max := len(m.configView.Lines()) - 1
	if max < 0 {
		max = 0
	}
	if v < 0 {
		v = 0
	}
	if v > max {
		v = max
	}
	m.configScroll = v
}

// clampEventsScroll keeps the events offset within the recorded lines.
func (m *Model) clampEventsScroll(v int) {
	s := m.state.Snapshot()
	max := len(s.RecentEvents) - 1
	if max < 0 {
		max = 0
	}
	if v < 0 {
		v = 0
	}
	if v > max {
		v = max
	}
	m.eventsScroll = v
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
	// Layout: 4 panels in 2x2 grid + 1 hint line. panelStyle fixes each box
	// to exactly w x h rows (lipgloss Height counts the border; MaxHeight
	// clips overflow), so the two bands plus the hint fill the terminal:
	// topHeight + bottomHeight + 1 = m.height.
	leftWidth := m.width / 2
	rightWidth := m.width - leftWidth
	total := m.height - 1 // hint line
	// The top panels are content-sized (the config card is ~6 rows, the
	// status panel 8-9): the top band is a small fixed height and the bottom
	// band gets the rest, instead of two equal halves that left a tall empty
	// strip under the short card (user feedback 2026-10-09).
	topHeight := 9
	if topHeight > total/2 {
		topHeight = total / 2
	}
	bottomHeight := total - topHeight

	// Render each panel
	configCard := m.renderConfigCard(leftWidth, topHeight)
	configView := m.renderConfig(leftWidth, bottomHeight)
	statusView := m.renderStatus(s, rightWidth, topHeight)
	eventsView := m.renderEvents(s, rightWidth, bottomHeight)

	// Combine into 2x2 grid. The config card is a short box, so top-align the
	// row (lipgloss.JoinHorizontal(Top)) — the space below the card is free.
	topRow := lipgloss.JoinHorizontal(lipgloss.Top, configCard, statusView)
	bottomRow := lipgloss.JoinHorizontal(lipgloss.Top, configView, eventsView)
	mainView := lipgloss.JoinVertical(lipgloss.Left, topRow, bottomRow)

	// Add hint line at bottom
	hint := m.renderHintLine()
	content := lipgloss.JoinVertical(lipgloss.Left, mainView, hint)

	// While the picker popup is open, overlay it centered on the frame.
	if m.pickerOpen {
		content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
			m.renderPickerPopup(m.width, m.height))
	}
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// renderConfigCard renders the top-left short card: the current config path,
// its size, and the "open picker" hint. It never renders the file listing —
// the picker lives in the popup overlay instead.
func (m *Model) renderConfigCard(w, h int) string {
	title := "Config File"
	if m.focus == PanelFilePicker {
		title = "▸ " + title
	}
	var b strings.Builder
	if m.state.Snapshot().ConfigPath == "" {
		b.WriteString("(no config selected)\n")
	} else {
		fmt.Fprintf(&b, "%s\n", m.state.Snapshot().ConfigPath)
		if fi, err := os.Stat(m.state.Snapshot().ConfigPath); err == nil {
			if fi.Size() < 1024 {
				fmt.Fprintf(&b, "%d B\n", fi.Size())
			} else {
				fmt.Fprintf(&b, "%d kB\n", fi.Size()/1024)
			}
		}
	}
	b.WriteString("Enter 選擇設定檔")
	// The card fills the content-sized top band (height h, shared with the
	// status panel) so both boxes end on the same line — the "short card +
	// blank strip" of the previous layout is gone. Content stays at the top.
	return m.panelStyle(m.focus == PanelFilePicker, w, h).Render(
		m.styles.PanelTitle.Render(title) + "\n" + b.String(),
	)
}

// renderPickerPopup renders the file-picker overlay frame (centered, sized to
// the terminal).
func (m *Model) renderPickerPopup(w, h int) string {
	pw := w * 3 / 4
	ph := h * 3 / 4
	if pw < 30 {
		pw = 30
	}
	if ph < 10 {
		ph = 10
	}
	title := m.styles.PanelTitle.Render("Select a config file")
	return m.panelStyle(true, pw, ph).Render(
		title + "\n" + clipLines(m.filePicker.View().Content, pw-2, ph-4),
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
	window := windowLines(content, m.configScroll, h-4)
	return m.panelStyle(m.focus == PanelConfig, w, h).MaxHeight(h).Render(
		m.styles.PanelTitle.Render(title) + "\n" + window,
	)
}

// windowLines returns the slice of content starting at offset, at most n
// lines long (offset = number of leading rows to skip).
func windowLines(content string, offset, n int) string {
	if n <= 0 {
		return ""
	}
	lines := strings.Split(content, "\n")
	if offset > len(lines) {
		offset = len(lines)
	}
	if offset < 0 {
		offset = 0
	}
	end := offset + n
	if end > len(lines) {
		end = len(lines)
	}
	return strings.Join(lines[offset:end], "\n")
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
		m.styles.PanelTitle.Render(title) + "\n" + clipLines(b.String(), w-2, h-4),
	)
}

func (m *Model) renderEvents(s StateSnapshot, w, h int) string {
	title := "Recent Events"
	if m.focus == PanelEvents {
		title = "▸ " + title
	}

	var b strings.Builder
	events := s.RecentEvents
	visible := h - 4
	if visible < 0 {
		visible = 0
	}
	// eventsScroll = number of most-recent lines to hide (0 = newest).
	start := len(events) - visible - m.eventsScroll
	if start < 0 {
		start = 0
	}
	if start > len(events) {
		start = len(events)
	}
	end := start + visible
	if end > len(events) {
		end = len(events)
	}
	for i := start; i < end; i++ {
		line := events[i]
		// Highlight only ERROR-level lines; a message may legitimately
		// contain the word "ERROR" at another level (probe: events panel).
		if strings.Contains(line, "level=ERROR") {
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

// clipLines keeps at most n lines of content and truncates each to a display
// width of width columns, so a panel's content always fits inside its box
// (border 2 rows + title 1 row leave n content rows). Truncation counts ANSI
// escape sequences as zero columns — a styled row keeps its visible text up
// to the budget — and resets styles on a cut so the color never bleeds into
// the border. Long configs and listings are clipped, never allowed to push
// the grid or the hint line off screen.
func clipLines(content string, width, n int) string {
	if n <= 0 {
		return ""
	}
	lines := strings.Split(content, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	for i, l := range lines {
		lines[i] = truncateANSI(l, width)
	}
	return strings.Join(lines, "\n")
}

// truncateANSI cuts s to at most width display columns. ANSI escape
// sequences occupy no columns and are preserved; a cut inside styled text
// appends a style reset so the remainder of the line stays unstyled.
func truncateANSI(s string, width int) string {
	var b strings.Builder
	col := 0
	inEsc := false
	cut := false
	for _, r := range s {
		if inEsc {
			b.WriteRune(r)
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		if r == '\x1b' {
			inEsc = true
			b.WriteRune(r)
			continue
		}
		if col >= width {
			cut = true
			break
		}
		b.WriteRune(r)
		col++
	}
	if cut {
		b.WriteString("\x1b[0m")
	}
	return b.String()
}

// panelStyle returns a fixed-size bordered style for a panel, focused or
// not. The outer box is exactly w x h: lipgloss's Width, Height and
// MaxHeight all count the border, and MaxHeight clips content longer than
// the band so a long listing or config can never push the grid (or the
// hint line) off screen.
func (m *Model) panelStyle(focused bool, w, h int) lipgloss.Style {
	st := m.styles.PanelBorder
	if focused {
		st = m.styles.FocusedBorder
	}
	return st.Width(w).Height(h).MaxHeight(h)
}

func (m *Model) renderHintLine() string {
	if m.confirmPending {
		return m.styles.ErrorStyle.Render(
			"Pipeline is running — Press q again to quit, any other key to cancel")
	}
	hints := []string{
		"↑↓ Navigate",
		"Tab Switch panel",
		"c Toggle config view",
		"r Run",
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

// TickMsg is sent periodically to trigger a re-render.
type TickMsg struct{}

// runPipelineWithConfig loads the selected config and runs the batch (single
// or watch loop) in-process. Failures are reported to RecentEvents and
// returned as the error. The pipeline's own slog output is teed into state by
// the bridge wired in cmd/tui.go, so status updates arrive the same way.
func runPipelineWithConfig(configPath string, state *AppState, ctx context.Context) error {
	fail := func(err error) error {
		state.AddRecentEvent("ERROR: " + err.Error())
		return err
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fail(fmt.Errorf("loading config: %w", err))
	}

	p, err := pipeline.NewPipeline(cfg, false, false)
	if err != nil {
		return fail(fmt.Errorf("initializing pipeline: %w", err))
	}
	defer p.Shutdown()

	// Pipeline.Shutdown/ResetContext touch the context pair without their
	// own synchronization; serialize our uses of them (pipeline code is
	// out of scope for this change).
	var pipelineMu sync.Mutex
	cancel := p.Shutdown
	stopOnCancel := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			pipelineMu.Lock()
			cancel()
			pipelineMu.Unlock()
		case <-stopOnCancel:
		}
	}()
	defer close(stopOnCancel)

	if !cfg.Batch.Watch {
		state.SetWatchIter(1) // single batch = iteration 1
		return runOneBatch(p, state)
	}

	// Watch mode: loop until cancelled, mirroring cmd/run.go.
	interval := time.Duration(cfg.Batch.WatchInterval) * time.Minute
	iteration := 0
	for {
		iteration++
		slog.Info(fmt.Sprintf("watch: iteration %d starting", iteration))
		state.SetWatchIter(iteration)

		pipelineMu.Lock()
		p.ResetContext()
		cancel = p.Shutdown
		p.ReloadConfig(configPath)
		p.RebuildIndex()
		pipelineMu.Unlock()

		if err := runOneBatch(p, state); err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			slog.Info(fmt.Sprintf("watch: iteration %d complete, stopping", iteration))
			return nil
		case <-time.After(interval):
		}
	}
}

// runOneBatch runs one ProcessBatchStreaming pass and records its outcome.
func runOneBatch(p *pipeline.Pipeline, state *AppState) error {
	stats, err := p.ProcessBatchStreaming()
	if stats != nil {
		state.UpdateStats(*stats)
	}
	if err != nil {
		state.AddRecentEvent("ERROR: batch processing: " + err.Error())
		return fmt.Errorf("batch processing: %w", err)
	}
	state.AddRecentEvent(fmt.Sprintf(
		"batch complete: %d success, %d skipped, %d partial, %d failed",
		stats.Success, stats.Skipped, stats.Partial, stats.Failed,
	))
	return nil
}
