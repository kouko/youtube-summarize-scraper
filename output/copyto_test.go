package output

import (
	"os"
	"path/filepath"
	"strings"
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

func TestNormalizeCopyToVars(t *testing.T) {
	tests := []struct {
		name       string
		input      CopyToVars
		wantDate   string
		wantTitle  string
	}{
		{
			name: "all fields present",
			input: CopyToVars{
				UploadDate: "2024-01-15",
				VideoID:    "abc123",
				Title:      "My Video",
			},
			wantDate:  "2024-01-15",
			wantTitle: "My Video",
		},
		{
			name: "empty upload_date falls back to unknown-date",
			input: CopyToVars{
				UploadDate: "",
				VideoID:    "abc123",
				Title:      "My Video",
			},
			wantDate:  "unknown-date",
			wantTitle: "My Video",
		},
		{
			name: "empty title falls back to video_id",
			input: CopyToVars{
				UploadDate: "2024-01-15",
				VideoID:    "abc123",
				Title:      "",
			},
			wantDate:  "2024-01-15",
			wantTitle: "abc123",
		},
		{
			name: "both empty",
			input: CopyToVars{
				UploadDate: "",
				VideoID:    "abc123",
				Title:      "",
			},
			wantDate:  "unknown-date",
			wantTitle: "abc123",
		},
		{
			name: "empty title and empty video_id",
			input: CopyToVars{
				UploadDate: "2024-01-15",
				VideoID:    "",
				Title:      "",
			},
			wantDate:  "2024-01-15",
			wantTitle: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeCopyToVars(tt.input)
			if got.UploadDate != tt.wantDate {
				t.Errorf("UploadDate = %q, want %q", got.UploadDate, tt.wantDate)
			}
			if got.Title != tt.wantTitle {
				t.Errorf("Title = %q, want %q", got.Title, tt.wantTitle)
			}
		})
	}
}

func TestExecuteCopyTo_EmptyMetadata(t *testing.T) {
	srcDir := t.TempDir()
	prefix := "__abc123__"
	os.WriteFile(filepath.Join(srcDir, prefix+"summary.md"), []byte("summary"), 0o644)

	dstBase := t.TempDir()
	targetPath := filepath.Join(dstBase, "target")

	cfg := config.CopyToConfig{
		Path:     targetPath,
		Files:    []string{"summary"},
		Filename: "{upload_date}_{title}_{type}.md",
	}

	// Empty UploadDate and Title — normalization should produce valid filename.
	vars := CopyToVars{
		UploadDate: "",
		VideoID:    "abc123",
		Title:      "",
	}

	err := ExecuteCopyTo(cfg, srcDir, prefix, vars)
	if err != nil {
		t.Fatalf("ExecuteCopyTo() error = %v", err)
	}

	expectedName := "unknown-date_abc123_summary.md"
	expectedPath := filepath.Join(targetPath, expectedName)
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("expected file %s to exist", expectedPath)
	}
}

func TestResolveTemplate_LongTitle(t *testing.T) {
	// Title that would produce a filename > 255 bytes.
	longTitle := strings.Repeat("あ", 200) // 200 CJK chars = 600 bytes
	vars := CopyToVars{
		UploadDate: "2024-01-15",
		VideoID:    "abc123",
		Title:      longTitle,
	}
	result := resolveTemplate("{upload_date}_{title}_{type}.md", vars, "summary")

	if len(result) > maxSegmentBytes {
		t.Errorf("result length %d bytes exceeds %d", len(result), maxSegmentBytes)
	}
	// Must preserve extension.
	if !strings.HasSuffix(result, ".md") {
		t.Errorf("result %q should end with .md", result)
	}
	// Must still start with the date prefix.
	if !strings.HasPrefix(result, "2024-01-15_") {
		t.Errorf("result %q should start with date prefix", result)
	}
}

func TestResolveTemplate_LongMultipleFields(t *testing.T) {
	// All three shrinkable fields are long.
	vars := CopyToVars{
		UploadDate:    "2024-01-15",
		VideoID:       "abc123",
		Title:         strings.Repeat("T", 80),
		ChannelName:   strings.Repeat("C", 80),
		ChannelHandle: "ch",
		PlaylistName:  strings.Repeat("P", 80),
		PlaylistID:    "PLxyz",
	}
	result := resolveTemplate("{channel_name}_{playlist_name}_{title}_{type}.md", vars, "summary")

	if len(result) > maxSegmentBytes {
		t.Errorf("result length %d bytes exceeds %d: %q", len(result), maxSegmentBytes, result)
	}
	if !strings.HasSuffix(result, ".md") {
		t.Errorf("result %q should end with .md", result)
	}
}

func TestResolveTemplate_FallbackTruncate(t *testing.T) {
	// Even after progressive shortening, segment is still too long.
	// Use a very long video_id (not shrinkable) plus long title.
	vars := CopyToVars{
		UploadDate: "2024-01-15",
		VideoID:    strings.Repeat("V", 200),
		Title:      strings.Repeat("T", 80),
	}
	result := resolveTemplate("{video_id}_{title}_{type}.md", vars, "summary")

	if len(result) > maxSegmentBytes {
		t.Errorf("result length %d bytes exceeds %d", len(result), maxSegmentBytes)
	}
	if !strings.HasSuffix(result, ".md") {
		t.Errorf("result %q should end with .md", result)
	}
}

func TestResolveTemplate_NormalNotAffected(t *testing.T) {
	vars := testVars()
	// Same as existing test — ensure normal-length results are unchanged.
	result := resolveTemplate("{upload_date}_{title}_{type}.md", vars, "summary")
	want := "2024-01-15_My_Test_Video_summary.md"
	if result != want {
		t.Errorf("resolveTemplate() = %q, want %q", result, want)
	}
}
