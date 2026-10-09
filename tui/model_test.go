package tui

import (
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

// W4-01 A7 positive: q while running emits a confirm command, not quit.
func TestModelQWhileRunningPromptsConfirm(t *testing.T) {
	m := sizedModel()
	m.state.UpdateConfig("/tmp/x.yaml", "x")
	m.isRunning = true
	m.state.SetRunning(true)

	_, cmd := m.handleKey(pressKey("q"))
	if cmd == nil {
		t.Fatal("q while running returned nil cmd, want confirm")
	}
	if msg := cmd(); msg != (QuitConfirmMsg{}) {
		t.Errorf("cmd() = %#v, want QuitConfirmMsg{}", msg)
	}
	// The model must still be running after the first q (confirm pending).
	if !m.isRunning {
		t.Error("isRunning false after first q; the confirm was consumed as quit")
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

	content := m.renderFilePicker(60, 20)
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

// Regression: the 2x2 grid must be exactly band-sized so the top-row panels
// have equal height, the bottom row starts after it, and the hint line is
// the final visible line (user-reported: uneven columns, hint cut off).
func TestModelGridBandsAndHintAlign(t *testing.T) {
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	lines := strings.Split(m.View().Content, "\n")
	// Panels in one row are joined side-by-side, so a row band's bottom
	// border is a single line carrying both corners. Expect exactly two
	// bottom-border lines: one for the top row, one for the bottom row.
	var bottoms []int
	for i, ln := range lines {
		if strings.Contains(ln, "╰") {
			bottoms = append(bottoms, i)
		}
	}
	if len(bottoms) != 2 {
		t.Fatalf("found %d row-bottom border lines, want 2:\n%s", len(bottoms), strings.Join(lines, "\n"))
	}
	last := lines[len(lines)-1]
	if !strings.Contains(last, "↑↓ Navigate") {
		t.Errorf("last line is not the hint: %q", last)
	}
	// Every content line must carry exactly 2 left borders (4 panels, 2 per
	// row), i.e. no panel overflowed and pushed the other out of the row.
	// Count on ANSI-stripped lines so escape codes do not split the glyphs.
	ansiRe := regexp.MustCompile(`\x1b\[[0-9;>?]*[a-zA-Z]`)
	// Two side-by-side panels render two left borders per content line
	// (e.g. "│pick│status"), unless a panel is joined under the other,
	// which is what an overflow looks like. i>0 skips the border-top line.
	for i, ln := range lines[:bottoms[0]] {
		plain := ansiRe.ReplaceAllString(ln, "")
		if i > 0 && strings.Count(plain, "│") != 4 {
			t.Errorf("line %d has %d vertical borders, want 4 (two side-by-side panels): %q", i, strings.Count(plain, "│"), plain[:60])
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
