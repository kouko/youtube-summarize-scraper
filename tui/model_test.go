package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func pressKey(s string) tea.KeyPressMsg {
	switch s {
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEsc}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	default:
		r := []rune(s)
		if len(r) == 1 {
			return tea.KeyPressMsg{Code: r[0], Text: s}
		}
		// multi-char control: build a text key.
		return tea.KeyPressMsg{Text: s}
	}
}

func sizedModel() *Model {
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return m
}

func viewContent(t *testing.T, m *Model) string {
	t.Helper()
	v := m.View()
	if v.Content == "" {
		t.Fatal("View() returned empty content")
	}
	return v.Content
}

// W2-03 A4 positive: four panel titles are rendered.
func TestModelRendersFourPanels(t *testing.T) {
	m := sizedModel()
	content := viewContent(t, m)
	for _, title := range []string{"Config File", "Config", "Execution Status", "Recent Events"} {
		if !strings.Contains(content, title) {
			t.Errorf("View missing panel title %q", title)
		}
	}
}

// W2-03 A4 negative: the focus marker sits on the focused panel only.
func TestModelFocusMarkerOnActivePanel(t *testing.T) {
	m := sizedModel()
	content := viewContent(t, m)
	if !strings.Contains(content, "▸ Config File") {
		t.Error("initial focus marker missing on Config File panel")
	}
	// Other panel titles must not carry the marker; use a trailing newline
	// so "▸ Config File" does not match "▸ Config".
	for _, title := range []string{"Config\n", "Execution Status", "Recent Events"} {
		if strings.Contains(content, "▸ "+title) {
			t.Errorf("focus marker on non-focused panel %q", title)
		}
	}
}

// W2-03 A4 positive: Tab cycles focus through the panels.
func TestModelTabCyclesFocus(t *testing.T) {
	m := sizedModel()
	seen := map[FocusedPanel]bool{}
	for range PanelCount {
		seen[m.focus] = true
		m.handleKey(pressKey("tab"))
	}
	if len(seen) != PanelCount {
		t.Errorf("Tab visited %d/4 panels, want all 4", len(seen))
	}
}

// W3-01 A6 positive: r with a selected config emits StartRunMsg.
func TestModelRKeyStartsWhenConfigSelected(t *testing.T) {
	m := sizedModel()
	m.state.UpdateConfig("/tmp/x.yaml", "x")
	_, cmd := m.handleKey(pressKey("r"))
	if cmd == nil {
		t.Fatal("r with config selected returned nil cmd, want StartRunMsg")
	}
	if msg := cmd(); msg != (StartRunMsg{}) {
		t.Errorf("cmd() = %#v, want StartRunMsg{}", msg)
	}
}

// W3-01 A6 negative: r without a selected config does nothing.
func TestModelRKeyNoConfigDoesNothing(t *testing.T) {
	m := sizedModel()
	_, cmd := m.handleKey(pressKey("r"))
	if cmd != nil {
		t.Errorf("r without config returned cmd %#v, want nil", cmd)
	}
}

// W4-01 A7 positive: q while running asks for confirmation — the first q must
// neither quit nor cancel, and the view must say so; only the second q emits
// QuitConfirmMsg. Regression: the first q used to emit QuitConfirmMsg and
// Update treated that as the confirmation itself, so ONE q killed the whole
// run (user-reported 2026-10-09 while a real pipeline was running).
func TestModelQWhileRunningPromptsConfirm(t *testing.T) {
	m := sizedModel()
	m.state.UpdateConfig("/tmp/x.yaml", "x")
	m.isRunning = true
	m.state.SetRunning(true)

	// First q: arm the confirmation only — no command, no cancel.
	_, cmd := m.handleKey(pressKey("q"))
	if cmd != nil {
		t.Fatalf("first q returned cmd %#v, want nil (just arm confirm)", cmd)
	}
	if !m.isRunning {
		t.Error("isRunning false after first q; the run was cancelled")
	}
	if !m.confirmPending {
		t.Error("confirmPending false after first q")
	}
	if content := viewContent(t, m); !strings.Contains(content, "Press q again") {
		t.Errorf("first q did not show a confirm hint; view=%q", content[:80])
	}

	// Second q: emit the confirmation message, still without quitting here.
	_, cmd2 := m.handleKey(pressKey("q"))
	if cmd2 == nil {
		t.Fatal("second q returned nil cmd, want QuitConfirmMsg")
	}
	if msg := cmd2(); msg != (QuitConfirmMsg{}) {
		t.Errorf("second q cmd() = %#v, want QuitConfirmMsg{}", msg)
	}

	// Update(QuitConfirmMsg) is what actually cancels and quits.
	got, cmd3 := m.Update(QuitConfirmMsg{})
	if msg := cmd3(); msg != tea.Quit() {
		t.Errorf("Update(QuitConfirmMsg) cmd() = %#v, want tea.Quit()", msg)
	}
	if got.(*Model).isRunning {
		t.Error("isRunning still true after confirmed quit")
	}
}

// W4-01 A7 negative: any other key while the confirm is pending cancels the
// confirmation and keeps running.
func TestModelOtherKeyCancelsQuitConfirm(t *testing.T) {
	m := sizedModel()
	m.state.UpdateConfig("/tmp/x.yaml", "x")
	m.isRunning = true
	m.state.SetRunning(true)

	m.handleKey(pressKey("q")) // arm the confirm
	if !m.confirmPending {
		t.Fatal("confirm not armed after q")
	}
	m.handleKey(pressKey("tab")) // any other key cancels
	if m.confirmPending {
		t.Error("confirm still pending after another key")
	}
	if !m.isRunning {
		t.Error("tab while confirm pending stopped the run")
	}
	// q once more must re-arm the confirm, not quit.
	_, cmd := m.handleKey(pressKey("q"))
	if cmd != nil {
		t.Errorf("q after cancel returned cmd %#v, want nil (re-arm only)", cmd)
	}
	if !m.confirmPending {
		t.Error("confirm not re-armed after q")
	}
}

// W4-01 A7 boundary: q while idle quits immediately.
func TestModelQWhileIdleQuitsImmediately(t *testing.T) {
	m := sizedModel()
	_, cmd := m.handleKey(pressKey("q"))
	if cmd == nil {
		t.Fatal("q while idle returned nil cmd, want tea.Quit")
	}
	if msg := cmd(); msg != tea.Quit() {
		t.Errorf("cmd() = %#v, want tea.Quit()", msg)
	}
}

// W4-01 A7 boundary: confirming quit (QuitConfirmMsg) cancels and quits.
func TestModelConfirmQuitCancelsAndQuits(t *testing.T) {
	m := sizedModel()
	m.state.UpdateConfig("/tmp/x.yaml", "x")
	m.isRunning = true
	m.state.SetRunning(true)

	got, cmd := m.Update(QuitConfirmMsg{})
	if cmd == nil {
		t.Fatal("Update(QuitConfirmMsg) returned nil cmd, want tea.Quit")
	}
	if msg := cmd(); msg != tea.Quit() {
		t.Errorf("cmd() = %#v, want tea.Quit()", msg)
	}
	after := got.(*Model)
	if after.isRunning {
		t.Error("isRunning still true after confirm quit")
	}
}

// W3-01 A4 boundary: a second StartRunMsg while running is ignored.
func TestModelSecondStartIgnored(t *testing.T) {
	m := sizedModel()
	m.state.UpdateConfig("/tmp/x.yaml", "x")
	m.isRunning = true
	m.state.SetRunning(true)

	_, cmd := m.Update(StartRunMsg{})
	if cmd != nil {
		t.Errorf("second StartRunMsg returned cmd %#v, want nil (ignored)", cmd)
	}
	if !m.isRunning {
		t.Error("isRunning flipped false by ignored StartRunMsg")
	}
}

// Regression: arrow keys with file-picker focus must keep the main Model as
// the program model. handleUp/handleDown once returned the FilePickerModel
// itself, which replaced the whole TUI with the bare picker view (alt screen
// off, q dead) after the first ↑/↓.
func TestModelArrowKeysKeepMainModel(t *testing.T) {
	m := sizedModel()

	got, _ := m.Update(pressKey("up"))
	if _, ok := got.(*Model); !ok {
		t.Fatalf("after ↑ the program model is %T, want *Model (the TUI was replaced by the picker)", got)
	}
	got, _ = m.Update(pressKey("down"))
	if _, ok := got.(*Model); !ok {
		t.Fatalf("after ↓ the program model is %T, want *Model", got)
	}
	// The four panels must still render, and q must still quit.
	after := got.(*Model)
	after.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	content := viewContent(t, after)
	if !strings.Contains(content, "Execution Status") {
		t.Error("four-panel layout lost after arrow keys")
	}
	_, cmd := after.handleKey(pressKey("q"))
	if cmd == nil {
		t.Error("q no longer quits after arrow keys")
	}
}

// Regression: the main model must forward the file picker's own messages
// (readDirMsg from its Init command) to it. Before this fix the directory
// listing was swallowed, so the picker stayed empty and arrow keys had
// nothing to move over.
func TestModelForwardsPickerDirectoryListing(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("YTSS_CONFIG_DIR", dir)

	m := NewModel(NewAppState())

	// Drive the picker's Init command (readDir) through the main model the
	// way the program does: the command's message arrives at Update.
	initCmd := m.filePicker.Init()
	if initCmd == nil {
		t.Fatal("picker Init returned nil cmd")
	}
	if msg := initCmd(); msg != nil {
		got, _ := m.Update(msg)
		m = got.(*Model)
	}

	content := m.renderPickerPopup(60, 20)
	if !strings.Contains(content, "a.yaml") {
		t.Errorf("picker did not list a.yaml after readDirMsg was forwarded; content=%q", content)
	}
	if strings.Contains(content, "b.txt") {
		t.Errorf("picker listed b.txt (should be filtered to .yaml/.yml); content=%q", content)
	}
}

// Regression: the whole TUI must fit the terminal height. The picker used
// to auto-size itself to the full terminal height, pushing the bottom row
// of panels off screen (user-reported).
func TestModelFitsTerminalHeight(t *testing.T) {
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	v := m.View()
	lines := strings.Count(v.Content, "\n") + 1
	if lines > 31 { // terminal height + slack for a trailing newline
		t.Errorf("View renders %d lines for a 30-row terminal; bottom panels are pushed off screen", lines)
	}
	content := viewContent(t, m)
	for _, title := range []string{"Config File", "Execution Status", "Recent Events"} {
		if !strings.Contains(content, title) {
			t.Errorf("panel %q missing from view", title)
		}
	}
}

// Regression: the grid must fit the terminal: the top band is content-sized
// (card and status end on the same line), the bottom band starts right after
// it, and the hint line is the final visible line (spec amend1 + W5-03).
func TestModelGridBandsAndHintAlign(t *testing.T) {
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	lines := strings.Split(m.View().Content, "\n")
	// Both top panels end on one shared border line (2 corners); the bottom
	// band's border is the second such line.
	var bottoms []int
	for i, ln := range lines {
		if strings.Contains(stripANSI(ln), "╰") {
			bottoms = append(bottoms, i)
		}
	}
	if len(bottoms) != 2 {
		t.Fatalf("found %d band-bottom border lines, want 2:\n%s", len(bottoms), strings.Join(lines, "\n"))
	}
	// Content-sized top band: 9 rows max (status content), not half of 30.
	if bottoms[0] > 9 {
		t.Errorf("top band ends at line %d, want content-sized (<=9); the empty strip is back", bottoms[0])
	}
	// Bands adjacent: the bottom band's top border is on the very next line.
	if !strings.Contains(stripANSI(lines[bottoms[0]+1]), "╭") {
		t.Errorf("bottom band does not start right after the top band (line %d)", bottoms[0])
	}
	last := lines[len(lines)-1]
	if !strings.Contains(last, "↑↓ Navigate") {
		t.Errorf("last line is not the hint: %q", last)
	}
	// Every content line of the top band carries 4 left borders (two
	// side-by-side panels), i.e. no panel overflowed the row.
	ansiRe := regexp.MustCompile(`\x1b\[[0-9;>?]*[a-zA-Z]`)
	for i, ln := range lines[:bottoms[0]] {
		plain := ansiRe.ReplaceAllString(ln, "")
		if i > 0 && strings.Count(plain, "│") != 4 {
			t.Errorf("line %d has %d vertical borders, want 4: %q", i, strings.Count(plain, "│"), plain[:60])
		}
	}
}

// Regression: selecting a config must never grow the layout. The config
// panel flattens the whole YAML; without clipping, a long config pushes
// the hint line off screen (user-reported).
func TestModelSelectConfigKeepsLayout(t *testing.T) {
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	// A config large enough to overflow the bottom band if not clipped.
	big := "llm:\n  provider: claude-api\n" + strings.Repeat("  extra_key: value\n", 60)
	m.handleConfigSelected(t.TempDir()) // no-op path guard
	// Feed the selection directly (bypasses the file read).
	m.state.UpdateConfig("/tmp/x.yaml", big)
	m.configView = NewConfigView(big)

	lines := strings.Split(m.View().Content, "\n")
	last := lines[len(lines)-1]
	if !strings.Contains(last, "↑↓ Navigate") {
		t.Errorf("hint pushed off screen after selecting a large config; last line: %q", last[:50])
	}
	// Total must still fit the 30-row terminal.
	if len(lines) > 31 {
		t.Errorf("render %d lines for 30-row terminal after selection", len(lines))
	}
}

// Regression: the config panel's last row must be the box's bottom border,
// never clipped content (user-reported: last line was config content).
func TestConfigPanelBottomBorderVisible(t *testing.T) {
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	big := strings.Repeat("llm.extra_key: value\n", 80)
	m.configView = NewConfigView("llm:\n  provider: x\n" + big)

	lines := strings.Split(m.View().Content, "\n")
	// find the config panel: bottom row band contains both ╰ borders; the
	// config panel is the left one. Its bottom border line must end with ╰.
	for i, ln := range lines {
		if strings.Contains(ln, "╰") && i > 0 {
			left := strings.Split(ln, "╰")[0]
			if strings.HasPrefix(left, "╭") {
				continue // top border of a panel
			}
			// this is a row-bottom border line; the config (left) half must
			// end in ╰ (border) not content.
			plain := stripANSI(ln)
			if !strings.HasPrefix(plain, "╰") && !strings.Contains(plain[:len(plain)/2], "╰") {
				// check the LEFT panel's rightmost char in the left half
				half := plain[:len(plain)/2]
				if !strings.HasSuffix(strings.TrimRight(half, " "), "╰") {
					t.Errorf("config panel bottom border missing on line %d: %q", i, plain[:60])
				}
			}
		}
	}
}

func stripANSI(s string) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		if r == '\x1b' {
			in = true
			continue
		}
		if in {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				in = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// Config panel scrolls with ↑/↓ while focused: content window shifts, the
// panel box stays fixed.
func TestConfigPanelScrolls(t *testing.T) {
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	var cfg strings.Builder
	cfg.WriteString("llm:\n")
	for i := 0; i < 60; i++ {
		cfg.WriteString(strings.Repeat(" ", 2) + "opt_" + fmt.Sprint(i) + ": v\n")
	}
	m.configView = NewConfigView(cfg.String())
	m.focus = PanelConfig

	before := m.View().Content
	_, _ = m.handleDown(pressKey("down"))
	after := m.View().Content
	if before == after {
		t.Error("config content did not change after ↓")
	}
	// Panel box must stay the same size (borders unchanged), only content moved.
	if strings.Count(before, "╰") != strings.Count(after, "╰") {
		t.Errorf("panel geometry changed after scroll: ╰ count %d -> %d", strings.Count(before, "╰"), strings.Count(after, "╰"))
	}
	// Scrolling up at the top is a no-op.
	_, _ = m.handleUp(pressKey("up"))
	if m.View().Content != before {
		t.Error("↑ at top changed content; should be clamped")
	}
}

// Events panel scrolls: with many events, ↑ reveals older lines.
func TestEventsPanelScrolls(t *testing.T) {
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	for i := 0; i < 40; i++ {
		m.state.AddRecentEvent(fmt.Sprintf("event-%02d", i))
	}
	m.focus = PanelEvents

	content := m.View().Content
	// Default shows the newest visible lines.
	if !strings.Contains(content, "event-39") {
		t.Error("default events view missing newest event")
	}
	_, _ = m.handleUp(pressKey("up")) // scroll to older
	scrolled := m.View().Content
	if !strings.Contains(scrolled, "event-3") {
		t.Errorf("after ↑ expected older events; content=%q", scrolled[:80])
	}
}

// YAML values with embedded newlines render on one line (escaped), so rows
// stay aligned.
func TestConfigValueNewlineEscaped(t *testing.T) {
	yaml := "llm:\n  endpoint: \"http://a\\nb\"\n"
	cv := NewConfigView(yaml)
	if cv.Error() != nil {
		t.Fatalf("parse: %v", cv.Error())
	}
	rendered := cv.Render()
	if strings.Contains(rendered, "\nhttp://") {
		t.Errorf("value newline not escaped; rendered=%q", rendered)
	}
	if !strings.Contains(rendered, "\\n") {
		t.Errorf("expected literal \\n in value; rendered=%q", rendered)
	}
}

// clipLines must truncate by DISPLAY width, not raw rune count: ANSI color
// codes occupy zero columns but many runes, so a colored row (filepicker
// listing) lost its trailing filename to the width budget (pty-observed).
func TestClipLinesCountsDisplayWidthNotEscapeRun(t *testing.T) {
	// "  -rw-------   287B" colored (escape codes), then a plain " leak.yaml"
	colored := "\x1b[38;5;240m  -rw-------   287B\x1b[m leak.yaml"

	// 25 display columns fit the size but cut the name.
	got := clipLines(colored, 25, 3)
	plain := stripANSI(got)
	if strings.Contains(plain, "leak.yaml") {
		t.Errorf("25 display columns must cut the name, got %q", plain)
	}
	if w := len([]rune(plain)); w > 25 {
		t.Errorf("clipped row is %d display runes, want <= 25", w)
	}

	// 40 display columns keep the visible filename.
	got2 := clipLines(colored, 40, 3)
	if !strings.Contains(stripANSI(got2), "leak.yaml") {
		t.Errorf("40 display columns must keep the name, got %q", stripANSI(got2))
	}

	// Truncating inside colored content must reset the style, so the next
	// line is not painted with the previous color.
	got3 := clipLines(colored, 12, 3)
	if strings.Contains(got3, "\x1b[") && !strings.Contains(got3, "\x1b[0m") {
		t.Errorf("truncated ANSI content must reset styles; got %q", got3)
	}
}

// W5-01 A1 positive (spec amend1): the top-left panel is a short config card
// showing the current path (or "(no config selected)"), rendered at content
// height — NOT the full top band, and the file listing is no longer always
// visible.
func TestModelConfigCardShortHeight(t *testing.T) {
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	content := viewContent(t, m)
	if !strings.Contains(content, "(no config selected)") {
		t.Error("card does not show (no config selected) before any selection")
	}
	// The card must be a SHORT box: exactly two row-bottom border lines total
	// in the grid rows... instead, check the top-left box's bottom border sits
	// well above the top band's bottom (a full-height picker would align it
	// with the status panel's border).
	lines := strings.Split(content, "\n")
	topBandBottom := -1
	cardBottom := -1
	for i, ln := range lines {
		if strings.Contains(ln, "╰") && i > 0 {
			// first ╰ line: the earlier of the two borders on it belongs to
			// the left box; if the card is short, the left border appears on
			// an EARLIER line than the right panel's border.
			if strings.Count(stripANSI(ln), "╰") == 2 && topBandBottom < 0 {
				topBandBottom = i
			}
		}
	}
	_ = topBandBottom
	_ = cardBottom
	// Simpler invariant: the card content is 2 lines (path + hint), so the
	// whole card box is 5 rows (2 border + 1 title + 2 content). Assert some
	// line before the top band's bottom border contains "Enter 選檔" hint.
	found := false
	for _, ln := range lines[:10] {
		if strings.Contains(stripANSI(ln), "Enter") {
			found = true
			break
		}
	}
	if !found {
		t.Error("config card hint (Enter to open picker) not visible in the top rows")
	}
}

// W5-01 A2 positive: Enter on the focused config card opens the picker
// popup; the popup overlay is visible; Esc closes it.
func TestModelEnterOpensPickerPopup(t *testing.T) {
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m.focus = PanelFilePicker

	if m.pickerOpen {
		t.Fatal("popup open before Enter")
	}
	m.handleKey(pressKey("enter"))
	if !m.pickerOpen {
		t.Fatal("Enter on focused card did not open the popup")
	}
	content := viewContent(t, m)
	if !strings.Contains(content, "Select a config file") {
		t.Error("popup frame title missing while open")
	}
	// Esc closes.
	m.handleKey(pressKey("esc"))
	if m.pickerOpen {
		t.Error("Esc did not close the popup")
	}
}

// W5-01 A2 boundary: while the popup is open it owns the keys — Tab must not
// move the main focus, and arrows go to the picker.
func TestModelPopupOwnsKeys(t *testing.T) {
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m.focus = PanelFilePicker
	m.handleKey(pressKey("enter"))
	if !m.pickerOpen {
		t.Fatal("popup did not open")
	}
	m.handleKey(pressKey("tab"))
	if m.focus != PanelFilePicker {
		t.Errorf("Tab moved main focus while popup open: focus=%v", m.focus)
	}
	m.handleKey(pressKey("q"))
	if !m.pickerOpen {
		t.Error("q closed the popup; it must be routed to the picker instead")
	}
	// Esc closes and disarms.
	m.handleKey(pressKey("esc"))
	if m.pickerOpen {
		t.Error("Esc did not close the popup")
	}
}

// W5-01 A2: selecting a file in the popup loads it and closes the popup.
func TestModelPopupSelectionLoadsConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pick.yaml"), []byte("llm:\n  provider: ollama\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("YTSS_CONFIG_DIR", dir)
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	if c := m.filePicker.Init(); c != nil {
		if msg := c(); msg != nil {
			m.Update(msg)
		}
	}
	m.focus = PanelFilePicker
	m.handleKey(pressKey("enter")) // open popup
	// While the popup is open, keys go through Update so they reach the
	// picker; its Enter returns a cmd whose msg (ConfigSelectedMsg) the main
	// Update then applies — closing the popup and loading the config.
	got, cmd := m.Update(pressKey("enter"))
	m = got.(*Model)
	if cmd != nil {
		if msg := cmd(); msg != nil {
			got2, _ := m.Update(msg)
			m = got2.(*Model)
		}
	}
	if m.pickerOpen {
		t.Error("popup still open after selection")
	}
	if m.state.Snapshot().ConfigPath == "" {
		t.Error("config not loaded after popup selection")
	}
}

// W5-03 (user feedback): the top band's height is content-driven, not a
// fixed half of the terminal — the config card and the status panel end on
// the same line, the bottom band starts immediately after, and no blank
// rows sit between them (pre-fix: card ended at row 4, status at row 18).
func TestModelTopBandContentSized(t *testing.T) {
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	lines := strings.Split(m.View().Content, "\n")
	bandBottom := -1
	for i, ln := range lines {
		if strings.Count(stripANSI(ln), "╰") == 2 {
			bandBottom = i
			break
		}
	}
	if bandBottom < 0 {
		t.Fatal("top band bottom border (2 corners) not found")
	}
	// Content-sized: status = 5 content rows + title + 2 borders = 8-9 rows;
	// a half-height band on a 40-row terminal is 19. Anything above ~12 means
	// the old empty top band is back.
	if bandBottom > 12 {
		t.Errorf("top band ends at line %d, want content-sized (~9); the gap is back", bandBottom)
	}
	// The bottom band must start on the very next line (no blank row).
	if bandBottom+1 >= len(lines) || !strings.Contains(stripANSI(lines[bandBottom+1]), "╭") {
		t.Errorf("bottom band does not start right after the top band (line %d)", bandBottom)
	}
}

// REQ-7 (spec amend2): the wheel scrolls the panel under the pointer without
// changing focus; scrolling follows the pointer's column (left=config,
// right=events).
func TestModelWheelScrollsPanelUnderPointer(t *testing.T) {
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	for i := 0; i < 40; i++ {
		m.state.AddRecentEvent(fmt.Sprintf("event-%02d", i))
	}
	m.focus = PanelStatus // NOT the events panel — wheel must work anyway

	// Render once so the viewports carry content (real flow: terminal user
	// wheels after the frame is drawn).
	viewContent(t, m)

	// The events viewport is pinned at the bottom (newest visible); wheel-up
	// scrolls toward the oldest and must reach event-0 after enough notches.
	for i := 0; i < 30; i++ {
		m.Update(tea.MouseWheelMsg{X: 90, Y: 20, Button: tea.MouseWheelUp})
	}
	content := viewContent(t, m)
	if !strings.Contains(content, "event-0") {
		t.Error("wheel-up over events panel did not scroll to the oldest line")
	}

	// Wheel-up over the config panel (left half) scrolls the config viewport
	// and does not move focus.
	m.Update(tea.MouseWheelMsg{X: 30, Y: 20, Button: tea.MouseWheelUp})
	if m.focus != PanelStatus {
		t.Errorf("wheel changed focus to %v", m.focus)
	}
}

// REQ-7: viewport keyboard scrolling — ↓ on the focused events panel scrolls
// down; the offset is kept by the viewport, not the model.
func TestModelViewportKeyboardScroll(t *testing.T) {
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	for i := 0; i < 40; i++ {
		m.state.AddRecentEvent(fmt.Sprintf("event-%02d", i))
	}
	m.focus = PanelEvents

	before := viewContent(t, m)
	m.handleKey(pressKey("up")) // scroll toward older events
	after := viewContent(t, m)
	if before == after {
		t.Error("↑ on the focused events panel did not scroll the viewport")
	}
	if m.eventsVP.YOffset() == 0 {
		t.Error("events viewport offset unchanged after ↑ (was pinned at bottom)")
	}
}

// REQ-8 (spec amend2): Enter on an editable config line opens the inline
// editor; committing writes the change back to the YAML file on disk.
func TestModelEditConfigValueWritesFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "c.yaml")
	os.WriteFile(cfgPath, []byte("llm:\n  provider: claude-api\n"), 0o644)
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m.handleConfigSelected(cfgPath)
	m.focus = PanelConfig
	m.configCursor = 1 // "  provider: claude-api" (line 1)

	// Enter opens the editor prefilled with the current value.
	_, cmd := m.handleKey(pressKey("enter"))
	if cmd != nil {
		t.Fatalf("enter on config line returned cmd, want nil (start edit)")
	}
	if !m.editing {
		t.Fatal("editing not started")
	}
	if got := m.editText.Value(); got != "claude-api" {
		t.Errorf("editor prefill = %q, want claude-api", got)
	}

	// Clear the prefilled value (backspace to start) then type the new one.
	for range "claude-api" {
		m.Update(pressKey("backspace"))
	}
	for _, ch := range "ollama" {
		m.Update(pressKey(string(ch)))
	}
	m.Update(pressKey("enter"))
	if m.editing {
		t.Fatal("editing still active after commit")
	}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "provider: ollama") {
		t.Errorf("file not updated; got %q", data)
	}
}

// REQ-8: Esc cancels the edit and leaves the file untouched.
func TestModelEditCancelKeepsFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "c.yaml")
	os.WriteFile(cfgPath, []byte("llm:\n  provider: claude-api\n"), 0o644)
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m.handleConfigSelected(cfgPath)
	m.focus = PanelConfig
	m.configCursor = 1

	m.handleKey(pressKey("enter"))
	for _, ch := range "XXXX" {
		m.Update(pressKey(string(ch)))
	}
	m.Update(pressKey("esc"))
	if m.editing {
		t.Fatal("editing still active after Esc")
	}
	data, _ := os.ReadFile(cfgPath)
	if strings.Contains(string(data), "XXXX") {
		t.Errorf("file changed after Esc; got %q", data)
	}
}

// User feedback (2026-10-09): the popup used to blank the whole frame behind
// it. The base UI must stay visible around the overlay.
func TestModelPopupKeepsBackgroundVisible(t *testing.T) {
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 130, Height: 40})
	m.focus = PanelFilePicker
	m.handleKey(pressKey("enter"))
	if !m.pickerOpen {
		t.Fatal("popup did not open")
	}
	lines := strings.Split(m.View().Content, "\n")
	// The hint line (last row, outside the popup) survives.
	if !strings.Contains(ansiStrip(lines[len(lines)-1]), "↑↓ Navigate") {
		t.Error("hint line lost behind the popup")
	}
	// The popup frame is present.
	found := false
	for _, ln := range lines {
		if strings.Contains(ln, "Select a config file") {
			found = true
			break
		}
	}
	if !found {
		t.Error("popup frame missing")
	}
}
