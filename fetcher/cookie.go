package fetcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/kouko/youtube-summarize-scraper/config"
)

// KeychainUnlockCommand is the copy-pasteable fix for a locked macOS login keychain.
const KeychainUnlockCommand = "security unlock-keychain ~/Library/Keychains/login.keychain-db"

// keychainLookupTimeout bounds the keychain lookup so an unattended run never waits for input.
const keychainLookupTimeout = 30 * time.Second

// macKeyringNames maps yt-dlp browser names to their macOS keychain account names
// (yt-dlp's macOS keyring table). Browsers not listed do not use the keychain.
var macKeyringNames = map[string]string{
	"chrome":   "Chrome",
	"chromium": "Chromium",
	"brave":    "Brave",
	"edge":     "Microsoft Edge",
	"opera":    "Opera",
	"vivaldi":  "Vivaldi",
	"whale":    "Whale",
}

// KeychainLookup looks up the "<name> Safe Storage" keychain item for the given
// keyring name. It returns the lookup's exit code, or a non-nil error when the
// lookup did not complete (timeout, failure to start); a non-nil error means
// the key is not accessible.
type KeychainLookup func(name string) (exitCode int, err error)

// LookupMacKeychain runs the same `security find-generic-password -w` lookup
// yt-dlp performs. The key is written to the null device and never read; only
// the exit code is used. The child is killed after 30 seconds.
func LookupMacKeychain(name string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), keychainLookupTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "security", "find-generic-password", "-w", "-a", name, "-s", name+" Safe Storage")
	// cmd.Stdout and cmd.Stderr stay nil: output goes to the null device.
	err := cmd.Run()
	if ctx.Err() != nil {
		return -1, ctx.Err()
	}
	if err == nil {
		return 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	return -1, err
}

// CheckBrowserCookieKeychain reports whether the macOS keychain key that
// decrypts each configured Chrome-family browser's cookies is readable.
// Only entries that read cookies from a browser (File empty, Browser set) are
// checked; each distinct keyring name is looked up once and the first failure
// is returned. Non-darwin hosts and non-Chrome-family browsers are skipped.
func CheckBrowserCookieKeychain(goos string, lookup KeychainLookup, cookies []config.CookieConfig) error {
	if goos != "darwin" {
		return nil
	}
	checked := map[string]bool{}
	for _, c := range cookies {
		if c.File != "" || c.Browser == "" {
			continue
		}
		name, ok := macKeyringNames[browserName(c.Browser)]
		if !ok || checked[name] {
			continue
		}
		checked[name] = true
		code, err := lookup(name)
		switch {
		case err == nil && code == 0:
			continue
		case err == nil && code == 44:
			return fmt.Errorf("cannot read %s cookies: no \"%s Safe Storage\" item in the macOS keychain. Open %s and sign in to YouTube once, then re-run.", name, name, name)
		default:
			return fmt.Errorf("cannot read %s cookies: the macOS keychain item \"%s Safe Storage\" is not accessible (keychain locked?). Unlock it and re-run: %s", name, name, KeychainUnlockCommand)
		}
	}
	return nil
}

// browserName parses a yt-dlp --cookies-from-browser value the way yt-dlp does:
// the text before the first '+' or ':', trimmed and lowercased.
func browserName(spec string) string {
	if i := strings.IndexAny(spec, "+:"); i >= 0 {
		spec = spec[:i]
	}
	return strings.ToLower(strings.TrimSpace(spec))
}

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

// hasCookies reports whether any cookie configuration is available.
func (f *Fetcher) hasCookies() bool {
	return f.cookieConfig.File != "" || f.cookieConfig.Browser != ""
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
