package output

import (
	"testing"
)

func TestSanitizeTag_Spaces(t *testing.T) {
	got := SanitizeTag("Federal Reserve")
	if got != "federal-reserve" {
		t.Errorf("got %q, want %q", got, "federal-reserve")
	}
}

func TestSanitizeTag_SpecialChars(t *testing.T) {
	got := SanitizeTag("AI/ML & Data Science!")
	if got != "aiml-data-science" {
		t.Errorf("got %q, want %q", got, "aiml-data-science")
	}
}

func TestSanitizeTag_CJK(t *testing.T) {
	got := SanitizeTag("量化寬鬆")
	if got != "量化寬鬆" {
		t.Errorf("got %q, want %q", got, "量化寬鬆")
	}
}

func TestSanitizeTag_AtPrefix(t *testing.T) {
	got := SanitizeTag("@ChannelName")
	if got != "channelname" {
		t.Errorf("got %q, want %q", got, "channelname")
	}
}

func TestSanitizeTag_Empty(t *testing.T) {
	got := SanitizeTag("  ")
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestSanitizeTag_Hyphens(t *testing.T) {
	got := SanitizeTag("some--tag---here")
	if got != "some-tag-here" {
		t.Errorf("got %q, want %q", got, "some-tag-here")
	}
}

func TestEnrichTagsForObsidian_Sanitized(t *testing.T) {
	tags := EnrichTagsForObsidian(
		[]string{"Federal Reserve", "量化寬鬆", "AI/ML"},
		"Test Channel",
		[]string{"youtube"},
	)

	expected := []string{"federal-reserve", "量化寬鬆", "aiml", "test-channel", "youtube"}
	if len(tags) != len(expected) {
		t.Errorf("got %d tags, want %d: %v", len(tags), len(expected), tags)
		return
	}
	for i, tag := range tags {
		if tag != expected[i] {
			t.Errorf("tags[%d]: got %q, want %q", i, tag, expected[i])
		}
	}
}

func TestEnrichTagsForObsidian_Dedup(t *testing.T) {
	tags := EnrichTagsForObsidian(
		[]string{"AI", "ai", "Ai"},
		"",
		[]string{},
	)

	if len(tags) != 1 {
		t.Errorf("dedup: got %d tags, want 1: %v", len(tags), tags)
	}
}
