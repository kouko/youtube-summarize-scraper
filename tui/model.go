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

// PanelCount is the number of panels (the four enum values survive; only the
// active focus set shrinks from 4 to 2).
const PanelCount = 4

// focusTargets is the active focus set (spec REQ-2): the two upper-row
// panels. PanelConfig and PanelEvents survive as the content kinds the
// lower full-width panel can display, but they are no longer focus targets.
func focusTargets() []FocusedPanel {
	return []FocusedPanel{PanelFilePicker, PanelStatus}
}

// nextFocusTarget returns the target dir steps from current within targets,
// wrapping at the ends. A current outside the set snaps into it (forward:
// the first target, backward: the last). A single-target set cycles to
// itself.
func nextFocusTarget(targets []FocusedPanel, current FocusedPanel, dir int) FocusedPanel {
	for i, p := range targets {
		if p == current {
			n := len(targets)
			return targets[((i+dir)%n+n)%n]
		}
	}
	if dir < 0 {
		return targets[len(targets)-1]
	}
	return targets[0]
}

// cycleFocus moves focus dir steps within the active focus set.
func (m *Model) cycleFocus(dir int) {
	m.focus = nextFocusTarget(focusTargets(), m.focus, dir)
}

// layoutGeom is the single source of layout geometry (spec REQ-3): the
// renderer and every mouse hit-test helper read these rectangles. Any new
// panel or resize rule is added here — never recomputed at the call site.
type layoutGeom struct {
	configRect image.Rectangle // upper-left config card
	statusRect image.Rectangle // upper-right execution status
	lowerRect  image.Rectangle // full-width lower content area
}

// geometry computes the panel rectangles for the current terminal size:
// an upper row with the config card and the status panel side by side and
// a lower row with one full-width content area, ending above the hint line.
func (m *Model) geometry() layoutGeom {
	total := m.height - 1 // the hint line takes the last row
	topHeight := 9
	if topHeight > total/2 {
		topHeight = total / 2
	}
	return layoutGeom{
		configRect: image.Rect(0, 0, m.leftWidth, topHeight),
		statusRect: image.Rect(m.leftWidth, 0, m.width, topHeight),
		lowerRect:  image.Rect(0, topHeight, m.width, m.height-1),
	}
}

// lowerContentKind returns the kind of content the lower full-width panel
// displays: the execution log while the status panel has focus, the config
// content otherwise (spec REQ-2).
func (m *Model) lowerContentKind() FocusedPanel {
	if m.focus == PanelStatus {
		return PanelEvents
	}
	return PanelConfig
}

// Popup hint texts (kept as constants so the renderer and the click hit
// tests always agree on the exact segment layout).
const (
	pickerPopupHint = "[Enter] Select  [Esc] Cancel"
	editPopupHint   = "[Enter] Save  [Esc] Cancel"

	// pickerNavClickGuard is how long after a mouse click navigates into a
	// directory a second click on the exact same spot is ignored. A
	// double-click's second press lands on the reloaded listing and would
	// select the new directory's first file, closing the popup. 400ms covers
	// typical double-click intervals (macOS allows up to ~500ms).
	pickerNavClickGuard = 400 * time.Millisecond
)

// pickerNavClick records a mouse click that navigated into a directory, so a
// fast second click on the same coordinates (a double-click) can be ignored.
type pickerNavClick struct {
	at   time.Time
	x, y int
}

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

	// runGen/runMu serialize the end-of-run app-state write against a newer
	// run's start: the goroutine from an ended run clears SetRunning(false)
	// only while its generation is still current, so a late write during a
	// graceful stop→restart unwind can never clobber the newer run's
	// SetRunning(true). Generation 0 means no run has started yet.
	runMu  sync.Mutex
	runGen uint64

	// confirmPending: a first q while running armed the quit confirmation.
	// The next q confirms (QuitConfirmMsg); any other key disarms it.
	confirmPending bool

	// pickerOpen: the file-picker popup overlay is showing (opened by Enter
	// on the focused config card). While open it owns all keys.
	pickerOpen bool

	// lastPickerNavClick guards against the second click of a double-click on
	// a directory row: the first click navigates into the directory and the
	// listing reloads, so the same coordinates now show a different entry
	// (often the new directory's first file) and the second click would
	// select it and close the popup (regression 2026-10-11). Only clicks that
	// actually navigated into a directory are recorded.
	lastPickerNavClick pickerNavClick

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

// panelRects returns the screen rectangles for each panel, derived from the
// single geometry source. PanelConfig and PanelEvents share the lower
// content rectangle: they are the kinds of content it displays.
func (m *Model) panelRects() map[FocusedPanel]image.Rectangle {
	g := m.geometry()
	return map[FocusedPanel]image.Rectangle{
		PanelFilePicker: g.configRect,
		PanelStatus:     g.statusRect,
		PanelConfig:     g.lowerRect,
		PanelEvents:     g.lowerRect,
	}
}

// isOnVerticalSash checks if the given coordinates are on the vertical sash
// between the two upper-row panels (the sash exists only in the upper row;
// the lower content area is full width).
func (m *Model) isOnVerticalSash(x, y int) bool {
	g := m.geometry()
	if y < 0 || y >= g.configRect.Max.Y {
		return false
	}
	return x == m.leftWidth || x == m.leftWidth-1
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
			// Do NOT forward mouse messages to file picker when picker popup is open;
			// we will handle them in handleClickPanel to support mouse clicks on hint segments and file list.
			shouldForwardToPicker = false
		default:
			// readDirMsg, WindowSizeMsg, etc. that file picker owns
			shouldForwardToPicker = true
		}
	} else {
		// Key messages: the picker only receives keys while its popup is
		// open (it owns navigation). With the popup closed the picker card
		// keeps Enter (opens the popup), Tab and the run/quit keys; arrows
		// drive the lower content area.
		shouldForwardToPicker = m.pickerOpen
	}
	// Enter may select a file. The picker never clears Path on directory
	// navigation, so a stale path from an earlier selection would make the
	// post-Enter checks (handleKey here, handlePickerPopupClick for the mouse)
	// believe a file was chosen even when this Enter just opened a directory.
	// Clearing right before the picker handles Enter makes a non-empty Path
	// afterwards mean "this Enter selected a file".
	if isKey && m.pickerOpen && keyStr == "enter" {
		m.filePicker.ClearChosenPath()
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
		// The wheel scrolls the lower content area under the pointer
		// (bubbles viewport: native wheel support, spec amend2 REQ-7).
		// Upper panels have nothing to scroll.
		g := m.geometry()
		if msg.Y >= g.lowerRect.Min.Y && m.width > 0 {
			if m.lowerContentKind() == PanelConfig {
				m.configVP, _ = m.configVP.Update(msg)
			} else {
				m.eventsVP, _ = m.eventsVP.Update(msg)
			}
		}

	case tea.MouseClickMsg:
		mouse := msg.Mouse()
		// Popup overlays own clicks: hint segments act like their keys and
		// the picker's file list is clickable. Clicks never fall through to
		// the panels underneath an open overlay.
		if m.pickerOpen {
			cmd = m.handlePickerPopupClick(mouse)
		} else if m.editing {
			if key, ok := m.popupHintHitTest(mouse, m.editPopupRect(), editPopupHint); ok {
				cmd = m.dispatchHintKey(key)
			} else {
				// Not on hint; let the editor handle it
				m.editText, _ = m.editText.Update(msg)
			}
		} else {
			cmd = m.handleMouseClick(mouse)
		}

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
		m.lastPickerNavClick = pickerNavClick{}
		m.handleConfigSelected(msg.Path)

	case StartRunMsg:
		// Snapshot the shared config path on the UI thread: the run goroutine
		// must not read AppState's fields without its lock.
		snap := m.state.Snapshot()
		if !m.runAlive() && snap.ConfigPath != "" {
			m.isRunning = true
			m.err = nil
			// Start pipeline in background. The context and its cancel are
			// per-run: the goroutine closes over the local cancel, never the
			// shared m.cancel field, so a stale goroutine from an earlier run
			// cannot cancel a newer run's context.
			ctx, cancel := context.WithCancel(context.Background())
			m.ctx, m.cancel = ctx, cancel
			// The generation token, paired with the start's SetRunning(true)
			// under the same mutex, stops an unwinding old run from writing
			// SetRunning(false) after a newer run set it true (stop→restart).
			m.runMu.Lock()
			m.runGen++
			gen := m.runGen
			m.state.SetRunning(true)
			m.runMu.Unlock()
			cfgPath := snap.ConfigPath
			go func() {
				defer cancel()
				// Run pipeline with selected config (path snapshotted above).
				if err := runPipelineWithConfig(cfgPath, m.state, ctx); err != nil {
					m.state.AddRecentEvent(fmt.Sprintf("ERROR: %v", err))
				}
				// Mark this run dead before the app state reports it stopped,
				// so the liveness guards never disagree with the status view.
				cancel()
				m.finishRun(gen)
			}()
			// Notify the UI thread when the run ends by itself. The ctx is the
			// run token: Update clears the flags only when it matches the
			// current run, so a stale completion cannot clear a newer run.
			cmd = func() tea.Msg {
				<-ctx.Done()
				return RunFinishedMsg{ctx: ctx}
			}
		}

	case RunFinishedMsg:
		// A run ended by itself (batch completed, watch loop cancelled, or
		// config failed to load). Clear both flags only for the current run;
		// the write happens on the UI thread for the run it belongs to.
		if msg.ctx == m.ctx {
			m.isRunning = false
			m.state.SetRunning(false)
		}

	case StopRunMsg:
		// Only the current run's stop token may cancel: a stop dispatched for
		// the previous run and delivered after a restart is a no-op.
		if msg.ctx == m.ctx {
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

// runAlive reports whether the model still considers a pipeline running.
// Both inputs are UI-thread-owned — isRunning is only written in Update, and
// ctx is only assigned in Update; the run goroutine never touches either
// (no cross-goroutine race). A run that ends by itself cancels its own ctx
// before the app state reports Stopped, so a stale isRunning (its
// RunFinishedMsg has not been processed yet) is caught by the dead ctx
// instead of the guards advertising a run that is over.
func (m *Model) runAlive() bool {
	return m.isRunning && (m.ctx == nil || m.ctx.Err() == nil)
}

// finishRun writes the end-of-run app state only while gen is still the
// current run generation, so an old run unwinding after stop→restart cannot
// clobber a newer run's running state.
func (m *Model) finishRun(gen uint64) {
	m.runMu.Lock()
	if m.runGen == gen {
		m.state.SetRunning(false)
	}
	m.runMu.Unlock()
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
	// Enter selects the current file (even if same as already chosen). Other keys
	// are forwarded by Update.
	if m.pickerOpen {
		if key == "esc" {
			m.pickerOpen = false
			m.lastPickerNavClick = pickerNavClick{}
			return m, nil
		}
		if key == "enter" {
			if path := m.filePicker.ChosenPath(); path != "" {
				m.pickerOpen = false
				m.lastPickerNavClick = pickerNavClick{}
				m.handleConfigSelected(path)
			}
			return m, nil
		}
		return m, nil
	}

	// Any key other than q/ctrl+c cancels a pending quit confirmation.
	if key != "q" && key != "ctrl+c" {
		m.confirmPending = false
	}
	switch key {
	case "enter":
		if m.focus == PanelFilePicker {
			if m.configCursorActionable() {
				if m.toggleCollapsedItem() {
					return m, nil
				}
				m.startEdit()
				return m, nil
			}
			// Enter on the focused config card opens the file-picker popup.
			m.pickerOpen = true
		}
		return m, nil

	case "ctrl+c", "q":
		if m.runAlive() {
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
		if !m.runAlive() && m.state.ConfigPath != "" {
			return m, func() tea.Msg { return StartRunMsg{} }
		}
		return m, nil

	case "s":
		if m.runAlive() {
			ctx := m.ctx
			return m, func() tea.Msg { return StopRunMsg{ctx: ctx} }
		}
		return m, nil

	case "c":
		if m.configView != nil {
			m.configView.Toggle()
			return m, nil
		}
		return m, nil

	case "tab":
		m.cycleFocus(1)
		return m, nil

	case "shift+tab":
		m.cycleFocus(-1)
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

// handleMouseClick handles mouse click events. Returns any command the click
// produced (a hint-segment click can emit a run command).
func (m *Model) handleMouseClick(mouse tea.Mouse) tea.Cmd {
	// Handle clicking the vertical sash to start dragging
	if m.isOnVerticalSash(mouse.X, mouse.Y) {
		m.dragSashStart = mouse.X
		m.dragSashStartOffset = mouse.X - m.leftWidth
		return nil
	}

	// Handle click on panels
	return m.handleClickPanel(mouse)
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
// handleClickPanel maps a click position to a panel, focuses it, and runs the
// panel's click behaviour. A click on a hint segment triggers the segment's
// key action (same as pressing the key); returns any command the action
// produced. It only runs while no popup overlay or inline editor is open —
// with the picker popup open, Update routes clicks to handlePickerPopupClick,
// with the editor open, to the editor.
func (m *Model) handleClickPanel(mouse tea.Mouse) tea.Cmd {
	g := m.geometry()
	pt := image.Pt(mouse.X, mouse.Y)

	var focusTarget FocusedPanel // the upper panel the click focuses
	var kind FocusedPanel        // content kind whose hints/rows the click maps to
	var rect image.Rectangle
	switch {
	case pt.In(g.configRect):
		focusTarget, kind, rect = PanelFilePicker, PanelFilePicker, g.configRect
	case pt.In(g.statusRect):
		focusTarget, kind, rect = PanelStatus, PanelStatus, g.statusRect
	case pt.In(g.lowerRect):
		focusTarget = m.focus // click on lower content focuses its owner
		kind = m.lowerContentKind()
		rect = g.lowerRect
	default:
		m.editing = false
		return nil
	}

	m.focus = focusTarget

	// Click on a hint segment triggers its key action (like pressing the
	// key). The inline editor owns input while open, so hint clicks wait.
	if !m.editing {
		if key, ok := m.hintHitTest(mouse, kind, rect); ok {
			switch {
			case kind == PanelConfig:
				return m.dispatchConfigContentKey(key)
			case kind == PanelFilePicker && key == "enter":
				// The card's hint is labeled "Select Config": it always
				// opens the picker popup, whatever the config cursor sits on.
				if !m.pickerOpen {
					m.pickerOpen = true
				}
				return nil
			default:
				return m.dispatchHintKey(key)
			}
		}
	}

	switch kind {
	case PanelFilePicker:
		// Click on config file card - open file picker popup
		if !m.pickerOpen {
			m.pickerOpen = true
		}
	case PanelConfig:
		if m.configView != nil && m.configView.Mode() == ConfigViewStructured {
			// Click on a table row starts editing it. The table lives in a
			// viewport that starts below the title bar and hint rows and is
			// scrolled by YOffset, so the screen Y must be mapped through
			// both before asking the view which content row is there.
			if tableRow := m.configTableRowAt(mouse.Y, rect); tableRow >= 0 {
				if content := m.configView.TableContentIndex(tableRow); content >= 0 {
					m.startEditAtRow(content)
				}
			}
		} else if m.configView != nil && m.configView.Mode() == ConfigViewRaw {
			// In raw mode, click just focuses the panel
			// Could add raw mode editing here if needed
		}
	}
	return nil
}

// dispatchConfigContentKey runs the lower config content's hint actions. The
// content's [Enter] Edit edits or toggles the cursor row; it never opens the
// picker popup (that is the card's own Enter action).
func (m *Model) dispatchConfigContentKey(key string) tea.Cmd {
	if key == "enter" {
		if m.toggleCollapsedItem() {
			return nil
		}
		m.startEdit()
		return nil
	}
	return m.dispatchHintKey(key)
}

// handlePickerPopupClick routes a click while the file picker popup is open.
// Hint segments trigger their key action; a click on a file row moves the
// picker cursor there and applies Enter (a directory opens, a file is
// selected) — the same effect as the keyboard path. Clicks on the title or
// separator and clicks outside the overlay do nothing. Returns any command
// the click produced.
func (m *Model) handlePickerPopupClick(mouse tea.Mouse) tea.Cmd {
	rect := m.pickerPopupRect()
	if key, ok := m.popupHintHitTest(mouse, rect, pickerPopupHint); ok {
		if key == "enter" {
			// Mirror the keyboard Enter path (handleKey): let the picker
			// apply Enter to its highlighted row — a file sets Path, a
			// directory reloads — then apply the selection here, exactly
			// like the file-list click path below. PressEnter clears the
			// stale selection first, so Path after the call means this Enter
			// selected a file (never a previously chosen one).
			pickerCmd := m.filePicker.PressEnter()
			if path := m.filePicker.ChosenPath(); path != "" {
				m.pickerOpen = false
				m.lastPickerNavClick = pickerNavClick{}
				m.handleConfigSelected(path)
				return nil
			}
			return pickerCmd
		}
		return m.dispatchHintKey(key)
	}
	if !popupHitTest(mouse, rect) {
		return nil
	}
	// Ignore the second click of a double-click on a directory row. The first
	// click navigated in and the listing reloaded, so these coordinates now
	// show a different entry (often the new directory's first file) and the
	// second click would select it and close the popup.
	if g := m.lastPickerNavClick; !g.at.IsZero() &&
		g.x == mouse.X && g.y == mouse.Y &&
		time.Since(g.at) < pickerNavClickGuard {
		return nil
	}
	// Content area: title row, hint rows (same layout the renderer uses),
	// then a separator row; the file list starts below them.
	hintLines := popupHintLineCount(pickerPopupHint, rect.Dx(), rect.Dy())
	contentStartY := rect.Min.Y + 1 + hintLines + 1
	handled, pickerCmd := m.filePicker.HandleMouseClick(mouse.Y - contentStartY)
	if !handled {
		return nil
	}
	if path := m.filePicker.ChosenPath(); path != "" {
		// Enter landed on a file: close the popup and load it, exactly like
		// the keyboard path in handleKey.
		m.pickerOpen = false
		m.lastPickerNavClick = pickerNavClick{}
		m.handleConfigSelected(path)
		return nil
	}
	// A directory was opened: the picker returned its reload command. Record
	// the click so the second press of a double-click on the same spot is
	// ignored until the guard window expires.
	m.lastPickerNavClick = pickerNavClick{at: time.Now(), x: mouse.X, y: mouse.Y}
	return pickerCmd
}

// hintHitTest maps a click inside a panel to a hint segment. Hint rows sit
// directly below the title bar (panel row 0): line i is at Min.Y+1+i, and
// the text area starts at Min.X+1 (right after the left border). Returns the
// segment's Key when the click lands on an actionable segment.
func (m *Model) hintHitTest(mouse tea.Mouse, p FocusedPanel, rect image.Rectangle) (string, bool) {
	segs := m.hintSegmentsFor(p)
	if len(segs) == 0 {
		return "", false
	}
	textAreaW := rect.Dx() - 4
	if textAreaW < 1 {
		return "", false
	}
	lines := layoutHintSegments(segs, textAreaW, rect.Dy()-5)
	row := mouse.Y - (rect.Min.Y + 1)
	if row < 0 || row >= len(lines) {
		return "", false
	}
	// Hint text starts immediately after the left border (no padding), so
	// column 0 of the text area is the panel's second column.
	col := mouse.X - (rect.Min.X + 1)
	for _, pw := range lines[row] {
		if segs[pw.seg].Key != "" && col >= pw.startCol && col < pw.endCol {
			return segs[pw.seg].Key, true
		}
	}
	return "", false
}

// popupRect computes the screen rectangle of the file picker popup overlay.
func (m *Model) pickerPopupRect() image.Rectangle {
	if !m.pickerOpen {
		return image.Rect(0, 0, 0, 0)
	}
	pw := m.width * 3 / 4
	if pw < 30 {
		pw = 30
	}
	ph := m.height * 3 / 4
	if ph < 10 {
		ph = 10
	}
	// The actual popup dimensions may be smaller due to centering trimming.
	// Use the exact same logic as overlayCentered to compute the rendered size.
	overlayStr := m.renderPickerPopup(m.width, m.height)
	ovLines := strings.Split(overlayStr, "\n")
	// Trim blank padding lines (same as overlayCentered)
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
	top := (m.height - ovH) / 2
	if top < 0 {
		top = 0
	}
	left := (m.width - ovW) / 2
	if left < 0 {
		left = 0
	}
	return image.Rect(left, top, left+ovW, top+ovH)
}

// editPopupSize returns the outer size of the inline value editor for a
// panel of the given content width (shared by renderConfig and editPopupRect).
func editPopupSize(w int) (ew int, eh int) {
	ew = w * 2 / 3
	if ew < 30 {
		ew = 30
	}
	if ew > w-2 {
		ew = w - 2
	}
	return ew, 9
}

// editPopupRect computes the screen rectangle of the inline edit popup,
// mirroring the exact placement arithmetic in renderConfig: the popup is
// centered inside a (w-2) x (h-3) box whose origin is the panel's content
// area (below the title bar and hint rows).
func (m *Model) editPopupRect() image.Rectangle {
	if !m.editing || m.configView == nil {
		return image.Rect(0, 0, 0, 0)
	}
	g := m.geometry()
	configRect := g.lowerRect
	w, h := configRect.Dx(), configRect.Dy()
	ew, eh := editPopupSize(w)
	// The Place box (w-2 x h-3) starts at the panel's content origin: the
	// title bar is row 0 and the hint rows (hint lines + separator) follow,
	// exactly the rows RenderPanelWithSegments skips before drawing the body.
	hintRows := m.panelHintRows(PanelConfig, configRect)
	left := configRect.Min.X + 1 + (w-2-ew)/2
	top := configRect.Min.Y + 1 + hintRows + (h-3-eh)/2
	return image.Rect(left, top, left+ew, top+eh)
}

// panelHintRows returns how many rows the panel's hint area occupies below
// the title bar: the wrapped hint lines plus the separator row, using the
// same layout arithmetic as RenderPanelWithSegments (0 when the panel has no
// hint segments).
func (m *Model) panelHintRows(p FocusedPanel, rect image.Rectangle) int {
	segs := m.hintSegmentsFor(p)
	if len(segs) == 0 {
		return 0
	}
	innerW, innerH := rect.Dx()-2, rect.Dy()-2
	if innerW <= 0 || innerH <= 0 {
		return 0
	}
	return len(layoutHintSegments(segs, innerW-2, innerH-3)) + 1
}

// popupHitTest checks if a mouse click is within the given popup rectangle.
func popupHitTest(mouse tea.Mouse, rect image.Rectangle) bool {
	return image.Pt(mouse.X, mouse.Y).In(rect)
}

// popupHintHitTest checks if a mouse click hits a hint segment in the given popup.
func (m *Model) popupHintHitTest(mouse tea.Mouse, rect image.Rectangle, hint string) (string, bool) {
	if hint == "" {
		return "", false
	}
	// Parse the hint string into segments. Expected format:
	// "[Enter] Select  [Esc] Cancel" -> two segments with keys "enter" and "esc"
	segs := parsePopupHint(hint)
	if len(segs) == 0 {
		return "", false
	}
	innerW := rect.Dx() - 2
	if innerW < 1 {
		return "", false
	}
	// The hint rows are laid out with the same logic as panels
	hintWords := layoutHintSegments(segs, innerW-2, rect.Dy()-3)
	row := mouse.Y - (rect.Min.Y + 1) // hint rows start at Min.Y+1 (below title bar)
	if row < 0 || row >= len(hintWords) {
		return "", false
	}
	// Text starts at Min.X+1 (after left border)
	col := mouse.X - (rect.Min.X + 1)
	for _, pw := range hintWords[row] {
		if segs[pw.seg].Key != "" && col >= pw.startCol && col < pw.endCol {
			return segs[pw.seg].Key, true
		}
	}
	return "", false
}

// parsePopupHint parses a popup hint string like "[Enter] Select  [Esc] Cancel"
// into separate segments with appropriate keys.
func parsePopupHint(hint string) []HintSegment {
	var segs []HintSegment
	// Split by double space (segment separator)
	parts := strings.Split(hint, "  ")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key := ""
		text := part
		// Extract key from bracketed notation like "[Enter]" or "[Esc]"
		if strings.HasPrefix(part, "[") {
			end := strings.Index(part, "]")
			if end > 1 {
				keyStr := strings.ToLower(part[1:end])
				// Map key strings to actual key names
				switch keyStr {
				case "enter":
					key = "enter"
				case "esc":
					key = "esc"
				case "save":
					key = "enter"
				case "cancel":
					key = "esc"
				}
				// Keep the full text including brackets for display
				text = part
			}
		}
		segs = append(segs, HintSegment{Text: text, Key: key})
	}
	return segs
}

// hintSegmentsFor returns the hint row segments of a panel. Key is the
// keyboard key a click on the segment triggers ("" = informational only).
func (m *Model) hintSegmentsFor(p FocusedPanel) []HintSegment {
	switch p {
	case PanelFilePicker:
		return []HintSegment{{Text: "[Enter] Select Config", Key: "enter"}}
	case PanelConfig:
		return []HintSegment{
			{Text: "[↑↓] Navigate"},
			{Text: "[Enter] Edit", Key: "enter"},
			{Text: "[c] Switch View", Key: "c"},
		}
	case PanelStatus:
		if m.runAlive() {
			return []HintSegment{
				{Text: "[s] Stop", Key: "s"},
				{Text: "[r] Run", Key: "r"},
			}
		}
		return []HintSegment{{Text: "[r] Run", Key: "r"}}
	case PanelEvents:
		return []HintSegment{{Text: "[↑↓] Scroll"}}
	}
	return nil
}

// dispatchHintKey runs the same handler as pressing the hinted key, so a
// hint click behaves exactly like the keyboard shortcut.
func (m *Model) dispatchHintKey(key string) tea.Cmd {
	var km tea.KeyPressMsg
	if key == "enter" {
		km = tea.KeyPressMsg{Code: tea.KeyEnter}
	} else {
		km = tea.KeyPressMsg{Text: key}
	}
	_, cmd := m.handleKey(km)
	return cmd
}

// configTableRowAt maps a screen Y to the config table row currently
// displayed at that position, or -1 when the click is above the table, in
// the panel chrome, or past the rendered rows. It mirrors renderConfig's
// layout: the table viewport starts below the title bar and hint rows
// (hint lines + separator) and is scrolled by the viewport's YOffset.
func (m *Model) configTableRowAt(screenY int, configRect image.Rectangle) int {
	hintRows := m.panelHintRows(PanelConfig, configRect)
	contentY := screenY - (configRect.Min.Y + 1 + hintRows)
	if contentY < 0 {
		return -1
	}
	// Rendered content rows: inner height minus the hint area.
	rows := configRect.Dy() - 2 - hintRows
	if contentY >= rows {
		return -1
	}
	return contentY + m.configVP.YOffset()
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
	switch m.lowerContentKind() {
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

// applyPanelHeights sizes the file picker to the popup overlay's inner rows
// and the viewports to the lower content area.
func (m *Model) applyPanelHeights() {
	if m.width == 0 || m.height == 0 {
		return
	}
	// The picker lives in the popup (3/4 of the terminal); its inner content
	// area is borders + title + hint + separator, i.e. popupH - 4 rows.
	popupH := m.height * 3 / 4
	if popupH < 10 {
		popupH = 10
	}
	m.filePicker.SetHeight(popupH - 4)

	// Lower content area: band height − 2 borders − 1 title row − 2 hint
	// rows (hint + separator; a wrapped hint clips the last content row).
	g := m.geometry()
	inner := g.lowerRect.Dy() - 5
	m.configVP.SetHeight(inner)
	m.eventsVP.SetHeight(inner)
	m.configVP.SetWidth(g.lowerRect.Dx() - 2)
	m.eventsVP.SetWidth(g.lowerRect.Dx() - 2)
}

func (m *Model) handleUp(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch m.lowerContentKind() {
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
	switch m.lowerContentKind() {
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

// configCursorActionable reports whether the config content's cursor sits on
// a row Enter can act on (an editable value or a collapsible item). With the
// config card and the config content sharing one focus target, this — not
// the focus — tells Enter apart between editing and opening the picker popup.
func (m *Model) configCursorActionable() bool {
	if m.configView == nil {
		return false
	}
	line := m.configVP.YOffset() + m.configCursor
	content := m.configView.TableContentIndex(line)
	if content < 0 {
		return false
	}
	if m.configView.lineIsSummary[content] || m.configView.isExpandedItemHeading(content) {
		return true
	}
	_, _, ok := m.configView.EditTargetForLine(content)
	return ok
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

// View renders the TUI. The layout consists of two rows: an upper row with
// the config card and the execution status panel side by side, and a lower
// full-width content area that shows the config content (when the config card
// has focus) or the execution log (when the status panel has focus). All
// panel rectangles are read from the single shared geometry source (spec REQ-3).
func (m *Model) View() tea.View {
	if m.width == 0 || m.height == 0 {
		return tea.NewView("Loading...")
	}

	// Snapshot the shared state so rendering never races with the
	// event-bridge goroutine that updates it.
	s := m.state.Snapshot()

	// Geometry comes from the single shared source (spec REQ-3).
	g := m.geometry()

	// Upper row: config card | execution status. Lower row: the content area
	// of the focused upper panel (config content or the execution log).
	configCard := m.renderConfigCard(g.configRect.Dx(), g.configRect.Dy())
	statusView := m.renderStatus(s, g.statusRect.Dx(), g.statusRect.Dy())
	topRow := lipgloss.JoinHorizontal(lipgloss.Top, configCard, statusView)

	var lowerRow string
	if m.lowerContentKind() == PanelConfig {
		lowerRow = m.renderConfig(g.lowerRect.Dx(), g.lowerRect.Dy())
	} else {
		lowerRow = m.renderEvents(s, g.lowerRect.Dx(), g.lowerRect.Dy())
	}

	mainView := lipgloss.JoinVertical(lipgloss.Left, topRow, lowerRow)

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
	// Hint for the hint row; keep content area for path/size only
	return RenderPanelWithSegments(title, b.String(), w, h, m.focus == PanelFilePicker, m.hintSegmentsFor(PanelFilePicker))
}

// renderPickerPopup renders the file-picker overlay frame (centered, sized to
// the terminal). It uses the same half-block title bar as the panels.
// The title shows the current directory for context.
func (m *Model) renderPickerPopup(w, h int) string {
	pw := w * 3 / 4
	ph := h * 3 / 4
	if pw < 30 {
		pw = 30
	}
	if ph < 10 {
		ph = 10
	}
	if pw > w-2 {
		pw = w - 2
	}
	if ph > h-2 {
		ph = h - 2
	}
	// innerW = pw-2; the content area is innerH - 2 (hint + separator),
	// so clip the picker content to that.
	title := fmt.Sprintf("Select a config file (%s)", m.filePicker.CurrentDirectory())
	if len(title) > pw-2 {
		title = title[:pw-5] + "..."
	}
	return RenderPopupWithTitle(title,
		clipLines(m.filePicker.View().Content, pw-2, ph-4), pw, ph, pickerPopupHint)
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
			if m.focus == PanelFilePicker {
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
	m.configVP.SetHeight(h - 4)
	// Set content to table rendered at natural width (allows horizontal scrolling)
	m.configVP.SetContent(content)
	body := m.configVP.View()
	// The value editor overlays the panel (spec amend2 REQ-8).
	if m.editing {
		// Popup size: width ~2/3 of panel, height enough for title+input+hint.
		ew, eh := editPopupSize(w)
		// Content: blank line, input, blank line, hint
		content := strings.Join([]string{
			"",
			m.editText.View(),
		}, "\n")
		edit := RenderPopupWithTitle("Edit "+m.editPath, content, ew, eh, editPopupHint)
		body = lipgloss.Place(w-2, h-3, lipgloss.Center, lipgloss.Center, edit)
	}
	// Hint row for config panel; active when the config card has focus (its
	// content is displayed in the lower area).
	return RenderPanelWithSegments(title, body, w, h, m.focus == PanelFilePicker, m.hintSegmentsFor(PanelConfig))
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

	return RenderPanelWithSegments(title, clipLines(b.String(), w-2, h-4), w, h, m.focus == PanelStatus, m.hintSegmentsFor(PanelStatus))
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
	m.eventsVP.SetHeight(h - 4)
	m.eventsVP.SetContent(b.String())
	if atBottom {
		m.eventsVP.GotoBottom()
	}

	return RenderPanelWithSegments(title, clipLines(m.eventsVP.View(), w-2, h-4), w, h, false, m.hintSegmentsFor(PanelEvents))
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
	// Clip to the terminal width: JoinVertical pads every frame line to the
	// widest one, so an over-long hint on a narrow terminal would stretch the
	// whole layout past the right edge (A1 boundary: resize stays in bounds).
	if m.confirmPending {
		return truncateANSI(m.styles.ErrorStyle.Render(
			"Pipeline is running — Press q again to quit, any other key to cancel"), m.width)
	}
	hints := []string{
		"↑↓ Navigate",
		"Tab Switch panel",
		"c Toggle config view",
		"r Run",
		"q Quit",
	}
	return truncateANSI(HintStyle().Render(strings.Join(hints, "  ")), m.width)
}

// QuitConfirmMsg is sent when user confirms quitting while pipeline is running.
type QuitConfirmMsg struct{}

// StartRunMsg signals to start the pipeline.
type StartRunMsg struct{}

// StopRunMsg signals to stop the pipeline. ctx is that run's context and
// doubles as a token, exactly like RunFinishedMsg: Update cancels only when
// it matches the current run, so a stop dispatched for a run that was already
// restarted can never cancel the newer run.
type StopRunMsg struct {
	ctx context.Context
}

// RunFinishedMsg is emitted when a run goroutine started by StartRunMsg ends
// by itself (batch completed, watch loop cancelled, or config failed to
// load). ctx is that run's own context and doubles as a token: Update clears
// isRunning only when it matches the current run's ctx, so a stale
// completion message from an earlier run can never clear the flag of a
// newer one. isRunning stays a UI-thread-only field (the message round-trip,
// not a goroutine write, clears it).
type RunFinishedMsg struct {
	ctx context.Context
}

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
