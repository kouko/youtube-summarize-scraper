package fetcher

import (
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
