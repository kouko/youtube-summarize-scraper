package tui

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/kouko/youtube-summarize-scraper/config"
	"github.com/kouko/youtube-summarize-scraper/embedded"
	"github.com/kouko/youtube-summarize-scraper/subtitle"
	"github.com/kouko/youtube-summarize-scraper/transcriber"
)

// setupRunLog wires the package log target and the default slog handler the
// way cmd/tui.go does: the handler writes through LogTarget, which forwards to
// the event bridge. It restores both when the test ends.
func setupRunLog(t *testing.T, state *AppState) *EventBridge {
	t.Helper()
	bridge := NewEventBridge(nil, state, MaxRecentEvents)
	prevTarget := LogTarget.current()
	prevDefault := slog.Default()
	LogTarget.Set(bridge)
	slog.SetDefault(slog.New(slog.NewTextHandler(LogTarget, nil)))
	t.Cleanup(func() {
		slog.SetDefault(prevDefault)
		LogTarget.Set(prevTarget)
		bridge.Close()
	})
	return bridge
}

// requireEmbedded skips a test when the embedded tool binaries cannot be
// extracted (the repo embeds only darwin-arm64).
func requireEmbedded(t *testing.T) {
	t.Helper()
	if _, err := embedded.ExtractAll(); err != nil {
		t.Skipf("embedded binaries unavailable: %v", err)
	}
}

// writeRunLogConfig writes a config whose output_dir is outputDir plus any
// extra YAML body.
func writeRunLogConfig(t *testing.T, outputDir, extra string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	body := "output_dir: \"" + outputDir + "\"\nbatch:\n  watch: false\n" + extra
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// runBatchOnce runs one non-watch batch through the TUI runner and returns
// once the run has ended.
func runBatchOnce(t *testing.T, configPath string, state *AppState) {
	t.Helper()
	if err := runPipelineWithConfig(configPath, state, context.Background()); err != nil {
		t.Fatalf("runPipelineWithConfig: %v", err)
	}
}

// readRunLog returns the run log file's content.
func readRunLog(t *testing.T, outputDir string) string {
	t.Helper()
	data, err := os.ReadFile(logFilePath(outputDir))
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	return string(data)
}

// logLines splits content into non-empty lines.
func logLines(content string) []string {
	var lines []string
	for _, line := range strings.Split(content, "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// A1 positive: after a batch the file holds the same messages the Recent
// Events panel showed.
func TestRunLogFileMatchesRecentEvents(t *testing.T) {
	requireEmbedded(t)
	out := t.TempDir()
	state := NewAppState()
	bridge := setupRunLog(t, state)
	cfg := writeRunLogConfig(t, out, "")

	runBatchOnce(t, cfg, state)

	lines := logLines(readRunLog(t, out))
	if len(lines) == 0 {
		t.Fatal("log file is empty after a batch run")
	}
	waitFor(t, func() bool { return bridge.Applied() >= uint64(len(lines)) })
	evs := state.Snapshot().RecentEvents
	for _, line := range lines {
		found := false
		for _, ev := range evs {
			if ev == line {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("log line missing from Recent Events: %q", line)
		}
	}
}

// A1 negative: a run that emits no records leaves an existing file
// byte-identical.
func TestRunLogFileUntouchedWithoutRecords(t *testing.T) {
	requireEmbedded(t)
	out := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(logFilePath(out)), 0o755); err != nil {
		t.Fatal(err)
	}
	seed := []byte("time=1 level=INFO msg=\"earlier run\"\n")
	if err := os.WriteFile(logFilePath(out), seed, 0o644); err != nil {
		t.Fatal(err)
	}

	state := NewAppState()
	setupRunLog(t, state)

	// Silence every record for this run: no bytes flow through the tee, so
	// opening the file must add nothing and truncate nothing.
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer slog.SetDefault(prev)

	runBatchOnce(t, writeRunLogConfig(t, out, ""), state)

	got, err := os.ReadFile(logFilePath(out))
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !bytes.Equal(got, seed) {
		t.Errorf("log file changed without records\nbefore: %q\nafter:  %q", seed, got)
	}
}

// A2 positive: the file is plain UTF-8 text with no ANSI escapes.
func TestRunLogFileIsPlainText(t *testing.T) {
	requireEmbedded(t)
	out := t.TempDir()
	state := NewAppState()
	setupRunLog(t, state)

	runBatchOnce(t, writeRunLogConfig(t, out, ""), state)

	data, err := os.ReadFile(logFilePath(out))
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("log file is empty")
	}
	if !utf8.Valid(data) {
		t.Errorf("log file is not valid UTF-8: %q", data)
	}
	if strings.ContainsRune(string(data), '\x1b') {
		t.Errorf("log file contains an ANSI escape byte:\n%s", data)
	}
}

// A2 boundary: every line ends with LF and keeps the slog key=value shape.
func TestRunLogFileLinesAreSlogText(t *testing.T) {
	requireEmbedded(t)
	out := t.TempDir()
	state := NewAppState()
	setupRunLog(t, state)

	runBatchOnce(t, writeRunLogConfig(t, out, ""), state)

	content := readRunLog(t, out)
	if !strings.HasSuffix(content, "\n") {
		t.Errorf("log file does not end with LF: %q", content)
	}
	for _, line := range logLines(content) {
		if !strings.HasPrefix(line, "time=") ||
			!strings.Contains(line, " level=") ||
			!strings.Contains(line, " msg=") {
			t.Errorf("line is not slog key=value text: %q", line)
		}
	}
}

// A3 positive: a second run appends and the first run's lines remain.
func TestRunLogFileAppendsAcrossRuns(t *testing.T) {
	requireEmbedded(t)
	out := t.TempDir()
	state := NewAppState()
	setupRunLog(t, state)
	cfg := writeRunLogConfig(t, out, "")

	runBatchOnce(t, cfg, state)
	first := readRunLog(t, out)

	runBatchOnce(t, cfg, state)
	second := readRunLog(t, out)

	if !bytes.HasPrefix([]byte(second), []byte(first)) {
		t.Errorf("second run did not preserve earlier content\nfirst:\n%s\nsecond:\n%s", first, second)
	}
	if len(second) == len(first) {
		t.Errorf("second run appended nothing (len still %d)", len(first))
	}
}

// A3 boundary: a fresh model/handler (a TUI process restart) keeps appending
// to the same path.
func TestRunLogFileAppendsAcrossRestart(t *testing.T) {
	requireEmbedded(t)
	out := t.TempDir()
	cfg := writeRunLogConfig(t, out, "")

	state1 := NewAppState()
	setupRunLog(t, state1)
	runBatchOnce(t, cfg, state1)
	first := readRunLog(t, out)

	// Fresh state, bridge and handler: the equivalent of restarting the TUI.
	state2 := NewAppState()
	setupRunLog(t, state2)
	runBatchOnce(t, cfg, state2)
	second := readRunLog(t, out)

	if !bytes.HasPrefix([]byte(second), []byte(first)) {
		t.Errorf("restart run did not append to the same file\nfirst:\n%s\nsecond:\n%s", first, second)
	}
	if len(second) == len(first) {
		t.Errorf("restart run appended nothing (len still %d)", len(first))
	}
}

// A4 positive: the file holds only program slog records, no raw progress
// output from external tools.
func TestRunLogFileHoldsOnlyProgramRecords(t *testing.T) {
	requireEmbedded(t)
	out := t.TempDir()
	state := NewAppState()
	setupRunLog(t, state)

	runBatchOnce(t, writeRunLogConfig(t, out, ""), state)

	lines := logLines(readRunLog(t, out))
	if len(lines) == 0 {
		t.Fatal("log file is empty")
	}
	for _, line := range lines {
		if !strings.HasPrefix(line, "time=") || !strings.Contains(line, " msg=") {
			t.Errorf("line is not a program slog record: %q", line)
		}
		if strings.Contains(line, "[download]") || strings.Contains(line, "whisper_print") {
			t.Errorf("line holds external tool progress: %q", line)
		}
	}
}

// A4 negative: raw subprocess stderr never reaches the writer.
func TestRunLogFileExcludesSubprocessStderr(t *testing.T) {
	requireEmbedded(t)
	out := t.TempDir()
	state := NewAppState()
	setupRunLog(t, state)

	var stderr bytes.Buffer
	prevSub, prevTrans := subtitle.DefaultStderr, transcriber.DefaultStderr
	subtitle.DefaultStderr = &stderr
	transcriber.DefaultStderr = &stderr
	defer func() {
		subtitle.DefaultStderr = prevSub
		transcriber.DefaultStderr = prevTrans
	}()

	const progress = "[download]  45.2% of 12.34MiB at 1.20MiB/s ETA 00:05"
	if _, err := stderr.WriteString(progress + "\n"); err != nil {
		t.Fatal(err)
	}

	runBatchOnce(t, writeRunLogConfig(t, out, ""), state)

	if !strings.Contains(stderr.String(), progress) {
		t.Fatal("test setup: the progress line did not reach the subprocess stderr sink")
	}
	if strings.Contains(readRunLog(t, out), progress) {
		t.Error("raw subprocess progress reached the log file")
	}
}

// secretFormats are common credential shapes the log file must never hold.
var secretFormats = []struct {
	name string
	re   *regexp.Regexp
}{
	{"openai-style key", regexp.MustCompile(`sk-[A-Za-z0-9_-]{16,}`)},
	{"google api key", regexp.MustCompile(`AIza[0-9A-Za-z_-]{20,}`)},
	{"assigned secret", regexp.MustCompile(`(?i)(password|secret|api[_-]?key)\s*[=:]\s*\S+`)},
	{"cookie header", regexp.MustCompile(`(?i)cookie:\s*\S+=\S+`)},
}

// scanForSecrets reports every credential match in content, including the
// given literal values (the run's configured cookie and API key).
func scanForSecrets(content string, values ...string) []string {
	var hits []string
	for _, v := range values {
		if v != "" && strings.Contains(content, v) {
			hits = append(hits, "literal:"+v)
		}
	}
	for _, f := range secretFormats {
		if m := f.re.FindString(content); m != "" {
			hits = append(hits, f.name+":"+m)
		}
	}
	return hits
}

// A5 positive: scanning a run's file finds no cookie, keychain, or API-key
// patterns even though the run's config carries them.
func TestRunLogFileHasNoSecretPatterns(t *testing.T) {
	requireEmbedded(t)
	out := t.TempDir()

	const (
		cookieValue = "COOKIE_SECRET_7d1c9a"
		apiKey      = "sk-live-0123456789abcdefghij"
	)
	cookieFile := filepath.Join(t.TempDir(), "cookies.txt")
	cookieBody := "# Netscape HTTP Cookie File\n.youtube.com\tTRUE\t/\tTRUE\t9999999999\tSID\t" + cookieValue + "\n"
	if err := os.WriteFile(cookieFile, []byte(cookieBody), 0o600); err != nil {
		t.Fatal(err)
	}

	state := NewAppState()
	setupRunLog(t, state)
	extra := "cookie:\n  file: \"" + cookieFile + "\"\n" +
		"llm:\n  claude-api:\n    api_key: \"" + apiKey + "\"\n"
	cfg := writeRunLogConfig(t, out, extra)

	// Prove the values really are in the config the run loaded.
	loaded, err := config.Load(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Cookie.File != cookieFile || loaded.LLM.ClaudeAPI.APIKey != apiKey {
		t.Fatalf("test setup: config does not carry the secrets: %+v", loaded)
	}

	runBatchOnce(t, cfg, state)

	if hits := scanForSecrets(readRunLog(t, out), cookieValue, apiKey); len(hits) > 0 {
		t.Errorf("log file leaks secrets: %v", hits)
	}
}

// A5 negative: the configured cookie value never passes through the tee.
func TestRunLogFileOmitsConfiguredCookieValue(t *testing.T) {
	requireEmbedded(t)
	out := t.TempDir()

	const cookieValue = "COOKIE_VALUE_b3e81f"
	cookieFile := filepath.Join(t.TempDir(), "cookies.txt")
	if err := os.WriteFile(cookieFile, []byte("SID\t"+cookieValue+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	state := NewAppState()
	setupRunLog(t, state)
	cfg := writeRunLogConfig(t, out, "cookie:\n  file: \""+cookieFile+"\"\n")

	runBatchOnce(t, cfg, state)

	if content := readRunLog(t, out); strings.Contains(content, cookieValue) {
		t.Errorf("configured cookie value reached the log file:\n%s", content)
	}
}

// A6 positive: the event bridge still receives every record the tee wrote, so
// the panels see what they saw before the file writer existed.
func TestRunLogKeepsEventBridgeContent(t *testing.T) {
	requireEmbedded(t)
	out := t.TempDir()
	state := NewAppState()
	bridge := setupRunLog(t, state)

	runBatchOnce(t, writeRunLogConfig(t, out, ""), state)

	lines := logLines(readRunLog(t, out))
	waitFor(t, func() bool { return bridge.Applied() >= uint64(len(lines)) })
	if got := bridge.Applied(); got != uint64(len(lines)) {
		t.Errorf("bridge applied %d records, file holds %d", got, len(lines))
	}
	evs := state.Snapshot().RecentEvents
	for _, line := range lines {
		found := false
		for _, ev := range evs {
			if ev == line {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("bridge did not deliver %q to Recent Events", line)
		}
	}
}

// A6 boundary: a successful run injects no log-writer error event.
func TestRunLogNoWriterErrorOnSuccess(t *testing.T) {
	requireEmbedded(t)
	out := t.TempDir()
	state := NewAppState()
	setupRunLog(t, state)

	runBatchOnce(t, writeRunLogConfig(t, out, ""), state)

	for _, ev := range state.Snapshot().RecentEvents {
		if strings.Contains(ev, "log file") || strings.Contains(ev, "log writer") {
			t.Errorf("unexpected log-writer event on a successful run: %q", ev)
		}
		if strings.HasPrefix(ev, "ERROR:") {
			t.Errorf("unexpected error event on a successful run: %q", ev)
		}
	}
}
