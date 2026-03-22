package fetcher

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// cookieArgs returns yt-dlp cookie arguments based on the configured cookie settings.
func (f *Fetcher) cookieArgs() []string {
	if f.cookieConfig.File != "" {
		return []string{"--cookies", f.cookieConfig.File}
	}
	if f.cookieConfig.Browser != "" {
		browser := f.cookieConfig.Browser
		profile := ResolveChromeProfile(f.cookieConfig.ChromeProfile)
		if profile != "" {
			browser += ":" + profile
		}
		return []string{"--cookies-from-browser", browser}
	}
	return nil
}

// ResolveChromeProfile resolves a Chrome profile identifier to a directory name.
// If the input contains "@" (email), it scans Chrome profile directories to find
// the matching account. Otherwise, it returns the input as-is (directory name).
func ResolveChromeProfile(profile string) string {
	if profile == "" {
		return ""
	}
	if !strings.Contains(profile, "@") {
		return profile // already a directory name
	}

	chromeDir := chromeConfigDir()
	if chromeDir == "" {
		slog.Warn("cannot determine Chrome config directory for this OS")
		return profile
	}

	// Scan profile directories: Default, Profile 1, Profile 2, etc.
	candidates := []string{"Default"}
	entries, err := os.ReadDir(chromeDir)
	if err != nil {
		slog.Warn("cannot read Chrome directory", "path", chromeDir, "error", err)
		return profile
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "Profile ") {
			candidates = append(candidates, e.Name())
		}
	}

	for _, dirName := range candidates {
		prefsPath := filepath.Join(chromeDir, dirName, "Preferences")
		email := readChromeProfileEmail(prefsPath)
		if strings.EqualFold(email, profile) {
			slog.Info("resolved Chrome profile email to directory", "email", profile, "dir", dirName)
			return dirName
		}
	}

	slog.Warn("Chrome profile not found for email, using as-is", "email", profile)
	return profile
}

// chromeConfigDir returns the Chrome user data directory for the current OS.
func chromeConfigDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(homeDir, "Library", "Application Support", "Google", "Chrome")
	case "linux":
		return filepath.Join(homeDir, ".config", "google-chrome")
	case "windows":
		return filepath.Join(homeDir, "AppData", "Local", "Google", "Chrome", "User Data")
	default:
		return ""
	}
}

// readChromeProfileEmail reads the email from a Chrome Preferences file.
func readChromeProfileEmail(prefsPath string) string {
	data, err := os.ReadFile(prefsPath)
	if err != nil {
		return ""
	}
	var prefs struct {
		AccountInfo []struct {
			Email string `json:"email"`
		} `json:"account_info"`
	}
	if err := json.Unmarshal(data, &prefs); err != nil {
		return ""
	}
	if len(prefs.AccountInfo) > 0 {
		return prefs.AccountInfo[0].Email
	}
	return ""
}

// needsCookie returns true when the video's availability requires authentication cookies.
func (f *Fetcher) needsCookie(availability string) bool {
	switch availability {
	case "members_only", "needs_auth", "premium_only", "subscriber_only", "private":
		return true
	default:
		return false
	}
}
