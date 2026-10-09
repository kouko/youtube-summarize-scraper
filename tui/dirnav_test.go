package tui

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// Verify the DirAllowed=false claim directly against bubbles v2.2.1: Enter
// on a directory must navigate into it (browsing), while only .yaml/.yml
// files can be selected (DirAllowed=false blocks selecting a dir as Path).
func TestDirNavigationWithDirAllowedFalse(t *testing.T) {
	base := t.TempDir()
	sub := filepath.Join(base, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "a.yaml"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	fp := NewFilePickerModel()
	fp.fp.CurrentDirectory = base

	// Drive the lazy directory load the way bubbles does: Init returns a
	// command that produces the readDirMsg; feed it back through Update.
	if cmd := fp.fp.Init(); cmd != nil {
		if msg := cmd(); msg != nil {
			var c tea.Cmd
			fp.fp, c = fp.fp.Update(msg)
			if c != nil {
				if msg2 := c(); msg2 != nil {
					fp.fp, _ = fp.fp.Update(msg2)
				}
			}
		}
	}

	// Enter on the (first, directory-first sorted) entry navigates into sub.
	got, _ := fp.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	after := got.(*FilePickerModel)
	if after.fp.CurrentDirectory != sub {
		t.Fatalf("CurrentDirectory = %q, want %q — Enter must navigate into sub even when DirAllowed=false", after.fp.CurrentDirectory, sub)
	}
	if after.fp.Path != "" {
		t.Fatalf("Path = %q, want empty — a directory must not be selectable as Path", after.fp.Path)
	}
}
