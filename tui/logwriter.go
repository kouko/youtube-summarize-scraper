package tui

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

const (
	logDirName  = "logs"
	logFileName = "ytss.log"
)

// LogWriter tees each write to the existing logger writer (in production the
// event bridge, so the TUI keeps showing the same lines) and to a plain-text
// log file at <outputDir>/logs/ytss.log. The logs directory is created on
// first use with mode 0755; the file is opened append-only
// (O_CREATE|O_APPEND|O_WRONLY) with mode 0644 (umask-dependent), so a second
// run appends to the same file and never clears earlier content.
//
// There is no fsync and no buffering: each write goes straight to the file and
// the OS page cache handles durability, so a crash loses at most the tail of
// the current run (spec Design decision).
//
// Go's slog discards the error returned by Handler.Handle, so write failures
// are detected here, in the writer itself. A failure is returned to the caller
// and never logged to the file: an on-failure record would recurse through
// this same Write (spec Design decision 31; surfacing the one ERROR event is
// the caller's job).
type LogWriter struct {
	inner io.Writer
	file  *os.File
	path  string

	mu sync.Mutex
}

// NewLogWriter opens (creating when absent) <outputDir>/logs/ytss.log for
// appending and returns a writer that tees each record to inner and the file.
// inner may be nil. An unwritable output dir returns an error naming the log
// file path and the cause (wrapping the underlying *os.PathError) instead of
// panicking.
func NewLogWriter(outputDir string, inner io.Writer) (*LogWriter, error) {
	logsDir := filepath.Join(outputDir, logDirName)
	path := filepath.Join(logsDir, logFileName)
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		// The mkdir error names only the logs directory; the caller surfaces
		// this as one Recent Events error line that must name the log file
		// path and the cause, so include the file path here.
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return &LogWriter{inner: inner, file: file, path: path}, nil
}

// Write implements io.Writer. It appends p to the log file and forwards p to
// inner, so a failed file write never stops delivery to the existing flow. It
// never writes a failure through itself.
func (w *LogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	_, fileErr := w.file.Write(p)
	var innerErr error
	if w.inner != nil {
		_, innerErr = w.inner.Write(p)
	}
	if fileErr != nil {
		return 0, fileErr
	}
	if innerErr != nil {
		return 0, innerErr
	}
	return len(p), nil
}

// Path returns the log file path this writer appends to.
func (w *LogWriter) Path() string { return w.path }

// Close closes the log file. Writes still forwarded to inner are unaffected.
func (w *LogWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.file.Close()
}
