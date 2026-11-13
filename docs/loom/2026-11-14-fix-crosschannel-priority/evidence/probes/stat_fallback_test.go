package probes

// concern: duplicate-candidate stat-failure fallback — when the highest-scoring
// candidate directory for a video ID has been deleted on disk, FindVideoDir
// must fall back to the next-best surviving candidate instead of dropping the
// video from the index, and the stale file flags of the deleted candidate must
// not survive the prune. Guards the candidate-slice removal against losing the
// video, resurrecting a dead candidate's flags, or flipping IsProcessed for a
// channel whose only directory is gone.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kouko/youtube-summarize-scraper/output"
)

func mkDirP1(t *testing.T, tmp, channel, dirName string, files []string) string {
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

// TestFindVideoDir_DeletedBest_FallsBack: with the best (summary) candidate
// deleted, FindVideoDir must serve the surviving .skipped candidate and the
// video must not be dropped.
func TestFindVideoDir_DeletedBest_FallsBack(t *testing.T) {
	tmp := t.TempDir()
	best := mkDirP1(t, tmp, "@aaa", "2024-01-01__dup__A", []string{"2024-01-01__dup__summary.md"})
	next := mkDirP1(t, tmp, "@bbb", "2024-02-01__dup__B", []string{"2024-02-01__dup__.skipped"})
	idx := output.BuildIndex(tmp)
	if err := os.RemoveAll(best); err != nil {
		t.Fatal(err)
	}
	if got := idx.FindVideoDir("dup"); got != next {
		t.Errorf("FindVideoDir after deletion: got %q want %q", got, next)
	}
	if again := idx.FindVideoDir("dup"); again != next {
		t.Errorf("second FindVideoDir: got %q want %q", again, next)
	}
}

// TestFindVideoDir_DeletedBest_PrunesStaleFlags: after the fallback the
// deleted candidate's summary flag must be gone and the surviving candidate's
// flags and per-channel processed state must be reported.
func TestFindVideoDir_DeletedBest_PrunesStaleFlags(t *testing.T) {
	tmp := t.TempDir()
	best := mkDirP1(t, tmp, "@aaa", "2024-01-01__dup__A", []string{"2024-01-01__dup__summary.md"})
	mkDirP1(t, tmp, "@bbb", "2024-02-01__dup__B", []string{"2024-02-01__dup__.skipped"})
	idx := output.BuildIndex(tmp)
	if err := os.RemoveAll(best); err != nil {
		t.Fatal(err)
	}
	if got := idx.FindVideoDir("dup"); got == "" {
		t.Fatal("video must not be dropped while a candidate exists")
	}
	if idx.HasFile("dup", "summary.md") {
		t.Error("deleted candidate's summary flag must be pruned")
	}
	if !idx.HasFile("dup", ".skipped") {
		t.Error("surviving candidate's .skipped flag must be reported")
	}
	if idx.IsProcessed("aaa", "dup") {
		t.Error("channel whose only directory was deleted must not stay processed")
	}
	if !idx.IsProcessed("bbb", "dup") {
		t.Error("channel with the surviving candidate must stay processed")
	}
}

// TestFindVideoDir_DeletedTwoBest_ReachesSurvivor: deleting the two
// highest-scoring candidates must reach the empty third candidate rather than
// dropping the video.
func TestFindVideoDir_DeletedTwoBest_ReachesSurvivor(t *testing.T) {
	tmp := t.TempDir()
	d1 := mkDirP1(t, tmp, "@aaa", "2024-01-01__dup__A", []string{"2024-01-01__dup__summary.md"})
	d2 := mkDirP1(t, tmp, "@bbb", "2024-02-01__dup__B", []string{"2024-02-01__dup__transcription.md"})
	d3 := mkDirP1(t, tmp, "@ccc", "2024-03-01__dup__C", nil)
	idx := output.BuildIndex(tmp)
	if err := os.RemoveAll(d1); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(d2); err != nil {
		t.Fatal(err)
	}
	if got := idx.FindVideoDir("dup"); got != d3 {
		t.Errorf("FindVideoDir: got %q want empty survivor %q", got, d3)
	}
	if idx.HasFile("dup", "transcription.md") {
		t.Error("deleted candidates' flags must not leak to the survivor")
	}
}

// TestFindVideoDir_AllDeleted_VideoDropped: when every candidate directory is
// gone the video is dropped and no flag or processed state survives.
func TestFindVideoDir_AllDeleted_VideoDropped(t *testing.T) {
	tmp := t.TempDir()
	d1 := mkDirP1(t, tmp, "@aaa", "2024-01-01__dup__A", nil)
	d2 := mkDirP1(t, tmp, "@bbb", "2024-02-01__dup__B", []string{"2024-02-01__dup__summary.md"})
	idx := output.BuildIndex(tmp)
	if err := os.RemoveAll(d1); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(d2); err != nil {
		t.Fatal(err)
	}
	if got := idx.FindVideoDir("dup"); got != "" {
		t.Errorf("FindVideoDir: got %q want empty", got)
	}
	if idx.HasFile("dup", "summary.md") {
		t.Error("HasFile must be false once every candidate is gone")
	}
	if idx.IsProcessed("bbb", "dup") {
		t.Error("IsProcessed must be false once every candidate is gone")
	}
}
