package transcriber

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kouko/youtube-summarize-scraper/config"
)

// Subprocess output must route through DefaultStderr (package-level, default
// os.Stderr) so an embedding UI can silence yt-dlp/whisper raw progress lines;
// it must never be hardcoded to os.Stderr.
func TestTranscriberRoutesSubprocessOutputThroughStderr(t *testing.T) {
	if DefaultStderr != os.Stderr {
		t.Fatalf("DefaultStderr must default to os.Stderr (CLI behavior unchanged), got %T", DefaultStderr)
	}

	var buf bytes.Buffer
	saved := DefaultStderr
	DefaultStderr = &buf
	defer func() { DefaultStderr = saved }()

	tr := NewTranscriber("/bin/echo", "/bin/echo", "/bin/echo", config.WhisperConfig{
		DownloadTimeout:   1,
		TranscribeTimeout: 1,
	})
	tmp := t.TempDir()
	// downloadAudio and runWhisper both exec their "tool" paths, which here
	// are /bin/echo: it writes its args to stdout/stderr and exits 0.
	_ = tr.downloadAudio("url", filepath.Join(tmp, "a.wav"), nil)
	_ = tr.runWhisper("model", "audio", filepath.Join(tmp, "w"), "ja")

	if !strings.Contains(buf.String(), "url") {
		t.Errorf("yt-dlp stdout did not land in DefaultStderr; got %q", buf.String())
	}
	if !strings.Contains(buf.String(), "model") {
		t.Errorf("whisper stdout did not land in DefaultStderr; got %q", buf.String())
	}
}
