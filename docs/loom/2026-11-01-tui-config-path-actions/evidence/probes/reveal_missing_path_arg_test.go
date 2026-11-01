package probes

// concern: dependency-failure boundary — the reveal action must hand the
// `open` command the -R flag AND the selected config path as separate argv
// elements. Feeding the path only to stdin (or dropping it) makes real macOS
// `open -R` fail with a usage error on every invocation, so REQ-2 silently
// never reveals anything while the UI still reports success or a misleading
// failure. Guards the argv plumbing that carries the path to the command
// line, including against regressions that reorder or drop arguments.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kouko/youtube-summarize-scraper/tui"
)

// fakeOpenDir writes a stub `open` that records every argv element to a
// marker file and replaces PATH with the stub directory, so the reveal
// action is exercised end-to-end without ever touching Finder.
func fakeOpenDir(t *testing.T, marker string) {
	t.Helper()
	bin := t.TempDir()
	// echo and for are shell builtins, so the stub needs no external command
	// even though PATH is narrowed to the stub directory.
	script := "#!/bin/sh\nfor a in \"$@\"; do echo \"$a\" >> " + marker + "; done\necho \"--argv-end--\" >> " + marker + "\n"
	if err := os.WriteFile(filepath.Join(bin, "open"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
}

// TestReveal_selectedConfig_passesPathArg: with a config selected and the
// config card focused, pressing o must run `open -R <path>` — the config
// path has to arrive as an argv element, not on stdin and not dropped.
func TestReveal_selectedConfig_passesPathArg(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "open-argv.log")
	fakeOpenDir(t, marker)

	cfg := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(cfg, []byte("k: v\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	state := tui.NewAppState()
	state.UpdateConfig(cfg, "k: v\n")
	m := tui.NewModel(state)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})

	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("open was never invoked: %v", err)
	}
	args := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(args) < 2 || args[len(args)-1] != "--argv-end--" {
		t.Fatalf("open argv log malformed: %q", args)
	}
	argv := args[:len(args)-1]
	sawFlag, sawPath := false, false
	for _, a := range argv {
		if a == "-R" {
			sawFlag = true
		}
		if a == cfg {
			sawPath = true
		}
	}
	if !sawFlag {
		t.Errorf("open never received the -R flag; argv=%q", argv)
	}
	if !sawPath {
		t.Errorf("open never received the config path as an argument; argv=%q", argv)
	}
}
