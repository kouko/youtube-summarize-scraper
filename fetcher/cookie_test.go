package fetcher

import (
	"context"
	"testing"

	"github.com/kouko/youtube-summarize-scraper/config"
)

func TestHasCookies_WithFile(t *testing.T) {
	f := NewFetcher("yt-dlp", config.CookieConfig{File: "/tmp/cookies.txt"})
	if !f.hasCookies() {
		t.Error("hasCookies should return true when cookie file is set")
	}
}

func TestHasCookies_WithBrowser(t *testing.T) {
	f := NewFetcher("yt-dlp", config.CookieConfig{Browser: "chrome"})
	if !f.hasCookies() {
		t.Error("hasCookies should return true when browser is set")
	}
}

func TestHasCookies_Empty(t *testing.T) {
	f := NewFetcher("yt-dlp", config.CookieConfig{})
	if f.hasCookies() {
		t.Error("hasCookies should return false when no cookie config")
	}
}

func TestNeedsCookie(t *testing.T) {
	f := NewFetcher("yt-dlp", config.CookieConfig{})

	needsAuth := []string{"members_only", "needs_auth", "premium_only", "subscriber_only", "private"}
	for _, a := range needsAuth {
		if !f.needsCookie(a) {
			t.Errorf("needsCookie(%q) should return true", a)
		}
	}

	noAuth := []string{"public", "", "unlisted"}
	for _, a := range noAuth {
		if f.needsCookie(a) {
			t.Errorf("needsCookie(%q) should return false", a)
		}
	}
}

func TestCookieArgs_File(t *testing.T) {
	f := NewFetcher("yt-dlp", config.CookieConfig{File: "/tmp/cookies.txt"})
	args := f.cookieArgs()
	if len(args) != 2 || args[0] != "--cookies" || args[1] != "/tmp/cookies.txt" {
		t.Errorf("cookieArgs with file: got %v", args)
	}
}

func TestCookieArgs_Browser(t *testing.T) {
	f := NewFetcher("yt-dlp", config.CookieConfig{Browser: "firefox"})
	args := f.cookieArgs()
	if len(args) != 2 || args[0] != "--cookies-from-browser" || args[1] != "firefox" {
		t.Errorf("cookieArgs with browser: got %v", args)
	}
}

func TestCookieArgs_BrowserWithProfile(t *testing.T) {
	f := NewFetcher("yt-dlp", config.CookieConfig{Browser: "chrome", ChromeProfile: "Default"})
	args := f.cookieArgs()
	if len(args) != 2 || args[0] != "--cookies-from-browser" || args[1] != "chrome:Default" {
		t.Errorf("cookieArgs with browser+profile: got %v", args)
	}
}

func TestCookieArgs_Empty(t *testing.T) {
	f := NewFetcher("yt-dlp", config.CookieConfig{})
	args := f.cookieArgs()
	if args != nil {
		t.Errorf("cookieArgs empty: got %v, want nil", args)
	}
}

// fakeKeychain returns a fixed lookup result and records the names it was asked for.
func fakeKeychain(code int, err error, calls *[]string) KeychainLookup {
	return func(name string) (int, error) {
		*calls = append(*calls, name)
		return code, err
	}
}

const chromeNotAccessible = `cannot read Chrome cookies: the macOS keychain item "Chrome Safe Storage" is not accessible (keychain locked?). Unlock it and re-run: security unlock-keychain ~/Library/Keychains/login.keychain-db`

// A1 positive: locked (exit 36) → not-accessible error; missing (exit 44) → not-found error.
func TestCheckBrowserCookieKeychain_Unreadable(t *testing.T) {
	var calls []string
	err := CheckBrowserCookieKeychain("darwin", fakeKeychain(36, nil, &calls), []config.CookieConfig{{Browser: "Chrome:Default"}})
	if err == nil || err.Error() != chromeNotAccessible {
		t.Errorf("exit 36: got %v, want %q", err, chromeNotAccessible)
	}
	if len(calls) != 1 || calls[0] != "Chrome" {
		t.Errorf("lookup calls = %v, want [Chrome]", calls)
	}

	want := `cannot read Chrome cookies: no "Chrome Safe Storage" item in the macOS keychain. Open Chrome and sign in to YouTube once, then re-run.`
	err = CheckBrowserCookieKeychain("darwin", fakeKeychain(44, nil, &calls), []config.CookieConfig{{Browser: "chrome"}})
	if err == nil || err.Error() != want {
		t.Errorf("exit 44: got %v, want %q", err, want)
	}
}

// A1 negative: a non-Chrome-family browser is skipped.
func TestCheckBrowserCookieKeychain_FirefoxSkipped(t *testing.T) {
	var calls []string
	if err := CheckBrowserCookieKeychain("darwin", fakeKeychain(36, nil, &calls), []config.CookieConfig{{Browser: "firefox"}}); err != nil {
		t.Errorf("firefox: got %v, want nil", err)
	}
	if len(calls) != 0 {
		t.Errorf("firefox: lookup called %v", calls)
	}
}

// A2 positive: readable key → nil; each distinct keyring name looked up once.
func TestCheckBrowserCookieKeychain_Readable(t *testing.T) {
	var calls []string
	cookies := []config.CookieConfig{{Browser: "chrome"}, {Browser: " Chrome:Profile 1"}, {Browser: "edge+gnomekeyring"}}
	if err := CheckBrowserCookieKeychain("darwin", fakeKeychain(0, nil, &calls), cookies); err != nil {
		t.Errorf("readable: got %v, want nil", err)
	}
	if len(calls) != 2 || calls[0] != "Chrome" || calls[1] != "Microsoft Edge" {
		t.Errorf("lookup calls = %v, want [Chrome Microsoft Edge]", calls)
	}
}

// A2 boundary: lookup timed out → not-accessible error.
func TestCheckBrowserCookieKeychain_Timeout(t *testing.T) {
	var calls []string
	err := CheckBrowserCookieKeychain("darwin", fakeKeychain(-1, context.DeadlineExceeded, &calls), []config.CookieConfig{{Browser: "chrome"}})
	if err == nil || err.Error() != chromeNotAccessible {
		t.Errorf("timeout: got %v, want %q", err, chromeNotAccessible)
	}
}

// A3 positive: a cookie file wins over the browser, so no lookup runs.
func TestCheckBrowserCookieKeychain_FileSkipped(t *testing.T) {
	var calls []string
	if err := CheckBrowserCookieKeychain("darwin", fakeKeychain(36, nil, &calls), []config.CookieConfig{{File: "/tmp/c.txt", Browser: "chrome"}, {}}); err != nil {
		t.Errorf("file set: got %v, want nil", err)
	}
	if len(calls) != 0 {
		t.Errorf("file set: lookup called %v", calls)
	}
}

// A3 negative: non-macOS hosts skip the check.
func TestCheckBrowserCookieKeychain_LinuxSkipped(t *testing.T) {
	var calls []string
	if err := CheckBrowserCookieKeychain("linux", fakeKeychain(36, nil, &calls), []config.CookieConfig{{Browser: "chrome"}}); err != nil {
		t.Errorf("linux: got %v, want nil", err)
	}
	if len(calls) != 0 {
		t.Errorf("linux: lookup called %v", calls)
	}
}
