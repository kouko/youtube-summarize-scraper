package output

import (
	"os"
	"path/filepath"
	"testing"
)

// helper to create a video directory with optional files.
func createVideoDir(t *testing.T, base, channel, dirName string, files []string) string {
	t.Helper()
	videoDir := filepath.Join(base, channel, dirName)
	if err := os.MkdirAll(videoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(videoDir, f), []byte("test"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return videoDir
}

// ---------------------------------------------------------------------------
// BuildIndex
// ---------------------------------------------------------------------------

func TestBuildIndex_MultiChannel(t *testing.T) {
	tmp := t.TempDir()
	createVideoDir(t, tmp, "@channelA", "2024-03-15__vid1__Title_A", []string{
		"2024-03-15__vid1__summary.md",
		"2024-03-15__vid1__transcription.md",
	})
	createVideoDir(t, tmp, "@channelB", "2024-03-16__vid2__Title_B", []string{
		"2024-03-16__vid2__subtitle.srt",
	})

	idx := BuildIndex(tmp)

	if dir := idx.FindVideoDir("vid1"); dir == "" {
		t.Error("BuildIndex: vid1 not found")
	}
	if dir := idx.FindVideoDir("vid2"); dir == "" {
		t.Error("BuildIndex: vid2 not found")
	}
}

func TestBuildIndex_PlaylistDir(t *testing.T) {
	tmp := t.TempDir()
	createVideoDir(t, tmp, "_playlist__WL__Watch_Later", "2024-03-10__vid3__Nested", []string{
		"2024-03-10__vid3__summary.md",
	})

	idx := BuildIndex(tmp)

	if dir := idx.FindVideoDir("vid3"); dir == "" {
		t.Error("BuildIndex: vid3 in playlist not found")
	}
	if !idx.HasFile("vid3", "summary.md") {
		t.Error("BuildIndex: vid3 should have summary.md")
	}
}

func TestBuildIndex_EmptyDir(t *testing.T) {
	tmp := t.TempDir()
	idx := BuildIndex(tmp)

	if dir := idx.FindVideoDir("anything"); dir != "" {
		t.Errorf("BuildIndex empty: got %q, want empty", dir)
	}
}

func TestBuildIndex_SkipsFiles(t *testing.T) {
	tmp := t.TempDir()
	// Create a file (not directory) at top level — should be skipped.
	if err := os.WriteFile(filepath.Join(tmp, "stray_file.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Create a valid video dir.
	createVideoDir(t, tmp, "@ch", "2024-01-01__vid1__title", []string{
		"2024-01-01__vid1__summary.md",
	})

	idx := BuildIndex(tmp)

	if dir := idx.FindVideoDir("vid1"); dir == "" {
		t.Error("BuildIndex: vid1 not found after skipping stray file")
	}
}

func TestBuildIndex_InvalidDirName(t *testing.T) {
	tmp := t.TempDir()
	// Directory without __ separator — should be skipped.
	badDir := filepath.Join(tmp, "@ch", "no-separator-dir")
	if err := os.MkdirAll(badDir, 0o755); err != nil {
		t.Fatal(err)
	}
	createVideoDir(t, tmp, "@ch", "2024-01-01__vid1__title", []string{})

	idx := BuildIndex(tmp)

	if dir := idx.FindVideoDir("vid1"); dir == "" {
		t.Error("BuildIndex: vid1 not found")
	}
}

// ---------------------------------------------------------------------------
// FindVideoDir
// ---------------------------------------------------------------------------

func TestFindVideoDir_Index_Found(t *testing.T) {
	tmp := t.TempDir()
	expected := createVideoDir(t, tmp, "@ch", "2024-01-01__findme__title", []string{})

	idx := BuildIndex(tmp)

	got := idx.FindVideoDir("findme")
	if got != expected {
		t.Errorf("FindVideoDir: got %q, want %q", got, expected)
	}
}

func TestFindVideoDir_Index_NotFound(t *testing.T) {
	tmp := t.TempDir()
	createVideoDir(t, tmp, "@ch", "2024-01-01__vid1__title", []string{})

	idx := BuildIndex(tmp)

	if got := idx.FindVideoDir("missing"); got != "" {
		t.Errorf("FindVideoDir: got %q, want empty", got)
	}
}

func TestFindVideoDir_Index_DeletedDir(t *testing.T) {
	tmp := t.TempDir()
	dir := createVideoDir(t, tmp, "@ch", "2024-01-01__vid1__title", []string{})

	idx := BuildIndex(tmp)

	// Delete the directory after building index.
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}

	// Should return "" because os.Stat fails, and entry should be cleaned up.
	if got := idx.FindVideoDir("vid1"); got != "" {
		t.Errorf("FindVideoDir deleted dir: got %q, want empty", got)
	}

	// Second call should also return "" (entry removed).
	if got := idx.FindVideoDir("vid1"); got != "" {
		t.Errorf("FindVideoDir deleted dir (2nd call): got %q, want empty", got)
	}
}

// ---------------------------------------------------------------------------
// HasFile
// ---------------------------------------------------------------------------

func TestHasFile_Index_Found(t *testing.T) {
	tmp := t.TempDir()
	createVideoDir(t, tmp, "@ch", "2024-01-01__v1__title", []string{
		"2024-01-01__v1__summary.md",
		"2024-01-01__v1__transcription.md",
	})

	idx := BuildIndex(tmp)

	if !idx.HasFile("v1", "summary.md") {
		t.Error("HasFile: expected true for summary.md")
	}
	if !idx.HasFile("v1", "transcription.md") {
		t.Error("HasFile: expected true for transcription.md")
	}
}

func TestHasFile_Index_NotFound(t *testing.T) {
	tmp := t.TempDir()
	createVideoDir(t, tmp, "@ch", "2024-01-01__v1__title", []string{
		"2024-01-01__v1__transcription.md",
	})

	idx := BuildIndex(tmp)

	if idx.HasFile("v1", "summary.md") {
		t.Error("HasFile: expected false for missing summary.md")
	}
}

func TestHasFile_Index_VideoNotInIndex(t *testing.T) {
	tmp := t.TempDir()
	idx := BuildIndex(tmp)

	if idx.HasFile("nonexistent", "summary.md") {
		t.Error("HasFile: expected false for unknown video")
	}
}

// ---------------------------------------------------------------------------
// IsProcessed
// ---------------------------------------------------------------------------

func TestIsProcessed_Index_Found(t *testing.T) {
	tmp := t.TempDir()
	createVideoDir(t, tmp, "@testchannel", "2024-03-15__abc123__some_title", []string{
		"2024-03-15__abc123__summary.md",
	})

	idx := BuildIndex(tmp)

	if !idx.IsProcessed("testchannel", "abc123") {
		t.Error("IsProcessed: expected true")
	}
}

func TestIsProcessed_Index_WrongChannel(t *testing.T) {
	tmp := t.TempDir()
	createVideoDir(t, tmp, "@channelA", "2024-03-15__abc123__some_title", []string{
		"2024-03-15__abc123__summary.md",
	})

	idx := BuildIndex(tmp)

	// Video exists but in channelA, not channelB.
	if idx.IsProcessed("channelB", "abc123") {
		t.Error("IsProcessed: expected false for wrong channel")
	}
}

func TestIsProcessed_Index_NoSummary(t *testing.T) {
	tmp := t.TempDir()
	createVideoDir(t, tmp, "@testchannel", "2024-03-15__abc123__some_title", []string{
		"2024-03-15__abc123__transcription.md",
	})

	idx := BuildIndex(tmp)

	if idx.IsProcessed("testchannel", "abc123") {
		t.Error("IsProcessed: expected false when no summary.md")
	}
}

// ---------------------------------------------------------------------------
// Add / AddFile
// ---------------------------------------------------------------------------

func TestAdd_NewVideo(t *testing.T) {
	tmp := t.TempDir()
	idx := BuildIndex(tmp)

	// Add is called after EnsureDir, so the directory exists on disk.
	dir := filepath.Join(tmp, "@ch", "2024-01-01__newvid__title")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	idx.Add("newvid", dir)

	if got := idx.FindVideoDir("newvid"); got != dir {
		t.Errorf("Add: got %q, want %q", got, dir)
	}
}

func TestAddFile_NewFile(t *testing.T) {
	tmp := t.TempDir()
	createVideoDir(t, tmp, "@ch", "2024-01-01__v1__title", []string{})

	idx := BuildIndex(tmp)

	if idx.HasFile("v1", "summary.md") {
		t.Error("AddFile before: expected false")
	}

	idx.AddFile("v1", "summary.md")

	if !idx.HasFile("v1", "summary.md") {
		t.Error("AddFile after: expected true")
	}
}

func TestAddFile_VideoNotInIndex(t *testing.T) {
	tmp := t.TempDir()
	idx := BuildIndex(tmp)

	// Should not panic on unknown video.
	idx.AddFile("unknown", "summary.md")

	if idx.HasFile("unknown", "summary.md") {
		t.Error("AddFile unknown: expected false (no entry to update)")
	}
}

// ---------------------------------------------------------------------------
// StatCheck before processing (3rd layer defense)
// ---------------------------------------------------------------------------

func TestStatCheck_FileAppearedExternally(t *testing.T) {
	tmp := t.TempDir()
	videoDir := createVideoDir(t, tmp, "@ch", "2024-01-01__v1__title", []string{})

	idx := BuildIndex(tmp)

	// Index says no summary.md.
	if idx.HasFile("v1", "summary.md") {
		t.Error("StatCheck: expected false before external add")
	}

	// Externally add summary.md (simulating another process).
	if err := os.WriteFile(filepath.Join(videoDir, "2024-01-01__v1__summary.md"), []byte("ext"), 0o644); err != nil {
		t.Fatal(err)
	}

	// StatCheck should detect the externally added file.
	if !idx.StatCheck("v1", "summary.md") {
		t.Error("StatCheck: expected true after external file addition")
	}
}

func TestStatCheck_VideoNotInIndex(t *testing.T) {
	tmp := t.TempDir()
	idx := BuildIndex(tmp)

	if idx.StatCheck("nonexistent", "summary.md") {
		t.Error("StatCheck: expected false for unknown video")
	}
}
