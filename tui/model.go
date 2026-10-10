package tui

import (
	"context"
	"fmt"
	"image"
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
	// bubbles v2.2.1 viewport: scrollable panel with keyboard + native mouse
	// wheel (spec amend2 REQ-7); textinput: the inline value editor (REQ-8).
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	// lipgloss v2.0.6: Style.Border(b Border, sides ...bool) takes a Border
	// value first (v1's Border(cond, ...) does not compile); JoinHorizontal/
	// JoinVertical lay the four panels out.
	"charm.land/lipgloss/v2"

	"github.com/kouko/youtube-summarize-scraper/config"
	"github.com/kouko/youtube-summarize-scraper/pipeline"

	// runewidth: display-width of runes (CJK = 2, ASCII = 1).
	"github.com/mattn/go-runewidth"
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

	// Width tracking for resizable panels (left:config, right:status/events)
	leftWidth           int
	rightWidth          int
	dragSashStart       int // -1 indicates not dragging, otherwise stores start X position
	dragSashStartOffset int // offset from sash start X to leftWidth at click start

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

	// program is the running tea program (set by cmd/tui.go). It is used to
	// poke a redraw when the bridge applies an event (spec amend2 REQ-6); nil
	// in tests.
	program *tea.Program

	// viewports scroll the config and events panels (bubbles v2 viewport:
	// keyboard + native mouse wheel). Spec amend2 REQ-7. They replace the
	// old manual configScroll/eventsScroll offsets.
	configVP viewport.Model
	eventsVP viewport.Model

	// configCursor is the selected line in the config panel (viewport-relative
	// row); Enter on an editable line opens the value editor. Spec amend2
	// REQ-8.
	configCursor int

	// editing: the config value editor is open (bubbles textinput overlay).
	editing  bool
	editText textinput.Model
	editPath string // dotted key path being edited

	// For item editing popup
	itemEditPopupIdx int    // -1 indicates no item edit popup, otherwise stores the item index
	itemEditContent  string // Content being edited in popup
	itemEditPath     string // dotted key path being edited in popup

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
		state:               state,
		bridge:              bridge,
		filePicker:          NewFilePickerModel(),
		focus:               PanelFilePicker,
		styles:              DefaultStyles(),
		configVP:            viewport.New(),
		eventsVP:            viewport.New(),
		editText:            textinput.New(),
		dragSashStart:       -1,
		dragSashStartOffset: 0,
		itemEditPopupIdx:    -1,
	}

	// Initialize config view with empty content
	m.configView = NewConfigView("")

	return m
}

// SetProgram wires the running tea program so the bridge can poke an
// immediate redraw when it applies an event (spec amend2 REQ-6). The poke is
// async: Program.Send blocks when the program is busy (its message queue is
// unbuffered), so it runs on its own goroutine and never stalls the event
// bridge's consumer.
func (m *Model) SetProgram(p *tea.Program) {
	m.program = p
	if m.bridge != nil && p != nil {
		m.bridge.SetNotify(func() {
			go p.Send(RefreshMsg{})
		})
	}
}

// closeBridge shuts down the event-bridge consumer, if one is wired.
func (m *Model) closeBridge() {
	if m.bridge != nil {
		m.bridge.Close()
	}
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd {
	// Initialize width tracking for 1:2 ratio based on known size, else defaults
	if m.width > 0 {
		m.leftWidth = m.width / 3
		if m.leftWidth < 10 {
			m.leftWidth = 10
		}
		m.rightWidth = m.width - m.leftWidth
	} else {
		m.leftWidth = 20
		m.rightWidth = 40
	}
	m.dragSashStart = -1
	m.itemEditPopupIdx = -1
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

// panelRects returns the screen rectangles for each panel
func (m *Model) panelRects() map[FocusedPanel]image.Rectangle {
	topHeight := 9
	total := m.height - 1
	if topHeight > total/2 {
		topHeight = total / 2
	}

	rects := make(map[FocusedPanel]image.Rectangle)
	// PanelFilePicker (top-left)
	rects[PanelFilePicker] = image.Rect(0, 0, m.leftWidth, topHeight)
	// PanelStatus (top-right)
	rects[PanelStatus] = image.Rect(m.leftWidth, 0, m.width, topHeight)
	// PanelConfig (bottom-left)
	rects[PanelConfig] = image.Rect(0, topHeight, m.leftWidth, m.height-1) // -1 for hint line
	// PanelEvents (bottom-right)
	rects[PanelEvents] = image.Rect(m.leftWidth, topHeight, m.width, m.height-1)
	return rects
}

// isOnVerticalSash checks if the given coordinates are on the vertical sash
func (m *Model) isOnVerticalSash(x, y int) bool {
	sashX := m.leftWidth
	// Sash spans the full height minus hint line
	if y < 0 || y >= m.height-1 {
		return false
	}
	// Allow a 1-column wide sash area
	return x == sashX || x == sashX-1
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

	// Allow Tab/Shift+Tab to always be handled by the main model for focus switching
	// Also allow 'c' for config toggle and 'r' for run to work from any panel
	// Forward to file picker if:
	// - it's a non-key message that the file picker owns (readDirMsg, WindowSizeMsg, etc.)
	// - OR it's a non-key mouse message AND the picker popup is open (so file picker can handle internal mouse interactions)
	// - OR it's a key message and (picker is open OR picker panel is focused and key is not excluded)
	shouldForwardToPicker := false
	if !isKey {
		// Non-key messages
		switch msg.(type) {
		case tea.MouseClickMsg, tea.MouseMotionMsg, tea.MouseReleaseMsg, tea.MouseWheelMsg:
			// Forward mouse messages only when picker popup is open
			shouldForwardToPicker = m.pickerOpen
		default:
			// readDirMsg, WindowSizeMsg, etc. that file picker owns
			shouldForwardToPicker = true
		}
	} else {
		// Key messages: forward when picker is open or picker panel focused (except excluded keys)
		shouldForwardToPicker = m.pickerOpen || (m.focus == PanelFilePicker && keyStr != "enter" && keyStr != "tab" && keyStr != "shift+tab" && keyStr != "c" && keyStr != "r")
	}
	if !shouldForwardToPicker {
		// Process the message normally in the main model
	} else {
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
		// If user hasn't dragged the sash, reset to 1:2 ratio based on new size
		if m.dragSashStart == -1 {
			idealLeft := m.width / 3
			if idealLeft < 10 {
				idealLeft = 10
			}
			idealRight := m.width - idealLeft
			if idealRight < 10 {
				// Not enough space for 1:2 with min 10 each
				if m.width < 20 {
					// Too narrow, split evenly
					idealLeft = m.width / 2
					idealRight = m.width - idealLeft
				} else {
					idealLeft = m.width - 10
					idealRight = 10
				}
			}
			m.leftWidth = idealLeft
			m.rightWidth = idealRight
		}
		m.applyPanelHeights()

	case tea.MouseWheelMsg:
		if m.pickerOpen {
			break
		}
		// The wheel scrolls the panel under the pointer (bubbles viewport:
		// native wheel support, spec amend2 REQ-7). Top panels have nothing
		// to scroll; the bottom band splits config (left) / events (right).
		// viewport.Update is value-receiver: assign its result back.
		if msg.Y >= m.topBandBottom() && m.width > 0 {
			if msg.X < m.leftWidth {
				m.configVP, _ = m.configVP.Update(msg)
			} else {
				m.eventsVP, _ = m.eventsVP.Update(msg)
			}
		}

	case tea.MouseClickMsg:
		if m.pickerOpen {
			break
		}
		m.handleMouseClick(msg.Mouse())

	case tea.MouseMotionMsg:
		if m.pickerOpen {
			break
		}
		m.handleMouseMotion(msg.Mouse())

	case tea.MouseReleaseMsg:
		if m.pickerOpen {
			break
		}
		m.handleMouseRelease(msg.Mouse())

	case tea.KeyPressMsg:
		_, cmd = m.handleKey(msg)

	case TickMsg:
		// Refresh happens on every render; nothing to do here.

	case RefreshMsg:
		// The bridge applied an event; Update returning triggers a re-render
		// on the next renderer frame (spec amend2 REQ-6).

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

	// While the value editor is open it owns everything: typing updates the
	// textinput, Enter commits (write-back), Esc cancels.
	if m.editing {
		switch key {
		case "enter":
			m.commitEdit()
		case "esc":
			m.editing = false
		default:
			m.editText, _ = m.editText.Update(msg)
		}
		return m, nil
	}

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
		switch m.focus {
		case PanelFilePicker:
			// Enter on the focused config card opens the file-picker popup.
			m.pickerOpen = true
		case PanelConfig:
			if m.toggleCollapsedItem() {
				return m, nil
			}
			m.startEdit()
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

// handleMouseClick handles mouse click events.
func (m *Model) handleMouseClick(mouse tea.Mouse) {
	// Handle clicking the vertical sash to start dragging
	if m.isOnVerticalSash(mouse.X, mouse.Y) {
		m.dragSashStart = mouse.X
		m.dragSashStartOffset = mouse.X - m.leftWidth
		return
	}

	// Handle click on panels
	m.handleClickPanel(mouse)
}

// handleMouseMotion handles mouse drag events.
func (m *Model) handleMouseMotion(mouse tea.Mouse) {
	// Handle dragging in progress
	if m.dragSashStart != -1 {
		// leftWidth = mouse.X - offset
		newLeft := mouse.X - m.dragSashStartOffset
		// Enforce minimum width constraints
		if newLeft < 10 {
			newLeft = 10
		}
		if newLeft > m.width-10 {
			newLeft = m.width - 10
		}
		m.leftWidth = newLeft
		m.rightWidth = m.width - m.leftWidth
		m.applyPanelHeights()
		return
	}
}

// handleMouseRelease handles mouse release events.
func (m *Model) handleMouseRelease(mouse tea.Mouse) {
	// Handle drag end
	m.dragSashStart = -1
}

// handleClickPanel handles left-click interactions on panels
func (m *Model) handleClickPanel(mouse tea.Mouse) {
	rects := m.panelRects()
	hitPanel := FocusedPanel(-1)
	for p, r := range rects {
		if image.Pt(mouse.X, mouse.Y).In(r) {
			hitPanel = p
			break
		}
	}
	if hitPanel == -1 || hitPanel > PanelCount {
		// Clicked empty space - close any open popups/editors
		m.editing = false
		m.itemEditPopupIdx = -1
		return
	}

	m.focus = hitPanel

	switch hitPanel {
	case PanelFilePicker:
		// Click on config file card - open file picker popup
		if !m.pickerOpen {
			m.pickerOpen = true
		}
	case PanelConfig:
		if m.itemEditPopupIdx >= 0 {
			// Click in config panel while item edit popup is open - close it
			m.itemEditPopupIdx = -1
		} else if m.configView != nil && m.configView.Mode() == ConfigViewStructured {
			// Check if clicked on a channel/playlist item
			if itemIdx := m.configView.ItemAtPos(mouse.X, mouse.Y); itemIdx >= 0 {
				m.showItemEditPopup(itemIdx)
			} else {
				// Click on regular config row - start editing if editable
				if row, ok := m.configView.PosToRow(mouse.Y); ok {
					m.startEditAtRow(row)
				}
			}
		} else if m.configView != nil && m.configView.Mode() == ConfigViewRaw {
			// In raw mode, click just focuses the panel
			// Could add raw mode editing here if needed
		}
	case PanelStatus:
		// Could add start/pause button handling here
		break
	case PanelEvents:
		// Events panel - just focus
		break
	}
}

// showItemEditPopup shows the full key-value edit popup for a channel/playlist item
func (m *Model) showItemEditPopup(itemIdx int) {
	if m.configView == nil {
		return
	}

	path, err := m.configView.showItemEditPopupInfo(itemIdx)
	if err != nil {
		m.state.AddRecentEvent("ERROR: " + err.Error())
		return
	}

	// Get the full serialized YAML for this item
	serialized, err := m.configView.Serialized()
	if err != nil {
		m.state.AddRecentEvent("ERROR: " + err.Error())
		return
	}

	m.itemEditPopupIdx = itemIdx
	m.itemEditContent = serialized
	m.itemEditPath = path
}

// startEditAtRow starts editing at the specified content row (for mouse clicks)
func (m *Model) startEditAtRow(row int) {
	if m.configView == nil {
		return
	}
	path, value, ok := m.configView.EditTargetForLine(row)
	if !ok {
		return
	}
	m.editing = true
	m.editPath = path
	m.editText.SetValue(value)
	m.editText.CursorEnd()
	m.editText.Focus()
}

// pageFocused pages the focused bottom panel by delta rows.
func (m *Model) pageFocused(delta int) (tea.Model, tea.Cmd) {
	switch m.focus {
	case PanelConfig:
		if delta > 0 {
			m.configVP.ScrollDown(delta)
		} else {
			m.configVP.ScrollUp(-delta)
		}
	case PanelEvents:
		if delta > 0 {
			m.eventsVP.ScrollDown(delta)
		} else {
			m.eventsVP.ScrollUp(-delta)
		}
	}
	return m, nil
}

// topBandBottom returns the first row of the bottom band (the row below the
// content-sized top band), used to route mouse-wheel events to the panel under
// the pointer.
func (m *Model) topBandBottom() int {
	if m.height == 0 {
		return 0
	}
	total := m.height - 1
	topHeight := 9
	if topHeight > total/2 {
		topHeight = total / 2
	}
	return topHeight
}

// applyPanelHeights sizes the file picker to the popup overlay's inner rows
// and the viewports to their panels.
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

	// Bottom panels: band height − 2 borders − 1 title row.
	total := m.height - 1
	topHeight := 9
	if topHeight > total/2 {
		topHeight = total / 2
	}
	bottomHeight := total - topHeight
	inner := bottomHeight - 3
	m.configVP.SetHeight(inner)
	m.eventsVP.SetHeight(inner)
	m.configVP.SetWidth(m.leftWidth - 2)
	m.eventsVP.SetWidth(m.rightWidth - 2)
}

func (m *Model) handleUp(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch m.focus {
	case PanelConfig:
		m.configCursorUp()
	case PanelEvents:
		// ↑ reveals older events (the viewport is newest-at-bottom).
		m.eventsVP.ScrollUp(1)
	}
	// File-picker navigation is forwarded by Update; bottom panels scroll
	// above. Returning m keeps the main model as the program model.
	return m, nil
}

func (m *Model) handleDown(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch m.focus {
	case PanelConfig:
		m.configCursorDown()
	case PanelEvents:
		m.eventsVP.ScrollDown(1)
	}
	return m, nil
}

// startEdit opens the value editor for the line under the config cursor
// (spec amend2 REQ-8). Non-editable lines are a no-op.
func (m *Model) startEdit() {
	if m.configView == nil {
		return
	}
	// The cursor is a viewport-relative row; translate to a content line.
	tableLine := m.configVP.YOffset() + m.configCursor
	contentLine := m.configView.TableContentIndex(tableLine)
	path, value, ok := m.configView.EditTargetForLine(contentLine)
	if !ok {
		return
	}
	m.editing = true
	m.editPath = path
	m.editText.SetValue(value)
	m.editText.CursorEnd()
	m.editText.Focus()
}

// commitEdit writes the edited value back to the YAML file on disk and
// reloads the tree (spec amend2 REQ-8). Failures surface in Recent Events.
func (m *Model) commitEdit() {
	m.editing = false
	m.editText.Blur()
	if m.configView == nil || m.editPath == "" {
		return
	}
	if err := m.configView.SetValue(m.editPath, m.editText.Value()); err != nil {
		m.state.AddRecentEvent("ERROR: edit failed: " + err.Error())
		return
	}
	out, err := m.configView.Serialized()
	if err != nil {
		m.state.AddRecentEvent("ERROR: edit serialize failed: " + err.Error())
		return
	}
	path := m.state.Snapshot().ConfigPath
	if path == "" {
		return
	}
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		m.state.AddRecentEvent("ERROR: writing config: " + err.Error())
		return
	}
	m.state.UpdateConfig(path, out)
}

// toggleCollapsedItem expands or collapses the list item under the config
// cursor (user feedback option B). It reports whether the row was a collapsed
// item row; other rows fall through to value editing.
func (m *Model) toggleCollapsedItem() bool {
	if m.configView == nil {
		return false
	}
	tableLine := m.configVP.YOffset() + m.configCursor
	contentLine := m.configView.TableContentIndex(tableLine)
	if contentLine < 0 || contentLine >= len(m.configView.lineIsSummary) {
		return false
	}
	if m.configView.lineIsSummary[contentLine] {
		m.configView.ExpandItem(contentLine)
		return true
	}
	// An expanded item's child row collapses back with Esc; Enter on a value
	// row edits it. But Enter on an expanded item's own heading row collapses.
	if m.configView.isExpandedItemHeading(contentLine) {
		m.configView.CollapseItem(contentLine)
		return true
	}
	return false
}

// cursorOnContent reports whether the cursor's table row is a content row
// (not the header or a separator).
func (m *Model) cursorOnContent() bool {
	tableLine := m.configVP.YOffset() + m.configCursor
	return m.configView != nil && m.configView.TableContentIndex(tableLine) >= 0
}

// configCursorUp moves the selection cursor up one content row, skipping
// table separator and header lines; at the top it scrolls the viewport.
func (m *Model) configCursorUp() {
	for {
		if m.configCursor > 0 {
			m.configCursor--
		} else {
			m.configVP.ScrollUp(1)
			return
		}
		if m.cursorOnContent() {
			return
		}
	}
}

// configCursorDown moves the selection cursor down one content row, skipping
// table separator and header lines; at the bottom it scrolls the viewport.
func (m *Model) configCursorDown() {
	for {
		if m.configCursor < m.configVP.Height()-1 {
			m.configCursor++
		} else {
			m.configVP.ScrollDown(1)
			return
		}
		if m.cursorOnContent() {
			return
		}
	}
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
	// Initialize width tracking if not set (e.g., first render before Init completes)
	if m.leftWidth == 0 {
		m.leftWidth = m.width / 3
		if m.leftWidth < 10 {
			m.leftWidth = 10
		}
		m.rightWidth = m.width - m.leftWidth
	}
	leftWidth := m.leftWidth
	rightWidth := m.rightWidth
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

	// While the picker popup is open, overlay it centered on the frame,
	// keeping the rendered UI visible behind it (user feedback: an empty
	// background loses context — other apps keep the app behind modals).
	if m.pickerOpen {
		content = overlayCentered(content, m.renderPickerPopup(m.width, m.height), m.width, m.height)
	}
	v := tea.NewView(content)
	v.AltScreen = true
	// Mouse wheel reporting (spec amend2 REQ-7): cell-motion covers clicks
	// and wheel; the viewports consume wheel events directly.
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// renderConfigCard renders the top-left short card: the current config path,
// its size, and the "open picker" hint. It never renders the file listing —
// the picker lives in the popup overlay instead.
func (m *Model) renderConfigCard(w, h int) string {
	title := "Config File"
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
	return RenderPanelWithTitle(title, b.String(), w, h, m.focus == PanelFilePicker)
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
	return PopupStyle().Width(pw).Height(ph).Render(
		lipgloss.JoinVertical(lipgloss.Top,
			PopupTitleStyle().Width(pw-2).Render("Select a config file"),
			clipLines(m.filePicker.View().Content, pw-2, ph-4),
		),
	)
}

func (m *Model) renderConfig(w, h int) string {
	title := "Config"
	var content string
	if m.configView == nil {
		content = "(no config selected)"
	} else {
		switch m.configView.Mode() {
		case ConfigViewStructured:
			// Table view: bordered two-column table with separator lines before
			// sections and items. The cursor is a table row; the marker only
			// lands on content rows (separators are skipped). REQ-8 editing
			// keeps its content-row index via the mapping helpers.
			// Calculate natural table width (capped to reasonable max) to allow
			// horizontal scrolling when content is wider than panel
			natWidth := m.configView.NaturalTableWidth()
			if natWidth < 40 { // minimum reasonable width
				natWidth = 40
			}
			if natWidth > 120 { // maximum reasonable width
				natWidth = 120
			}
			tbl := m.configView.RenderTable(natWidth)
			if m.focus == PanelConfig {
				tableRow := m.configVP.YOffset() + m.configCursor
				if tableRow >= 0 && tableRow < len(tbl) && !m.configView.IsTableSeparator(tableRow) &&
					m.configView.TableContentIndex(tableRow) >= 0 {
					// Mark the whole row with the focus style: replaces no
					// characters, so the row's display width never changes and
					// it cannot wrap (a '▸' or '>' prefix that swapped the left
					// border shifted widths in some terminals).
					tbl[tableRow] = m.styles.FocusedTitle.Render(tbl[tableRow])
				}
			}
			content = strings.Join(tbl, "\n")
		case ConfigViewRaw:
			content = m.configView.Render()
		}
	}
	// The viewport owns scrolling (keyboard + wheel); size it to the panel's
	// inner rows on every render (cheap; offset is preserved).
	m.configVP.SetWidth(w - 2)
	m.configVP.SetHeight(h - 3)
	// Set content to table rendered at natural width (allows horizontal scrolling)
	m.configVP.SetContent(content)
	body := m.configVP.View()
	// The value editor overlays the panel (spec amend2 REQ-8).
	if m.editing {
		edit := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("33")).
			Padding(0, 1).
			Render(lipgloss.JoinVertical(lipgloss.Top,
				TitleStyle(true).Width(w-4).Render("Edit "+m.editPath),
				"\n\n"+
					m.editText.View()+
					"\n\nEnter 存檔   Esc 取消",
			))
		body = lipgloss.Place(w-2, h-3, lipgloss.Center, lipgloss.Center, edit)
	}
	return RenderPanelWithTitle(title, body, w, h, m.focus == PanelConfig)
}

func (m *Model) renderStatus(s StateSnapshot, w, h int) string {
	title := "Execution Status"

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

	return RenderPanelWithTitle(title, clipLines(b.String(), w-2, h-4), w, h, m.focus == PanelStatus)
}

func (m *Model) renderEvents(s StateSnapshot, w, h int) string {
	title := "Recent Events"

	var b strings.Builder
	for _, line := range s.RecentEvents {
		// Highlight only ERROR-level lines; a message may legitimately
		// contain the word "ERROR" at another level (probe: events panel).
		if strings.Contains(line, "level=ERROR") {
			b.WriteString(m.styles.ErrorHighlight.Render("[ERROR] "+line) + "\n")
		} else {
			b.WriteString(line + "\n")
		}
	}
	if len(s.RecentEvents) == 0 {
		b.WriteString("(no events)")
	}
	// The viewport owns scrolling (keyboard + wheel). Newest events arrive at
	// the bottom: keep the view pinned there unless the user scrolled up.
	atBottom := m.eventsVP.AtBottom()
	m.eventsVP.SetWidth(w - 2)
	m.eventsVP.SetHeight(h - 3)
	m.eventsVP.SetContent(b.String())
	if atBottom {
		m.eventsVP.GotoBottom()
	}

	return RenderPanelWithTitle(title, clipLines(m.eventsVP.View(), w-2, h-4), w, h, m.focus == PanelEvents)
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
		runeWidth := runewidth.RuneWidth(r)
		if col+runeWidth > width {
			cut = true
			break
		}
		b.WriteRune(r)
		col += runeWidth
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
	return PanelBorderStyle(focused).Width(w).Height(h).MaxHeight(h)
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
	return HintStyle().Render(strings.Join(hints, "  "))
}

// QuitConfirmMsg is sent when user confirms quitting while pipeline is running.
type QuitConfirmMsg struct{}

// StartRunMsg signals to start the pipeline.
type StartRunMsg struct{}

// StopRunMsg signals to stop the pipeline.
type StopRunMsg struct{}

// TickMsg is sent periodically to trigger a re-render.
type TickMsg struct{}

// RefreshMsg pokes an immediate re-render after the bridge applied an event
// (spec amend2 REQ-6); the periodic tick stays as the fallback.
type RefreshMsg struct{}

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

// ansiStrip removes ANSI escape sequences from s.
func ansiStrip(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		if r == '\x1b' {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// overlayCentered composites overlay onto base: the overlay's non-blank cells
// replace the base's cells at the same position; everything else keeps the
// base. The base UI stays visible behind the popup (user feedback: an empty
// background loses context).
func overlayCentered(base, overlay string, width, height int) string {
	baseLines := strings.Split(base, "\n")
	ovLines := strings.Split(overlay, "\n")
	// Trim blank padding lines of the overlay (lipgloss.Place centering).
	for len(ovLines) > 0 && strings.TrimSpace(ansiStrip(ovLines[len(ovLines)-1])) == "" {
		ovLines = ovLines[:len(ovLines)-1]
	}
	for len(ovLines) > 0 && strings.TrimSpace(ansiStrip(ovLines[0])) == "" {
		ovLines = ovLines[1:]
	}
	ovH := len(ovLines)
	ovW := 0
	for _, l := range ovLines {
		if w := lipgloss.Width(l); w > ovW {
			ovW = w
		}
	}
	top := (height - ovH) / 2
	if top < 0 {
		top = 0
	}
	left := (width - ovW) / 2
	if left < 0 {
		left = 0
	}
	for i, ov := range ovLines {
		row := top + i
		if row >= len(baseLines) {
			break
		}
		baseLines[row] = spliceLine(baseLines[row], ov, left, width)
	}
	return strings.Join(baseLines, "\n")
}

// spliceLine replaces the display columns [left, left+ovWidth) of a base row
// with the overlay row, keeping the base row's visible content before and
// after (ANSI codes before the cut are preserved; the tail is re-rendered
// plain, which is fine for box borders).
func spliceLine(line, ov string, left, width int) string {
	before := truncateANSI(line, left)
	after := ansiTail(line, left, width)
	pad := ""
	if bw := lipgloss.Width(before); bw < left {
		pad = strings.Repeat(" ", left-bw)
	}
	return before + pad + ov + after
}

// ansiTail returns the visible suffix of line starting at display column col,
// truncating to width total columns. ANSI codes before the cut are dropped;
// a reset closes any styling so the tail renders plain.
func ansiTail(line string, col, width int) string {
	var b strings.Builder
	display := 0
	inEsc := false
	cut := false
	for _, r := range line {
		if inEsc {
			if cut {
				b.WriteRune(r)
			}
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		if r == '\x1b' {
			inEsc = true
			continue // drop codes before the cut
		}
		runeWidth := runewidth.RuneWidth(r)
		if !cut && display >= col {
			cut = true
			b.WriteString("\x1b[0m")
		}
		if cut {
			if display >= width {
				break
			}
			b.WriteRune(r)
		}
		display += runeWidth
	}
	return b.String()
}
