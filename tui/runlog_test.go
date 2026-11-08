package tui

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
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

// makeReadOnly makes dir unwritable for the duration of the test, so opening
// the run log under it fails the way a read-only volume would.
func makeReadOnly(t *testing.T, dir string) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) }) // let TempDir cleanup remove it
}

// A6 positive (W2-01): when the log file cannot be opened (read-only output
// dir), Recent Events gains exactly one error line naming the log path and the
// cause, and nothing is written to the log file.
func TestRunLogOpenFailureShowsOneErrorEvent(t *testing.T) {
	requireEmbedded(t)
	out := t.TempDir()
	makeReadOnly(t, out)
	state := NewAppState()
	setupRunLog(t, state)

	runBatchOnce(t, writeRunLogConfig(t, out, ""), state)

	var logErrs []string
	for _, ev := range state.Snapshot().RecentEvents {
		if strings.HasPrefix(ev, "ERROR: log file") {
			logErrs = append(logErrs, ev)
		}
	}
	if len(logErrs) != 1 {
		t.Fatalf("log-writer error events = %d, want exactly 1: %q", len(logErrs), logErrs)
	}
	msg := logErrs[0]
	if !strings.Contains(msg, logFilePath(out)) {
		t.Errorf("error event does not name the log path %q: %q", logFilePath(out), msg)
	}
	if !strings.Contains(msg, "permission denied") {
		t.Errorf("error event does not name the cause: %q", msg)
	}
	if _, err := os.Stat(logFilePath(out)); !os.IsNotExist(err) {
		t.Errorf("log file exists after an open failure: err=%v", err)
	}
}

// A6 negative (W2-01): the open failure does not abort the run — later stages
// keep emitting events after the error line.
func TestRunLogOpenFailureRunContinues(t *testing.T) {
	requireEmbedded(t)
	out := t.TempDir()
	makeReadOnly(t, out)
	state := NewAppState()
	setupRunLog(t, state)

	// runBatchOnce fails this test if runPipelineWithConfig returns an error,
	// i.e. if the open failure aborted the run.
	runBatchOnce(t, writeRunLogConfig(t, out, ""), state)

	waitFor(t, func() bool {
		for _, ev := range state.Snapshot().RecentEvents {
			if strings.HasPrefix(ev, "batch complete:") {
				return true
			}
		}
		return false
	})

	evs := state.Snapshot().RecentEvents
	errIdx, doneIdx := -1, -1
	for i, ev := range evs {
		if strings.HasPrefix(ev, "ERROR: log file") {
			errIdx = i
		}
		if strings.HasPrefix(ev, "batch complete:") {
			doneIdx = i
		}
	}
	if errIdx < 0 {
		t.Fatalf("no log-writer error event in %q", evs)
	}
	if doneIdx <= errIdx {
		t.Errorf("no later event after the error line: error at %d, batch complete at %d\nevents: %q", errIdx, doneIdx, evs)
	}
}

// injectRunWriteFailure swaps the per-run LogWriter factory for one whose
// file writes succeed for the first succeedFirst records and then fail like a
// full disk. It restores the factory at test end and returns the number of
// failing writes; it is safe to read after runPipelineWithConfig returns
// because all tee writes have stopped by then (the writer is closed on exit).
func injectRunWriteFailure(t *testing.T, outputDir string, succeedFirst int) *int {
	t.Helper()
	prev := newRunLogWriter
	failures := 0
	newRunLogWriter = func(dir string, inner io.Writer) (*LogWriter, error) {
		lw, err := prev(dir, inner)
		if err != nil {
			return nil, err
		}
		full := &fs.PathError{Op: "write", Path: logFilePath(outputDir), Err: errors.New("no space left on device")}
		writes := 0
		lw.writeFailure = func() error {
			writes++
			if writes <= succeedFirst {
				return nil
			}
			failures++
			return full
		}
		return lw, nil
	}
	t.Cleanup(func() { newRunLogWriter = prev })
	return &failures
}

// A6 positive (W2-02): a mid-run write failure adds exactly one ERROR event to
// the Recent Events panel naming the log path and the cause; the run completes.
func TestRunLogWriteFailureShowsOneErrorEvent(t *testing.T) {
	requireEmbedded(t)
	out := t.TempDir()
	state := NewAppState()
	setupRunLog(t, state)

	injectRunWriteFailure(t, out, 1) // first record lands, the rest fail

	runBatchOnce(t, writeRunLogConfig(t, out, ""), state)

	var logErrs []string
	for _, ev := range state.Snapshot().RecentEvents {
		if strings.HasPrefix(ev, "ERROR: log file") {
			logErrs = append(logErrs, ev)
		}
	}
	if len(logErrs) != 1 {
		t.Fatalf("log-writer error events = %d, want exactly 1: %q", len(logErrs), logErrs)
	}
	msg := logErrs[0]
	if !strings.Contains(msg, logFilePath(out)) {
		t.Errorf("error event does not name the log path %q: %q", logFilePath(out), msg)
	}
	if !strings.Contains(msg, "no space left on device") {
		t.Errorf("error event does not name the cause: %q", msg)
	}
}

// A6 negative (W2-02): further write failures in the same run add no more
// events after the first.
func TestRunLogWriteFailureReportsOnlyOnce(t *testing.T) {
	requireEmbedded(t)
	out := t.TempDir()
	state := NewAppState()
	setupRunLog(t, state)

	failures := injectRunWriteFailure(t, out, 1)
	runBatchOnce(t, writeRunLogConfig(t, out, ""), state)

	if *failures < 2 {
		t.Fatalf("test did not exercise repeated failure: %d failing writes", *failures)
	}
	count := 0
	for _, ev := range state.Snapshot().RecentEvents {
		if strings.HasPrefix(ev, "ERROR: log file") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("log-writer error events = %d after %d failing writes, want exactly 1", count, *failures)
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

// watchRunLogConfigBody is a config body whose batch runs in watch mode with a
// zero interval, so a test can observe several iterations without waiting
// minutes between them.
func watchRunLogConfigBody(outputDir string) string {
	return "output_dir: \"" + outputDir + "\"\n" +
		"batch:\n  watch: true\n  watch_interval: 0\n"
}

// runWatchLoop starts runPipelineWithConfig in watch mode (the way the TUI's
// run key does) and once the loop has entered at least minIterations
// iterations, cancels it the way the TUI's stop key would, then waits for the
// run to end.
func runWatchLoop(t *testing.T, cfg string, state *AppState, minIterations int) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runPipelineWithConfig(cfg, state, ctx) }()

	waitFor(t, func() bool { return state.Snapshot().WatchIter >= minIterations })
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runPipelineWithConfig: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("watch loop did not stop after context cancel")
	}
}

// countRunLogWriters swaps the per-run writer factory for one that counts the
// writers the run creates, and restores the factory at test end. The counter
// is safe to read once the run has returned (the factory is only called from
// the run goroutine, and runWatchLoop synchronizes on its completion).
func countRunLogWriters(t *testing.T) *int {
	t.Helper()
	prev := newRunLogWriter
	n := 0
	newRunLogWriter = func(dir string, inner io.Writer) (*LogWriter, error) {
		lw, err := prev(dir, inner)
		if err == nil {
			n++
		}
		return lw, err
	}
	t.Cleanup(func() { newRunLogWriter = prev })
	return &n
}

// W3-01 (Acceptance 3) positive: two watch-loop iterations append to the same
// log file — the first iteration's lines are still present and the second
// iteration's lines were appended — and the writer is opened once for the
// whole run, before the loop, not once per iteration.
func TestRunLogWatchLoopAppendsAcrossIterations(t *testing.T) {
	requireEmbedded(t)
	out := t.TempDir()
	cfg := writeMinimalConfig(t, watchRunLogConfigBody(out))
	state := NewAppState()
	setupRunLog(t, state)

	writers := countRunLogWriters(t)
	runWatchLoop(t, cfg, state, 2)

	if *writers != 1 {
		t.Errorf("log writer created %d times over two watch iterations, want 1", *writers)
	}
	content := readRunLog(t, out)
	first := strings.Index(content, "watch: iteration 1 starting")
	second := strings.Index(content, "watch: iteration 2 starting")
	if first < 0 || second < 0 {
		t.Fatalf("watch iteration lines missing from the log file:\n%s", content)
	}
	if first > second {
		t.Errorf("iteration 2's lines were written before iteration 1's")
	}
}

// W3-01 (Acceptance 3) boundary: a watch-loop config reload that changes
// output_dir does not move the current run's writer mid-batch — every
// iteration keeps writing to the run-start path — and the new path is used
// only at the next run start.
func TestRunLogWatchReloadKeepsWriterPath(t *testing.T) {
	requireEmbedded(t)
	out1, out2 := t.TempDir(), t.TempDir()
	cfg := writeMinimalConfig(t, watchRunLogConfigBody(out1))

	// Rewrite the config to a different output_dir from inside the writer
	// factory: the factory runs once per run, after the run's initial
	// config.Load picked out1 and before the watch loop's first per-iteration
	// ReloadConfig, so every reload of this run sees out2 deterministically.
	prev := newRunLogWriter
	var writers []*LogWriter
	newRunLogWriter = func(dir string, inner io.Writer) (*LogWriter, error) {
		lw, err := prev(dir, inner)
		if err == nil {
			writers = append(writers, lw)
		}
		if werr := os.WriteFile(cfg, []byte(watchRunLogConfigBody(out2)), 0o644); werr != nil {
			t.Errorf("rewrite config for reload: %v", werr)
		}
		return lw, err
	}
	t.Cleanup(func() { newRunLogWriter = prev })

	state := NewAppState()
	setupRunLog(t, state)
	runWatchLoop(t, cfg, state, 2)

	if len(writers) == 0 {
		t.Fatal("no log writer created for the watch run")
	}
	if len(writers) != 1 {
		t.Errorf("log writers created = %d over two watch iterations, want 1", len(writers))
	}
	if got, want := writers[0].Path(), logFilePath(out1); got != want {
		t.Errorf("run writer path = %q, want the run-start path %q", got, want)
	}
	content := readRunLog(t, out1)
	if !strings.Contains(content, "watch: iteration 1 starting") ||
		!strings.Contains(content, "watch: iteration 2 starting") {
		t.Errorf("iteration lines did not keep going to the run-start path:\n%s", content)
	}
	if _, err := os.Stat(logFilePath(out2)); !os.IsNotExist(err) {
		t.Errorf("log path switched mid-batch: %s exists (err=%v)", logFilePath(out2), err)
	}

	// The reloaded output_dir applies at the next run start: that run's
	// writer opens the new path, the earlier run's file stays untouched.
	runBatchOnce(t, writeRunLogConfig(t, out2, ""), state)
	if len(writers) < 2 {
		t.Errorf("log writers created = %d after the next run, want 2", len(writers))
	} else {
		if got, want := writers[1].Path(), logFilePath(out2); got != want {
			t.Errorf("next run writer path = %q, want %q", got, want)
		}
	}
	if next := readRunLog(t, out2); !strings.Contains(next, "streaming batch complete") {
		t.Errorf("next run did not write to the reloaded path:\n%s", next)
	}
	if again := readRunLog(t, out1); again != content {
		t.Errorf("next run appended to the earlier run's log file")
	}
}
