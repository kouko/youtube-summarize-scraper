package fetcher

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestYtDlpError_CookieDecryptHint(t *testing.T) {
	base := errors.New("exit status 1")
	stderr := "ERROR: cannot decrypt v10 cookies: no key found"
	args := []string{"--flat-playlist", "PLx"}

	got := ytDlpError("darwin", args, base, stderr)
	hint := "cannot read browser cookies (keychain locked?) — run: " + KeychainUnlockCommand
	if !strings.HasPrefix(got.Error(), hint) || !strings.Contains(got.Error(), stderr) || !errors.Is(got, base) {
		t.Errorf("darwin: got %q, want prefix %q, original stderr and wrapped err", got, hint)
	}

	plain := ytDlpError("linux", args, base, stderr)
	want := "yt-dlp [--flat-playlist PLx]: exit status 1\nstderr: " + stderr
	if plain.Error() != want || !errors.Is(plain, base) {
		t.Errorf("linux: got %q, want %q", plain, want)
	}
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
