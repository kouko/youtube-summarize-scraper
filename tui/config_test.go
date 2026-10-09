package tui

import (
	"strings"
	"testing"
)

// W2-02 A3 positive (spec amend1): nested YAML renders as a sectioned tree —
// sections get heading lines, nested keys indent, arrays of scalars join.
func TestConfigViewFlattensNestedYAML(t *testing.T) {
	yaml := `
llm:
  provider: claude-api
  model: opus
channels:
  - https://a
  - https://b
whisper:
  max_duration: 1800
`
	cv := NewConfigView(yaml)
	if cv.Error() != nil {
		t.Fatalf("parse error: %v", cv.Error())
	}
	joined := strings.Join(cv.Lines(), "\n")
	for _, want := range []string{
		"llm",
		"provider: claude-api",
		"model: opus",
		"channels (2)",
		"[1] https://a",
		"[2] https://b",
		"whisper",
		"max_duration: 1800",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("tree missing %q; lines=%q", want, cv.Lines())
		}
	}
}

// W2-02 A3 positive: Toggle switches structured <-> raw, and Render
// reflects the mode.
func TestConfigViewToggle(t *testing.T) {
	yaml := "llm:\n  provider: ollama\n"
	cv := NewConfigView(yaml)
	if cv.Mode() != ConfigViewStructured {
		t.Fatalf("initial mode = %v, want structured", cv.Mode())
	}
	structured := cv.Render()
	cv.Toggle()
	if cv.Mode() != ConfigViewRaw {
		t.Fatalf("mode after toggle = %v, want raw", cv.Mode())
	}
	if cv.Render() != yaml {
		t.Errorf("raw render = %q, want original %q", cv.Render(), yaml)
	}
	cv.Toggle()
	if cv.Mode() != ConfigViewStructured || cv.Render() != structured {
		t.Errorf("toggle back did not restore structured view")
	}
}

// W2-02 A3 boundary: invalid YAML renders an error string, not a panic.
func TestConfigViewInvalidYAMLError(t *testing.T) {
	cv := NewConfigView("llm: [unclosed")
	if cv.Error() == nil {
		t.Fatal("Error() = nil, want parse error")
	}
	rendered := cv.Render()
	if !strings.Contains(rendered, "Error parsing config") {
		t.Errorf("Render = %q, want error string", rendered)
	}
}

// W2-02 A3 boundary: empty YAML renders (empty) not a panic.
func TestConfigViewEmptyYAML(t *testing.T) {
	cv := NewConfigView("")
	if cv.Error() != nil {
		t.Fatalf("empty yaml parse error: %v", cv.Error())
	}
	if got := cv.Render(); !strings.Contains(got, "(empty)") {
		t.Errorf("Render = %q, want (empty)", got)
	}
}

// W5-02 A3 positive: the tree renderer — top-level sections get heading
// lines, nested keys indent under them, list items are numbered, and no raw
// Go map[...] dump ever leaks into the view (the pre-amend1 dot-flatten
// rendered nested objects as "map[cookie:map[...] ...]" — unreadable).
func TestConfigTreeSectionsAndItems(t *testing.T) {
	yaml := `
output_dir: /tmp/out
batch:
  watch: true
  watch_interval: 10
playlists:
  - name: Watch Later
    url: https://youtube.com/playlist?list=WL
    count: 10
    cookie:
      browser: chrome
  - name: Graph History
    url: https://x
    count: 99
`
	cv := NewConfigView(yaml)
	if cv.Error() != nil {
		t.Fatalf("parse error: %v", cv.Error())
	}
	lines := cv.Lines()
	if len(lines) == 0 {
		t.Fatal("Lines() empty")
	}
	joined := strings.Join(lines, "\n")
	for _, want := range []string{
		"output_dir: /tmp/out",
		"batch", "watch: true", "watch_interval: 10",
		"playlists (2)",
		"[1]", "Watch Later", "count: 10",
		"browser: chrome",
		"[2]", "Graph History", "count: 99",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("tree missing %q; lines=%q", want, lines)
		}
	}
	if strings.Contains(joined, "map[") {
		t.Errorf("raw Go map dump leaked into the tree: %q", joined)
	}
}

// W5-02 A3: render follows document order, not map iteration order.
func TestConfigTreePreservesOrder(t *testing.T) {
	cv := NewConfigView("zzz: 1\naaa: 2\n")
	lines := cv.Lines()
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "zzz:") {
		t.Errorf("first line %q, want zzz first (document order)", lines[0])
	}
}

// W5-02 boundary: multiline scalar values are escaped so a value never
// splits the tree into misaligned rows.
func TestConfigTreeMultilineValueEscaped(t *testing.T) {
	cv := NewConfigView("llm:\n  prompt: \"line1\\nline2\"\n")
	lines := cv.Lines()
	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, "\nline2") {
		t.Errorf("value newline not escaped; lines=%q", lines)
	}
	if !strings.Contains(joined, "\\n") {
		t.Errorf("expected literal \\n in value; lines=%q", lines)
	}
}
