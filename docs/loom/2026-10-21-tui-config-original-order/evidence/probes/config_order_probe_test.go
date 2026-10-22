package probes

// concern: regression that re-introduces the TUI config display shuffle or the
// write-back corruption — top-level sections no longer render in the file's
// order or differ between parses, or an edit drops the legacy top-level
// random_order key, reorders the document, or loses its comments and the
// nested batch.random_order setting.

import (
	"slices"
	"strings"
	"testing"

	"github.com/kouko/youtube-summarize-scraper/tui"
)

// topLevelRows returns the depth-0 keys of rendered view lines, in order. A
// depth-0 row (heading or scalar) starts at column zero; nested rows are
// indented, and a scalar row is "key" + padding + "value".
func topLevelRows(lines []string) []string {
	rows := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" || strings.HasPrefix(line, "  ") {
			continue
		}
		key := line
		if i := strings.Index(line, "  "); i >= 0 {
			key = line[:i]
		}
		rows = append(rows, key)
	}
	return rows
}

// Test_configorder_repeat_identical pins that a config with a legacy top-level
// random_order key renders every top-level section in the file's order, and
// that repeated parses of the same content render identically (no shuffle,
// no random seed state).
func Test_configorder_repeat_identical(t *testing.T) {
	const src = "random_order: true\n" +
		"output_dir: /tmp/out\n" +
		"llm:\n" +
		"  provider: claude-api\n" +
		"whisper:\n" +
		"  max_duration: 1800\n" +
		"batch:\n" +
		"  random_order: true\n"
	want := []string{"random_order", "output_dir", "llm", "whisper", "batch"}

	var firstLines []string
	var firstRender string
	for parse := 1; parse <= 3; parse++ {
		cv := tui.NewConfigView(src)
		if cv.Error() != nil {
			t.Fatalf("parse %d: %v", parse, cv.Error())
		}
		lines := cv.Lines()
		if got := topLevelRows(lines); !slices.Equal(got, want) {
			t.Fatalf("parse %d: top-level rows %q, want file order %q; lines=%q", parse, got, want, lines)
		}
		if len(lines) == 0 || !strings.HasPrefix(lines[0], "random_order") {
			t.Fatalf("parse %d: first line %q, want the legacy random_order row first (file order)", parse, lines)
		}
		if !strings.Contains(strings.Join(lines, "\n"), "random_order") {
			t.Fatalf("parse %d: legacy top-level random_order key vanished from the view; lines=%q", parse, lines)
		}
		render := cv.Render()
		if parse == 1 {
			firstLines, firstRender = lines, render
			continue
		}
		if !slices.Equal(lines, firstLines) || render != firstRender {
			t.Fatalf("parse %d rendered differently from parse 1 (shuffle or per-parse state); lines=%q want=%q", parse, lines, firstLines)
		}
	}
}

// Test_configorder_writeback_preserved pins that parse -> SetValue ->
// Serialized keeps the top-level order and comments, keeps the legacy
// top-level random_order key as ordinary data, and leaves the nested
// batch.random_order setting in place.
func Test_configorder_writeback_preserved(t *testing.T) {
	const src = "# top comment\n" +
		"random_order: true\n" +
		"output_dir: /tmp/out\n" +
		"llm:\n" +
		"  provider: claude-api # inline comment\n" +
		"batch:\n" +
		"  random_order: true\n"
	cv := tui.NewConfigView(src)
	if cv.Error() != nil {
		t.Fatalf("parse: %v", cv.Error())
	}
	if err := cv.SetValue("llm.provider", "openai"); err != nil {
		t.Fatalf("SetValue(llm.provider): %v", err)
	}
	out, err := cv.Serialized()
	if err != nil {
		t.Fatalf("Serialized: %v", err)
	}

	if !strings.Contains(out, "provider: openai") || strings.Contains(out, "claude-api") {
		t.Errorf("edit not applied on write-back; out=%q", out)
	}
	if !strings.Contains(out, "random_order: true") {
		t.Errorf("legacy top-level random_order key lost on write-back; out=%q", out)
	}
	legacy := strings.Index(out, "random_order: true")
	outputDir := strings.Index(out, "output_dir")
	llm := strings.Index(out, "llm:")
	batch := strings.Index(out, "batch:")
	if !(legacy >= 0 && legacy < outputDir && outputDir < llm && llm < batch) {
		t.Errorf("top-level order not preserved on write-back; out=%q", out)
	}
	if !strings.Contains(out, "# top comment") {
		t.Errorf("leading comment lost on write-back; out=%q", out)
	}
	if !strings.Contains(out, "# inline comment") {
		t.Errorf("inline comment lost on write-back; out=%q", out)
	}
	if n := strings.Count(out, "random_order: true"); n != 2 {
		t.Errorf("want the legacy top-level key and the nested batch.random_order key on write-back, got %d occurrences; out=%q", n, out)
	}
	if !strings.Contains(out, "batch:") || strings.LastIndex(out, "random_order: true") < batch {
		t.Errorf("batch.random_order no longer under batch after write-back; out=%q", out)
	}
}
