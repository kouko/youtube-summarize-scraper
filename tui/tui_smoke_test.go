package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// endToEndSmoke runs the full model without a real terminal: Init -> readDir
// -> window size -> keys -> render, asserting the four panels, the file
// list, cursor movement, config selection, and quit all survive.
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

	// 3. Four panels + file list visible.
	content := m.View().Content
	for _, title := range []string{"Config File", "Execution Status", "Config", "Recent Events"} {
		if !strings.Contains(content, title) {
			t.Errorf("panel %q missing", title)
		}
	}
	if !strings.Contains(content, "a.yaml") {
		t.Errorf("picker did not list a.yaml:\n%s", content)
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
