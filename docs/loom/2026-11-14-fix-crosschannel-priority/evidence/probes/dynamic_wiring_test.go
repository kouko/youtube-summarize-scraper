package probes

// concern: dynamic Add/AddFile candidate routing — AddFile carries no
// directory argument, so a mark must land on the working directory registered
// by the latest Add (or on the highest-scoring candidate when no Add preceded
// it), never on a higher-scoring stale duplicate in another channel. Guards the
// resume path and the Whisper duration gate against a .skipped or summary mark
// being written onto the wrong channel's candidate.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kouko/youtube-summarize-scraper/output"
)

func mkDirP3(t *testing.T, tmp, channel, dirName string, files []string) string {
	t.Helper()
	dir := filepath.Join(tmp, channel, dirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// TestAddFile_NoPriorAdd_MarksRepresentative: an AddFile for a video known
// only to BuildIndex must mark the highest-scoring candidate.
func TestAddFile_NoPriorAdd_MarksRepresentative(t *testing.T) {
	tmp := t.TempDir()
	mkDirP3(t, tmp, "@aaa", "2024-01-01__dup__A", nil)
	best := mkDirP3(t, tmp, "@bbb", "2024-02-01__dup__B", []string{"2024-02-01__dup__transcription.md"})
	idx := output.BuildIndex(tmp)
	idx.AddFile("dup", "summary.md")
	if !idx.HasFile("dup", "summary.md") {
		t.Error("AddFile without Add must mark the representative candidate")
	}
	if got := idx.FindVideoDir("dup"); got != best {
		t.Errorf("FindVideoDir: got %q want representative %q", got, best)
	}
}

// TestAddFile_SkipMarkOnWorkingDir_NotBestCandidate: after Add registers a new
// working directory, a .skipped mark must land there even though an older
// summary candidate still outranks it; both channels must stay processed and
// the summary candidate must stay the global representative.
func TestAddFile_SkipMarkOnWorkingDir_NotBestCandidate(t *testing.T) {
	tmp := t.TempDir()
	summaryDir := mkDirP3(t, tmp, "@aaa", "2024-01-01__dup__A", []string{"2024-01-01__dup__summary.md"})
	idx := output.BuildIndex(tmp)
	workDir := filepath.Join(tmp, "@bbb", "2024-02-01__dup__B")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	idx.Add("dup", workDir)
	idx.AddFile("dup", ".skipped")
	if !idx.IsProcessed("bbb", "dup") {
		t.Error("the .skipped mark must land on the working directory")
	}
	if !idx.IsProcessed("aaa", "dup") {
		t.Error("the existing summary channel must stay processed")
	}
	if got := idx.FindVideoDir("dup"); got != summaryDir {
		t.Errorf("FindVideoDir: got %q want summary candidate %q", got, summaryDir)
	}
}
