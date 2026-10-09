package tui

import (
	"os"
	"path/filepath"
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
