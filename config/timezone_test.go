package config

import (
	"testing"
	"time"
)

func TestLoadTimezone_IANA(t *testing.T) {
	loc := LoadTimezone("Asia/Taipei")
	if loc.String() != "Asia/Taipei" {
		t.Errorf("expected Asia/Taipei, got %s", loc.String())
	}
}

func TestLoadTimezone_Invalid(t *testing.T) {
	loc := LoadTimezone("Invalid/Zone")
	if loc != time.UTC {
		t.Errorf("expected UTC for invalid timezone, got %s", loc.String())
	}
}

func TestLoadTimezone_Empty(t *testing.T) {
	loc := LoadTimezone("")
	if loc == nil {
		t.Fatal("expected non-nil location for empty timezone")
	}
	// Should resolve to a real IANA name, not "Local".
	if loc.String() == "Local" {
		t.Error("expected IANA timezone name, got 'Local'")
	}
}

func TestLoadTimezone_UTC(t *testing.T) {
	loc := LoadTimezone("UTC")
	if loc.String() != "UTC" {
		t.Errorf("expected UTC, got %s", loc.String())
	}
}

func TestExtractIANA(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/var/db/timezone/zoneinfo/Asia/Taipei", "Asia/Taipei"},
		{"/usr/share/zoneinfo/US/Pacific", "US/Pacific"},
		{"/usr/share/zoneinfo/UTC", "UTC"},
		{"/some/random/path", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := extractIANA(tt.path)
		if got != tt.want {
			t.Errorf("extractIANA(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}
