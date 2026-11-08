package tui

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"unicode/utf8"
)

func newTestLogWriter(t *testing.T, outputDir string, inner io.Writer) *LogWriter {
	t.Helper()
	w, err := NewLogWriter(outputDir, inner)
	if err != nil {
		t.Fatalf("NewLogWriter(%q): %v", outputDir, err)
	}
	t.Cleanup(func() { w.Close() })
	return w
}

func logFilePath(outputDir string) string {
	return filepath.Join(outputDir, "logs", "ytss.log")
}

// A1 positive: the writer creates <output_dir>/logs/ytss.log holding every
// record passed to it, and still delivers each record to the existing writer
// (the event bridge in production).
func TestLogWriterCreatesFile(t *testing.T) {
	out := t.TempDir()
	var inner bytes.Buffer
	w := newTestLogWriter(t, out, &inner)

	logger := slog.New(slog.NewTextHandler(w, nil))
	logger.Info("streaming: processing video", "video_id", "abc123")
	logger.Error("stage failed", "stage", "transcribe")

	got, err := os.ReadFile(logFilePath(out))
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	content := string(got)
	for _, want := range []string{
		`msg="streaming: processing video"`,
		`video_id=abc123`,
		`level=ERROR`,
		`msg="stage failed"`,
		`stage=transcribe`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("log file missing %q\ngot:\n%s", want, content)
		}
	}

	// The tee keeps the existing flow intact: the same records reach inner.
	innerContent := inner.String()
	for _, want := range []string{`msg="streaming: processing video"`, `msg="stage failed"`} {
		if !strings.Contains(innerContent, want) {
			t.Errorf("inner writer missing %q\ngot:\n%s", want, innerContent)
		}
	}
}

// A1 negative: a read-only output dir returns an open error instead of
// panicking.
func TestLogWriterReadOnlyOutputDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	out := t.TempDir()
	if err := os.Chmod(out, 0o555); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	w, err := NewLogWriter(out, io.Discard)
	if err == nil {
		w.Close()
		t.Fatal("NewLogWriter on read-only output dir: want error, got nil")
	}
}

// A2 positive: written bytes are plain UTF-8 slog text lines with no ANSI
// escapes.
func TestLogWriterPlainText(t *testing.T) {
	out := t.TempDir()
	w := newTestLogWriter(t, out, io.Discard)

	logger := slog.New(slog.NewTextHandler(w, nil))
	logger.Info("streaming: processing channel video", "url", "https://example.com/watch?v=1")

	got, err := os.ReadFile(logFilePath(out))
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !utf8.Valid(got) {
		t.Errorf("log file is not valid UTF-8: %q", got)
	}
	if strings.ContainsRune(string(got), '\x1b') {
		t.Errorf("log file contains an ANSI escape byte\ngot:\n%s", got)
	}
	lines := strings.Split(strings.TrimRight(string(got), "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("want 1 log line, got %d:\n%s", len(lines), got)
	}
	line := lines[0]
	if !strings.HasPrefix(line, "time=") {
		t.Errorf("log line does not start with time=: %q", line)
	}
	for _, want := range []string{"level=INFO", `msg="streaming: processing channel video"`, `url="https://example.com/watch?v=1"`} {
		if !strings.Contains(line, want) {
			t.Errorf("log line missing %q\ngot: %q", want, line)
		}
	}
}

// A2 boundary: with the umask zeroed, the logs directory is created 0755 and
// the file 0644.
func TestLogWriterCreationModes(t *testing.T) {
	old := syscall.Umask(0)
	defer syscall.Umask(old)

	out := t.TempDir()
	newTestLogWriter(t, out, io.Discard)

	di, err := os.Stat(filepath.Join(out, "logs"))
	if err != nil {
		t.Fatalf("stat logs dir: %v", err)
	}
	if got := di.Mode().Perm(); got != 0o755 {
		t.Errorf("logs dir mode = %o, want 755", got)
	}
	fi, err := os.Stat(logFilePath(out))
	if err != nil {
		t.Fatalf("stat log file: %v", err)
	}
	if got := fi.Mode().Perm(); got != 0o644 {
		t.Errorf("log file mode = %o, want 644", got)
	}
}

// A3 positive: a second writer opened on the same path appends and keeps
// earlier lines.
func TestLogWriterAppendsAcrossWriters(t *testing.T) {
	out := t.TempDir()

	w1 := newTestLogWriter(t, out, io.Discard)
	slog.New(slog.NewTextHandler(w1, nil)).Info("first run record")
	w1.Close()

	w2 := newTestLogWriter(t, out, io.Discard)
	slog.New(slog.NewTextHandler(w2, nil)).Info("second run record")
	w2.Close()

	got, err := os.ReadFile(logFilePath(out))
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	content := string(got)
	if !strings.Contains(content, "msg=\"first run record\"") {
		t.Errorf("earlier run's line lost\ngot:\n%s", content)
	}
	if !strings.Contains(content, "msg=\"second run record\"") {
		t.Errorf("second run's line missing\ngot:\n%s", content)
	}
	if got := strings.Count(content, "first run record"); got != 1 {
		t.Errorf("first run record appears %d times, want 1", got)
	}
}

// openFlags reads the open file status flags via fcntl(F_GETFL). Uses the
// raw syscall because syscall.FcntlFcntl is Linux-only.
func openFlags(t *testing.T, w *LogWriter) int {
	t.Helper()
	flags, _, errno := syscall.Syscall(syscall.SYS_FCNTL, w.file.Fd(), uintptr(syscall.F_GETFL), 0)
	if errno != 0 {
		t.Fatalf("fcntl(F_GETFL): %v", errno)
	}
	return int(flags)
}

// A3 boundary: the file is opened O_CREATE|O_APPEND|O_WRONLY with no O_TRUNC;
// opening a second writer after writing must not clear earlier content.
func TestLogWriterOpenFlags(t *testing.T) {
	out := t.TempDir()
	w := newTestLogWriter(t, out, io.Discard)

	flags := openFlags(t, w)
	if got := flags & syscall.O_ACCMODE; got != syscall.O_WRONLY {
		t.Errorf("access mode = %o, want O_WRONLY (%o)", got, syscall.O_WRONLY)
	}
	if flags&syscall.O_APPEND == 0 {
		t.Errorf("O_APPEND not set (flags = %o)", flags)
	}
	if flags&syscall.O_TRUNC != 0 {
		t.Errorf("O_TRUNC set (flags = %o)", flags)
	}

	// O_CREATE and the absence of O_TRUNC are also observable through
	// behaviour: a fresh writer must create the file and a later writer must
	// leave earlier bytes intact.
	w.Close()
	if _, err := os.Stat(logFilePath(out)); err != nil {
		t.Fatalf("file not created by writer: %v", err)
	}
	w2 := newTestLogWriter(t, out, io.Discard)
	slog.New(slog.NewTextHandler(w2, nil)).Info("after reopen")
	w2.Close()

	got, err := os.ReadFile(logFilePath(out))
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(got), "after reopen") || len(got) == 0 {
		t.Errorf("earlier content not preserved across reopen\ngot:\n%s", got)
	}
}
