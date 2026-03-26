package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// LoadTimezone resolves a timezone location from the given name.
// Resolution order: IANA name → system timezone (via /etc/localtime) → UTC.
func LoadTimezone(name string) *time.Location {
	if name != "" {
		loc, err := time.LoadLocation(name)
		if err != nil {
			slog.Warn("invalid timezone, falling back to UTC", "timezone", name, "error", err)
			return time.UTC
		}
		return loc
	}

	if iana := detectSystemTimezone(); iana != "" {
		loc, err := time.LoadLocation(iana)
		if err == nil {
			return loc
		}
	}

	return time.UTC
}

// detectSystemTimezone attempts to resolve the IANA timezone name from the system.
// It checks /etc/localtime (symlink on macOS/Linux) and /etc/timezone (Debian/Ubuntu).
func detectSystemTimezone() string {
	// Try /etc/localtime symlink (macOS, most Linux).
	if target, err := os.Readlink("/etc/localtime"); err == nil {
		if iana := extractIANA(target); iana != "" {
			return iana
		}
	}

	// Try /etc/timezone (Debian/Ubuntu).
	if data, err := os.ReadFile("/etc/timezone"); err == nil {
		if tz := strings.TrimSpace(string(data)); tz != "" {
			return tz
		}
	}

	return ""
}

// extractIANA extracts the IANA timezone name from a zoneinfo file path.
// e.g. "/var/db/timezone/zoneinfo/Asia/Taipei" → "Asia/Taipei"
//
//	"/usr/share/zoneinfo/US/Pacific"          → "US/Pacific"
func extractIANA(path string) string {
	path = filepath.Clean(path)
	const marker = "zoneinfo/"
	if idx := strings.LastIndex(path, marker); idx >= 0 {
		return path[idx+len(marker):]
	}
	return ""
}
