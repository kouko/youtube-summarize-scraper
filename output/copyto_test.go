package output

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kouko/youtube-summarize-scraper/config"
)

func testVars() CopyToVars {
	return CopyToVars{
		UploadDate:    "2024-01-15",
		VideoID:       "abc123",
		Title:         "My Test Video!",
		ChannelName:   "Test Channel",
		ChannelHandle: "testchannel",
		PlaylistName:  "My Playlist",
		PlaylistID:    "PLxyz",
	}
}

func TestResolveTemplate(t *testing.T) {
	vars := testVars()

	tests := []struct {
		name     string
		template string
		fileType string
		want     string
	}{
		{
			name:     "path with channel and date",
			template: "/vault/{channel_name}/{upload_date}__{title}",
			fileType: "summary",
			want:     "/vault/Test_Channel/2024-01-15__My_Test_Video",
		},
		{
			name:     "filename with type",
			template: "{upload_date}_{title}_{type}.md",
			fileType: "summary",
			want:     "2024-01-15_My_Test_Video_summary.md",
		},
		{
			name:     "all variables",
			template: "{upload_date}/{video_id}/{channel_handle}/{playlist_name}/{playlist_id}/{type}",
			fileType: "transcription",
			want:     "2024-01-15/abc123/testchannel/My_Playlist/PLxyz/transcription",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveTemplate(tt.template, vars, tt.fileType)
			if got != tt.want {
				t.Errorf("resolveTemplate() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExecuteCopyTo_BasicCopy(t *testing.T) {
	// Create source directory with files.
	srcDir := t.TempDir()
	prefix := "2024-01-15__abc123__"
	summaryContent := "# Summary\nHello world"
	transcriptionContent := "# Transcription\nFull text"

	os.WriteFile(filepath.Join(srcDir, prefix+"summary.md"), []byte(summaryContent), 0o644)
	os.WriteFile(filepath.Join(srcDir, prefix+"transcription.md"), []byte(transcriptionContent), 0o644)
	os.WriteFile(filepath.Join(srcDir, prefix+"subtitle.srt"), []byte("1\n00:00:00,000 --> 00:00:01,000\nHello"), 0o644)

	// Target directory.
	dstBase := t.TempDir()
	targetPath := filepath.Join(dstBase, "target")

	cfg := config.CopyToConfig{
		Path:  targetPath,
		Files: []string{"summary", "transcription"},
	}

	err := ExecuteCopyTo(cfg, srcDir, prefix, testVars())
	if err != nil {
		t.Fatalf("ExecuteCopyTo() error = %v", err)
	}

	// Verify files were copied.
	for _, name := range []string{prefix + "summary.md", prefix + "transcription.md"} {
		dst := filepath.Join(targetPath, name)
		if _, err := os.Stat(dst); os.IsNotExist(err) {
			t.Errorf("expected file %s to exist", dst)
		}
	}

	// Verify subtitle was NOT copied.
	subtitleDst := filepath.Join(targetPath, prefix+"subtitle.srt")
	if _, err := os.Stat(subtitleDst); !os.IsNotExist(err) {
		t.Errorf("subtitle should not have been copied")
	}

	// Verify content.
	data, _ := os.ReadFile(filepath.Join(targetPath, prefix+"summary.md"))
	if string(data) != summaryContent {
		t.Errorf("copied summary content = %q, want %q", string(data), summaryContent)
	}
}

func TestExecuteCopyTo_OverwriteFalse(t *testing.T) {
	srcDir := t.TempDir()
	prefix := "2024-01-15__abc123__"
	os.WriteFile(filepath.Join(srcDir, prefix+"summary.md"), []byte("new content"), 0o644)

	dstBase := t.TempDir()
	targetPath := filepath.Join(dstBase, "target")
	os.MkdirAll(targetPath, 0o755)

	// Pre-create existing file with old content.
	existingPath := filepath.Join(targetPath, prefix+"summary.md")
	os.WriteFile(existingPath, []byte("old content"), 0o644)

	cfg := config.CopyToConfig{
		Path:      targetPath,
		Files:     []string{"summary"},
		Overwrite: false,
	}

	err := ExecuteCopyTo(cfg, srcDir, prefix, testVars())
	if err != nil {
		t.Fatalf("ExecuteCopyTo() error = %v", err)
	}

	// Verify file was NOT overwritten.
	data, _ := os.ReadFile(existingPath)
	if string(data) != "old content" {
		t.Errorf("file should not be overwritten, got %q", string(data))
	}
}

func TestExecuteCopyTo_OverwriteTrue(t *testing.T) {
	srcDir := t.TempDir()
	prefix := "2024-01-15__abc123__"
	os.WriteFile(filepath.Join(srcDir, prefix+"summary.md"), []byte("new content"), 0o644)

	dstBase := t.TempDir()
	targetPath := filepath.Join(dstBase, "target")
	os.MkdirAll(targetPath, 0o755)

	// Pre-create existing file with old content.
	existingPath := filepath.Join(targetPath, prefix+"summary.md")
	os.WriteFile(existingPath, []byte("old content"), 0o644)

	cfg := config.CopyToConfig{
		Path:      targetPath,
		Files:     []string{"summary"},
		Overwrite: true,
	}

	err := ExecuteCopyTo(cfg, srcDir, prefix, testVars())
	if err != nil {
		t.Fatalf("ExecuteCopyTo() error = %v", err)
	}

	// Verify file WAS overwritten.
	data, _ := os.ReadFile(existingPath)
	if string(data) != "new content" {
		t.Errorf("file should be overwritten, got %q, want %q", string(data), "new content")
	}
}

func TestExecuteCopyTo_DefaultFiles(t *testing.T) {
	srcDir := t.TempDir()
	prefix := "2024-01-15__abc123__"
	os.WriteFile(filepath.Join(srcDir, prefix+"summary.md"), []byte("summary"), 0o644)
	os.WriteFile(filepath.Join(srcDir, prefix+"transcription.md"), []byte("transcription"), 0o644)

	dstBase := t.TempDir()
	targetPath := filepath.Join(dstBase, "target")

	// Empty files list should default to ["summary"].
	cfg := config.CopyToConfig{
		Path:  targetPath,
		Files: nil,
	}

	err := ExecuteCopyTo(cfg, srcDir, prefix, testVars())
	if err != nil {
		t.Fatalf("ExecuteCopyTo() error = %v", err)
	}

	// Summary should be copied.
	if _, err := os.Stat(filepath.Join(targetPath, prefix+"summary.md")); os.IsNotExist(err) {
		t.Error("summary should have been copied with default files")
	}

	// Transcription should NOT be copied.
	if _, err := os.Stat(filepath.Join(targetPath, prefix+"transcription.md")); !os.IsNotExist(err) {
		t.Error("transcription should not have been copied with default files")
	}
}

func TestExecuteCopyTo_CustomFilename(t *testing.T) {
	srcDir := t.TempDir()
	prefix := "2024-01-15__abc123__"
	os.WriteFile(filepath.Join(srcDir, prefix+"summary.md"), []byte("summary"), 0o644)

	dstBase := t.TempDir()
	targetPath := filepath.Join(dstBase, "target")

	cfg := config.CopyToConfig{
		Path:     targetPath,
		Files:    []string{"summary"},
		Filename: "{upload_date}_{title}_{type}.md",
	}

	err := ExecuteCopyTo(cfg, srcDir, prefix, testVars())
	if err != nil {
		t.Fatalf("ExecuteCopyTo() error = %v", err)
	}

	expectedName := "2024-01-15_My_Test_Video_summary.md"
	expectedPath := filepath.Join(targetPath, expectedName)
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("expected file %s to exist", expectedPath)
	}
}
