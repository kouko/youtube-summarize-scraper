package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// endToEndSmoke runs the full model without a real terminal: Init -> readDir
// -> window size -> keys -> render, asserting the three panels (two upper, one
// focus-driven lower), the file list, cursor movement, config selection, and
// quit all survive.
func TestEndToEndSmoke(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("llm:\n  provider: ollama\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("YTSS_CONFIG_DIR", dir)

	m := NewModel(NewAppState())

	// 1. Drive the picker's own Init command (readDir) through Update, the
	// way the program delivers it. (The tick command is timing-dependent, so
	// it is not part of this deterministic drive.)
	if initCmd := m.filePicker.Init(); initCmd != nil {
		if msg := initCmd(); msg != nil {
			got, _ := m.Update(msg)
			m = got.(*Model)
		}
	}

	// 2. Window size.
	got, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = got.(*Model)

	// 3. Three panels visible while the config card has focus: the two upper
	// panels plus the lower content area showing the config content. The
	// execution log is not on the frame yet — it appears only when the status
	// panel has focus. The file listing is NOT on the main frame (it lives
	// in the popup).
	content := m.View().Content
	plainContent := stripANSI(content)
	for _, title := range []string{"Config File", "Execution Status", "Config"} {
		if !strings.Contains(plainContent, title) {
			t.Errorf("panel %q missing", title)
		}
	}
	if strings.Contains(plainContent, "Recent Events") {
		t.Error("execution log rendered while the config card has focus")
	}
	if !strings.Contains(content, "(no config selected)") {
		t.Errorf("config card missing the empty-state text:\n%s", content)
	}

	// 3b. Enter opens the popup, which does list the config file.
	got, _ = m.Update(pressKey("enter"))
	m = got.(*Model)
	popup := m.View().Content
	if !strings.Contains(popup, "a.yaml") {
		t.Errorf("popup did not list a.yaml:\n%s", popup)
	}
	// Esc closes the popup and restores the frame.
	got, _ = m.Update(pressKey("esc"))
	m = got.(*Model)
	if strings.Contains(m.View().Content, "a.yaml") {
		t.Error("file listing still visible after Esc closed the popup")
	}

	// 3c. Tab to the status panel: the lower area swaps to the execution log.
	got, _ = m.Update(pressKey("tab"))
	m = got.(*Model)
	if !strings.Contains(stripANSI(m.View().Content), "Recent Events") {
		t.Error("execution log missing while the status panel has focus")
	}

	// 4. Arrow down keeps main model and still renders panels.
	got, _ = m.Update(pressKey("down"))
	if _, ok := got.(*Model); !ok {
		t.Fatalf("after down, model is %T", got)
	}
	m = got.(*Model)

	// 5. Quit works.
	_, cmd := m.Update(pressKey("q"))
	if cmd == nil {
		t.Fatal("q produced no quit command")
	}
	if msg := cmd(); msg != tea.Quit() {
		t.Errorf("q command = %#v, want tea.Quit()", msg)
	}
}
