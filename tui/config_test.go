package tui

import (
	"strings"
	"testing"
)

// W2-02 A3 positive: nested YAML flattens with dot-joined keys and
// comma-joined arrays.
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
	rows := cv.Rows()
	joined := strings.Join(flat(rows), "\n")
	for _, want := range []string{
		"llm.provider.", "claude-api",
		"llm.model.", "opus",
		"channels.", "https://a, https://b",
		"whisper.max_duration.", "1800",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("rows missing %q; rows=%v", want, rows)
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

func flat(rows [][]string) []string {
	out := make([]string, 0, len(rows)*2)
	for _, r := range rows {
		out = append(out, r[0], r[1])
	}
	return out
}
