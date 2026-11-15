package fetcher

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kouko/youtube-summarize-scraper/config"
)

func TestYtDlpError_CookieDecryptHint(t *testing.T) {
	base := errors.New("exit status 1")
	args := []string{"--flat-playlist", "PLx"}

	t.Run("locked keychain", func(t *testing.T) {
		stderr := "ERROR: cannot decrypt v10 cookies: no key found"
		hint := "cannot read browser cookies (keychain locked?) — run: " + KeychainUnlockCommand
		got := ytDlpError("darwin", args, base, stderr)
		if !strings.HasPrefix(got.Error(), hint) || !strings.Contains(got.Error(), stderr) || !errors.Is(got, base) {
			t.Errorf("darwin: got %q, want prefix %q, original stderr and wrapped err", got, hint)
		}
	})

	t.Run("wrong key", func(t *testing.T) {
		stderr := "ERROR: failed to decrypt cookie (AES-CBC) because UTF-8 decoding failed. Possibly the key is wrong?"
		hint := "cannot decrypt browser cookies with the keychain key (wrong key? browser profile copied from another machine?) — sign in to YouTube in the browser again"
		got := ytDlpError("darwin", args, base, stderr)
		if !strings.HasPrefix(got.Error(), hint) || !strings.Contains(got.Error(), stderr) || !errors.Is(got, base) {
			t.Errorf("darwin: got %q, want prefix %q, original stderr and wrapped err", got, hint)
		}
		if strings.Contains(got.Error(), "unlock-keychain") {
			t.Errorf("darwin wrong key: hint must not suggest the unlock command: %q", got)
		}
	})

	t.Run("no hint off mac", func(t *testing.T) {
		stderr := "ERROR: cannot decrypt v10 cookies: no key found"
		plain := ytDlpError("linux", args, base, stderr)
		want := "yt-dlp [--flat-playlist PLx]: exit status 1\nstderr: " + stderr
		if plain.Error() != want || !errors.Is(plain, base) {
			t.Errorf("linux: got %q, want %q", plain, want)
		}
	})
}

func TestChannelTabSuffixes_Video(t *testing.T) {
	got := ChannelTabSuffixes([]string{"video"})
	want := []string{"/videos"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ChannelTabSuffixes([video]): got %v, want %v", got, want)
	}
}

func TestChannelTabSuffixes_Live(t *testing.T) {
	got := ChannelTabSuffixes([]string{"live"})
	want := []string{"/streams"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ChannelTabSuffixes([live]): got %v, want %v", got, want)
	}
}

func TestChannelTabSuffixes_Short(t *testing.T) {
	got := ChannelTabSuffixes([]string{"short"})
	want := []string{"/shorts"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ChannelTabSuffixes([short]): got %v, want %v", got, want)
	}
}

func TestChannelTabSuffixes_VideoAndLive(t *testing.T) {
	got := ChannelTabSuffixes([]string{"video", "live"})
	want := []string{"/videos", "/streams"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ChannelTabSuffixes([video,live]): got %v, want %v", got, want)
	}
}

func TestChannelTabSuffixes_All(t *testing.T) {
	got := ChannelTabSuffixes([]string{"video", "live", "short"})
	want := []string{"/videos", "/streams", "/shorts"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ChannelTabSuffixes([video,live,short]): got %v, want %v", got, want)
	}
}

func TestChannelTabSuffixes_Empty(t *testing.T) {
	got := ChannelTabSuffixes([]string{})
	want := []string{"/videos", "/streams", "/shorts"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ChannelTabSuffixes([]): got %v, want %v", got, want)
	}
}

func TestChannelTabSuffixes_Nil(t *testing.T) {
	got := ChannelTabSuffixes(nil)
	want := []string{"/videos", "/streams", "/shorts"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ChannelTabSuffixes(nil): got %v, want %v", got, want)
	}
}

// concern: silent regression of metadataTimeout back to a throttling-fatal value (60s) that SIGKILLs throttled yt-dlp metadata fetches mid-run
// metadataTimeout must stay wide enough that a throttled yt-dlp full-metadata
// fetch (~26s measured on 2026-11-15 under YouTube throttling) finishes.
func TestMetadataTimeout_AtLeast120s(t *testing.T) {
	if metadataTimeout < 120*time.Second {
		t.Errorf("metadataTimeout = %v, want >= 120s so throttled metadata fetches (~26s) are not killed", metadataTimeout)
	}
}

// The previous 60s deadline is the observed failure threshold: on 2026-11-14
// and 2026-11-15 yt-dlp metadata fetches exceeded 60s under YouTube
// throttling, hit the deadline, and were SIGKILLed (exec.CommandContext) —
// observed as 427 failures on 11-14 and 106 on 11-15, all "full metadata
// fetch failed".
func TestMetadataTimeout_Below60WouldFail(t *testing.T) {
	if metadataTimeout <= 60*time.Second {
		t.Errorf("metadataTimeout = %v, want > 60s (the observed throttled-fetch SIGKILL threshold)", metadataTimeout)
	}
}

// fakeYtDlp returns a shell script that sleeps then prints one JSON metadata
// line, standing in for the real yt-dlp binary. The sleep's stdout/stderr are
// redirected so a killed sh does not leave the orphaned sleep holding the
// command pipes open.
func fakeYtDlp(t *testing.T, sleep time.Duration) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "yt-dlp")
	script := "#!/bin/sh\nsleep %.1f >/dev/null 2>&1\necho '{\"id\":\"fake-video\",\"title\":\"fake\"}'\n"
	if err := os.WriteFile(path, []byte(fmt.Sprintf(script, sleep.Seconds())), 0o755); err != nil {
		t.Fatalf("writing fake yt-dlp: %v", err)
	}
	return path
}

// The default metadata path (FetchVideoMeta -> runYtDlp) must apply the
// package-level metadataTimeout: with the var overridden to a short value a
// slow fetch is killed; with a generous value the same fetch succeeds.
func TestWidenedValueAppliesToRunYtDlp(t *testing.T) {
	orig := metadataTimeout
	t.Cleanup(func() { metadataTimeout = orig })

	f := NewFetcher(fakeYtDlp(t, time.Second), config.CookieConfig{})

	metadataTimeout = 500 * time.Millisecond
	if _, err := f.FetchVideoMeta("https://example.com/v"); err == nil {
		t.Fatal("FetchVideoMeta with metadataTimeout=500ms succeeded; want timeout error, proving the default path applies metadataTimeout")
	}

	metadataTimeout = 5 * time.Second
	meta, err := f.FetchVideoMeta("https://example.com/v")
	if err != nil {
		t.Fatalf("FetchVideoMeta with metadataTimeout=3s failed: %v", err)
	}
	if meta.ID != "fake-video" {
		t.Errorf("FetchVideoMeta: got ID %q, want %q", meta.ID, "fake-video")
	}
}

// runYtDlpWithTimeout must still kill a hung yt-dlp when its deadline hits,
// after the metadataTimeout value change (proves the kill-on-timeout logic).
func TestRunYtDlpWithTimeout_ShortTimeoutStillKills(t *testing.T) {
	f := NewFetcher(fakeYtDlp(t, time.Second), config.CookieConfig{})
	_, err := f.runYtDlpWithTimeout([]string{"--dump-json", "https://example.com/v"}, false, 500*time.Millisecond)
	if err == nil {
		t.Fatal("runYtDlpWithTimeout with 500ms timeout succeeded; want kill-on-timeout error")
	}
	if !strings.Contains(err.Error(), "signal: killed") {
		t.Errorf("runYtDlpWithTimeout: got %q, want error reporting SIGKILL from the context deadline", err)
	}
}
