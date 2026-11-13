package probes

// concern: cross-channel duplicate priority — a video ID that contains "__"
// and exists in two channel directories with a completed summary must resolve
// as processed for both channels, and a playlist duplicate carrying only a
// stale .skipped marker must never shadow the channel's completed summary.md
// regardless of directory sort order. Guards the per-channel candidate scan and
// the summary-over-.skipped rank against a single global winner reappearing.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kouko/youtube-summarize-scraper/output"
)

func mkDirP2(t *testing.T, tmp, channel, dirName string, files []string) string {
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

// TestIsProcessed_DoubleUnderscoreBothChannels_True: the same __ video ID in
// two channels, each with a summary, must be processed in both channels.
func TestIsProcessed_DoubleUnderscoreBothChannels_True(t *testing.T) {
	tmp := t.TempDir()
	mkDirP2(t, tmp, "@aaa", "2023-12-05__tp_F__iRC04__A", []string{"2023-12-05__tp_F__iRC04__summary.md"})
	mkDirP2(t, tmp, "@bbb", "2023-12-06__tp_F__iRC04__B", []string{"2023-12-06__tp_F__iRC04__summary.md"})
	idx := output.BuildIndex(tmp)
	if !idx.IsProcessed("aaa", "tp_F__iRC04") {
		t.Error("IsProcessed(@aaa): expected true")
	}
	if !idx.IsProcessed("bbb", "tp_F__iRC04") {
		t.Error("IsProcessed(@bbb): expected true")
	}
	if !idx.HasFile("tp_F__iRC04", "summary.md") {
		t.Error("HasFile summary.md: expected true")
	}
}

// TestHasFile_PlaylistSkippedDuplicate_SummaryWins: a playlist .skipped
// duplicate that sorts first must not hide the channel's completed summary.
func TestHasFile_PlaylistSkippedDuplicate_SummaryWins(t *testing.T) {
	tmp := t.TempDir()
	mkDirP2(t, tmp, "_playlist__PL__List", "2024-01-01__dup__P", []string{"2024-01-01__dup__.skipped"})
	summaryDir := mkDirP2(t, tmp, "@ch", "2024-02-01__dup__C", []string{"2024-02-01__dup__summary.md"})
	idx := output.BuildIndex(tmp)
	if !idx.HasFile("dup", "summary.md") {
		t.Error("HasFile summary.md: expected the channel summary to win")
	}
	if !idx.IsProcessed("ch", "dup") {
		t.Error("IsProcessed(@ch): expected true")
	}
	if got := idx.FindVideoDir("dup"); got != summaryDir {
		t.Errorf("FindVideoDir: got %q want summary dir %q", got, summaryDir)
	}
}
