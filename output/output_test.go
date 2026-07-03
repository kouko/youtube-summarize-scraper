package output

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// SanitizeTitle
// ---------------------------------------------------------------------------

func TestSanitizeTitle_SpecialChars(t *testing.T) {
	got := SanitizeTitle("Hello, World! @2024 #trending", 0)
	want := "Hello_World_2024_trending"
	if got != want {
		t.Errorf("SanitizeTitle special chars: got %q, want %q", got, want)
	}
}

func TestSanitizeTitle_CJK(t *testing.T) {
	got := SanitizeTitle("日本語テスト title", 0)
	want := "日本語テスト_title"
	if got != want {
		t.Errorf("SanitizeTitle CJK: got %q, want %q", got, want)
	}
}

func TestSanitizeTitle_Long(t *testing.T) {
	long := strings.Repeat("a", 100)
	got := SanitizeTitle(long, 80)
	if len(got) != 80 {
		t.Errorf("SanitizeTitle long: got len %d, want 80", len(got))
	}
}

func TestSanitizeTitle_CustomMaxLen(t *testing.T) {
	got := SanitizeTitle("abcdefghij", 5)
	if got != "abcde" {
		t.Errorf("SanitizeTitle custom maxLen: got %q, want %q", got, "abcde")
	}
}

func TestSanitizeTitle_Empty(t *testing.T) {
	got := SanitizeTitle("", 0)
	if got != "" {
		t.Errorf("SanitizeTitle empty: got %q, want %q", got, "")
	}
}

func TestSanitizeTitle_LeadingTrailingSpaces(t *testing.T) {
	got := SanitizeTitle("  hello  ", 0)
	if got != "hello" {
		t.Errorf("SanitizeTitle leading/trailing: got %q, want %q", got, "hello")
	}
}

func TestSanitizeTitle_OnlySpecialChars(t *testing.T) {
	got := SanitizeTitle("!@#$%^&*()", 0)
	if got != "" {
		t.Errorf("SanitizeTitle only special: got %q, want %q", got, "")
	}
}

func TestSanitizeTitle_NoTrailingUnderscoreAfterTruncation(t *testing.T) {
	// "aaaa_" truncated at 5 would be "aaaa_", ensure trailing underscore removed.
	got := SanitizeTitle("aaaa bbbbb", 5)
	if strings.HasSuffix(got, "_") {
		t.Errorf("SanitizeTitle trailing underscore after truncation: got %q", got)
	}
}

// ---------------------------------------------------------------------------
// VideoDir
// ---------------------------------------------------------------------------

func TestVideoDir(t *testing.T) {
	got := VideoDir("/out", "channelXYZ", "20240315", "abc123", "My Video Title!")
	want := filepath.Join("/out", "@channelXYZ", "2024-03-15__abc123__My_Video_Title")
	if got != want {
		t.Errorf("VideoDir: got %q, want %q", got, want)
	}
}

func TestVideoDir_CJK(t *testing.T) {
	got := VideoDir("/out", "ch", "20240101", "vid1", "日本語タイトル")
	want := filepath.Join("/out", "@ch", "2024-01-01__vid1__日本語タイトル")
	if got != want {
		t.Errorf("VideoDir CJK: got %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// VideoFilePrefix
// ---------------------------------------------------------------------------

func TestVideoFilePrefix(t *testing.T) {
	got := VideoFilePrefix("20240315", "abc123")
	want := "2024-03-15__abc123__"
	if got != want {
		t.Errorf("VideoFilePrefix: got %q, want %q", got, want)
	}
}

func TestVideoFilePrefix_ShortDate(t *testing.T) {
	// Non-standard date passes through as-is.
	got := VideoFilePrefix("2024", "vid")
	want := "2024__vid__"
	if got != want {
		t.Errorf("VideoFilePrefix short date: got %q, want %q", got, want)
	}
}

func TestVideoDir_EmptyDate(t *testing.T) {
	got := VideoDir("/out", "ch", "", "vid1", "Title")
	want := filepath.Join("/out", "@ch", "unknown-date__vid1__Title")
	if got != want {
		t.Errorf("VideoDir empty date: got %q, want %q", got, want)
	}
}

func TestVideoFilePrefix_EmptyDate(t *testing.T) {
	got := VideoFilePrefix("", "vid1")
	want := "unknown-date__vid1__"
	if got != want {
		t.Errorf("VideoFilePrefix empty date: got %q, want %q", got, want)
	}
}

func TestFallbackDate(t *testing.T) {
	if got := fallbackDate(""); got != "unknown-date" {
		t.Errorf("fallbackDate empty: got %q, want %q", got, "unknown-date")
	}
	if got := fallbackDate("2024-03-15"); got != "2024-03-15" {
		t.Errorf("fallbackDate non-empty: got %q, want %q", got, "2024-03-15")
	}
}

// ---------------------------------------------------------------------------
// IsProcessed
// ---------------------------------------------------------------------------

func TestIsProcessed_Found(t *testing.T) {
	tmp := t.TempDir()
	channelDir := filepath.Join(tmp, "@testchannel")
	videoDir := filepath.Join(channelDir, "2024-03-15__abc123__some_title")
	if err := os.MkdirAll(videoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Must have summary.md to be considered fully processed.
	if err := os.WriteFile(filepath.Join(videoDir, "2024-03-15__abc123__summary.md"), []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	found, err := IsProcessed(tmp, "testchannel", "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Error("IsProcessed: expected true, got false")
	}
}

func TestIsProcessed_PartialNoSummary(t *testing.T) {
	tmp := t.TempDir()
	channelDir := filepath.Join(tmp, "@testchannel")
	videoDir := filepath.Join(channelDir, "2024-03-15__abc123__some_title")
	if err := os.MkdirAll(videoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Only transcription, no summary → not fully processed.
	if err := os.WriteFile(filepath.Join(videoDir, "2024-03-15__abc123__transcription.md"), []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	found, err := IsProcessed(tmp, "testchannel", "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("IsProcessed: expected false (no summary), got true")
	}
}

func TestIsProcessed_NotFound(t *testing.T) {
	tmp := t.TempDir()
	channelDir := filepath.Join(tmp, "@testchannel")
	if err := os.MkdirAll(channelDir, 0o755); err != nil {
		t.Fatal(err)
	}

	found, err := IsProcessed(tmp, "testchannel", "nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("IsProcessed: expected false, got true")
	}
}

func TestIsProcessed_NoChannelDir(t *testing.T) {
	tmp := t.TempDir()

	found, err := IsProcessed(tmp, "nope", "vid1")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("IsProcessed: expected false when channel dir missing, got true")
	}
}

// ---------------------------------------------------------------------------
// EnsureDir
// ---------------------------------------------------------------------------

func TestEnsureDir(t *testing.T) {
	tmp := t.TempDir()
	newDir := filepath.Join(tmp, "a", "b", "c")
	if err := EnsureDir(newDir); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(newDir)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Error("EnsureDir: expected directory")
	}
}

// ---------------------------------------------------------------------------
// Frontmatter
// ---------------------------------------------------------------------------

// parseFrontmatter strips the --- fences and unmarshals the YAML body,
// asserting the generated frontmatter is valid, parseable YAML.
func parseFrontmatter(t *testing.T, fm string) map[string]any {
	t.Helper()
	body := strings.TrimSuffix(strings.TrimPrefix(fm, "---\n"), "---\n")
	var m map[string]any
	if err := yaml.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("generated frontmatter is not valid YAML: %v\n%s", err, fm)
	}
	return m
}

// A title/tag containing " or \ must not break the YAML frontmatter: the
// values must survive a round-trip through a real YAML parser (as Obsidian
// does). Raw YouTube titles (meta.Title) reach the frontmatter unsanitized.
func TestBuildSummaryFrontmatter_EscapesSpecialChars(t *testing.T) {
	data := FrontmatterData{
		Title:      `Say "hi" \o/`, // both a double-quote and a backslash
		UploadDate: "2026-07-03",
		Tags:       []string{`quo"te`},
		Categories: []string{`back\slash`},
	}

	m := parseFrontmatter(t, BuildSummaryFrontmatter(data))

	if got, want := m["title"], `2026-07-03 Say "hi" \o/ (summary)`; got != want {
		t.Errorf("title round-trip: got %q, want %q", got, want)
	}
	tags, _ := m["tags"].([]any)
	if len(tags) != 1 || tags[0] != `quo"te` {
		t.Errorf("tags round-trip: got %#v, want [quo\"te]", m["tags"])
	}
	cats, _ := m["categories"].([]any)
	if len(cats) != 1 || cats[0] != `back\slash` {
		t.Errorf("categories round-trip: got %#v, want [back\\slash]", m["categories"])
	}
}

func TestYAMLEscape(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"plain", "plain text", "plain text"},
		{"double quote", `say "hi"`, `say \"hi\"`},
		{"backslash", `back\slash`, `back\\slash`},
		{"backslash before quote", `a\"b`, `a\\\"b`},
		{"newline", "line1\nline2", `line1\nline2`},
		{"carriage return", "a\rb", `a\rb`},
		{"tab", "a\tb", `a\tb`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := yamlEscape(c.in); got != c.want {
				t.Errorf("yamlEscape(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// updateFrontmatter rewrites playlist/playlist_id/processed_at when copying; a
// playlist name containing " must not break the resulting YAML frontmatter.
func TestUpdateFrontmatter_EscapesQuotes(t *testing.T) {
	content := "---\n" +
		`playlist: "old"` + "\n" +
		`playlist_id: "PLold"` + "\n" +
		`processed_at: "2020-01-01T00:00:00Z"` + "\n" +
		"---\n"

	got := updateFrontmatter(content, `My "Best" List`, "PL123")

	m := parseFrontmatter(t, got)
	if m["playlist"] != `My "Best" List` {
		t.Errorf("playlist round-trip: got %#v, want %q", m["playlist"], `My "Best" List`)
	}
	if m["playlist_id"] != "PL123" {
		t.Errorf("playlist_id round-trip: got %#v", m["playlist_id"])
	}
}

func TestBuildTranscriptionFrontmatter(t *testing.T) {
	data := FrontmatterData{
		Title:        "My Video",
		VideoID:      "abc123",
		URL:          "https://youtube.com/watch?v=abc123",
		Channel:      "@testchannel",
		ChannelName:  "Test Channel",
		UploadDate:   "2024-03-15",
		UploadTime:   "2024-03-15T20:00:00+08:00",
		Timezone:     "Asia/Taipei",
		Duration:     "10:30",
		Language:     "en",
		VideoTags:    []string{"go", "tutorial"},
		Categories:   []string{},
		SubtitleType: "auto",
		ProcessedAt:  "2024-03-16T12:00:00Z",
	}

	got := BuildTranscriptionFrontmatter(data)

	// Check key contents.
	checks := []string{
		`title: "2024-03-15 My Video (transcription)"`,
		`video_id: "abc123"`,
		`upload_date: "2024-03-15"`,
		`upload_time: "2024-03-15T20:00:00+08:00"`,
		`timezone: "Asia/Taipei"`,
		`video_tags:`,
		`- "go"`,
		`- "tutorial"`,
		`categories: []`,
		`subtitle_type: "auto"`,
	}
	for _, c := range checks {
		if !strings.Contains(got, c) {
			t.Errorf("transcription frontmatter missing %q\ngot:\n%s", c, got)
		}
	}

	// Should NOT contain summary-only fields.
	if strings.Contains(got, "llm_provider") {
		t.Error("transcription frontmatter should not contain llm_provider")
	}
}

func TestBuildSummaryFrontmatter(t *testing.T) {
	data := FrontmatterData{
		Title:        "My Video",
		VideoID:      "abc123",
		URL:          "https://youtube.com/watch?v=abc123",
		Channel:      "@testchannel",
		ChannelName:  "Test Channel",
		UploadDate:   "2024-03-15",
		UploadTime:   "2024-03-15T20:00:00+08:00",
		Timezone:     "Asia/Taipei",
		Duration:     "10:30",
		Language:     "en",
		VideoTags:    []string{},
		Categories:   []string{"Education"},
		SubtitleType: "manual",
		ProcessedAt:  "2024-03-16T12:00:00Z",
		Tags:         []string{"golang", "testing"},
		LLMProvider:  "anthropic",
		LLMModel:     "claude-3-opus",
	}

	got := BuildSummaryFrontmatter(data)

	checks := []string{
		`title: "2024-03-15 My Video (summary)"`,
		`video_tags: []`,
		`categories:`,
		`- "Education"`,
		`tags:`,
		`- "golang"`,
		`- "testing"`,
		`llm_provider: "anthropic"`,
		`llm_model: "claude-3-opus"`,
	}
	for _, c := range checks {
		if !strings.Contains(got, c) {
			t.Errorf("summary frontmatter missing %q\ngot:\n%s", c, got)
		}
	}
}

// ---------------------------------------------------------------------------
// IsProcessedGlobal
// ---------------------------------------------------------------------------

func TestIsProcessedGlobal_Found(t *testing.T) {
	tmp := t.TempDir()
	videoDir := filepath.Join(tmp, "@somechannel", "2024-03-15__vid1__some_title")
	if err := os.MkdirAll(videoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(videoDir, "2024-03-15__vid1__summary.md"), []byte("done"), 0o644); err != nil {
		t.Fatal(err)
	}

	found, err := IsProcessedGlobal(tmp, "vid1")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Error("IsProcessedGlobal: expected true, got false")
	}
}

func TestIsProcessedGlobal_PartialNoSummary(t *testing.T) {
	tmp := t.TempDir()
	videoDir := filepath.Join(tmp, "@somechannel", "2024-03-15__vid2__some_title")
	if err := os.MkdirAll(videoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(videoDir, "2024-03-15__vid2__transcription.md"), []byte("partial"), 0o644); err != nil {
		t.Fatal(err)
	}

	found, err := IsProcessedGlobal(tmp, "vid2")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("IsProcessedGlobal: expected false (no summary), got true")
	}
}

func TestIsProcessedGlobal_NotFound(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "@somechannel"), 0o755); err != nil {
		t.Fatal(err)
	}

	found, err := IsProcessedGlobal(tmp, "nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("IsProcessedGlobal: expected false, got true")
	}
}

// ---------------------------------------------------------------------------
// FindVideoDir
// ---------------------------------------------------------------------------

func TestFindVideoDir_Found(t *testing.T) {
	tmp := t.TempDir()
	videoDir := filepath.Join(tmp, "@ch", "2024-01-01__findme__my_title")
	if err := os.MkdirAll(videoDir, 0o755); err != nil {
		t.Fatal(err)
	}

	got := FindVideoDir(tmp, "findme")
	if got != videoDir {
		t.Errorf("FindVideoDir: got %q, want %q", got, videoDir)
	}
}

func TestFindVideoDir_NotFound(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "@ch"), 0o755); err != nil {
		t.Fatal(err)
	}

	got := FindVideoDir(tmp, "missing")
	if got != "" {
		t.Errorf("FindVideoDir: got %q, want empty string", got)
	}
}

// ---------------------------------------------------------------------------
// HasFile
// ---------------------------------------------------------------------------

func TestHasFile_Found(t *testing.T) {
	tmp := t.TempDir()
	videoDir := filepath.Join(tmp, "@ch", "2024-01-01__v1__title")
	if err := os.MkdirAll(videoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(videoDir, "2024-01-01__v1__summary.md"), []byte("s"), 0o644); err != nil {
		t.Fatal(err)
	}

	if !HasFile(videoDir, "summary.md") {
		t.Error("HasFile: expected true, got false")
	}
}

func TestHasFile_NotFound(t *testing.T) {
	tmp := t.TempDir()
	videoDir := filepath.Join(tmp, "@ch", "2024-01-01__v1__title")
	if err := os.MkdirAll(videoDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if HasFile(videoDir, "summary.md") {
		t.Error("HasFile: expected false, got true")
	}
}
