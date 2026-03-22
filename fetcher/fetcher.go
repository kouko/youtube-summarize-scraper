package fetcher

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/kouko/youtube-summarize-scraper/config"
)

const metadataTimeout = 60 * time.Second

// Fetcher wraps yt-dlp for retrieving video metadata and channel listings.
type Fetcher struct {
	ytdlpPath    string
	cookieConfig config.CookieConfig
}

// NewFetcher creates a Fetcher with the given yt-dlp binary path and cookie configuration.
func NewFetcher(ytdlpPath string, cookie config.CookieConfig) *Fetcher {
	return &Fetcher{
		ytdlpPath:    ytdlpPath,
		cookieConfig: cookie,
	}
}

// FetchVideoMeta retrieves full metadata for a single video URL.
// If the video requires authentication (based on availability), cookies are used automatically.
func (f *Fetcher) FetchVideoMeta(videoURL string) (*VideoMeta, error) {
	args := []string{"--dump-json", "--no-download", videoURL}
	out, err := f.runYtDlp(args, false)
	if err != nil {
		return nil, fmt.Errorf("fetching video metadata: %w", err)
	}

	var meta VideoMeta
	if err := json.Unmarshal(out, &meta); err != nil {
		return nil, fmt.Errorf("parsing video metadata JSON: %w", err)
	}

	// Retry with cookies if the video requires authentication.
	if f.needsCookie(meta.Availability) {
		out, err = f.runYtDlp(args, true)
		if err != nil {
			return nil, fmt.Errorf("fetching video metadata with cookies: %w", err)
		}
		if err := json.Unmarshal(out, &meta); err != nil {
			return nil, fmt.Errorf("parsing video metadata JSON (cookie retry): %w", err)
		}
	}

	return &meta, nil
}

// ChannelTabSuffixes returns the URL suffixes to fetch based on the requested content types.
// Each type maps to a specific YouTube channel tab.
func ChannelTabSuffixes(types []string) []string {
	if len(types) == 0 {
		return []string{"/videos", "/streams", "/shorts"}
	}
	typeSet := make(map[string]bool, len(types))
	for _, t := range types {
		typeSet[t] = true
	}
	var suffixes []string
	if typeSet["video"] {
		suffixes = append(suffixes, "/videos")
	}
	if typeSet["live"] {
		suffixes = append(suffixes, "/streams")
	}
	if typeSet["short"] {
		suffixes = append(suffixes, "/shorts")
	}
	if len(suffixes) == 0 {
		return []string{"/videos", "/streams", "/shorts"}
	}
	return suffixes
}

// FetchChannelVideos lists videos from a channel URL using --flat-playlist for speed.
// Returns lightweight metadata (ID, title, duration). Full metadata must be fetched
// separately via FetchVideoMeta for videos that need processing.
func (f *Fetcher) FetchChannelVideos(channelURL string, limit int, tabSuffixes []string) ([]VideoMeta, error) {
	var allVideos []VideoMeta

	for _, suffix := range tabSuffixes {
		tabURL := channelURL + suffix
		remaining := limit - len(allVideos)
		if remaining <= 0 {
			break
		}

		videos, err := f.fetchChannelTab(tabURL, remaining)
		if err != nil {
			return nil, fmt.Errorf("fetching %s: %w", tabURL, err)
		}
		allVideos = append(allVideos, videos...)
	}

	if len(allVideos) > limit {
		allVideos = allVideos[:limit]
	}
	return allVideos, nil
}

// fetchChannelTab fetches videos from a single channel tab URL using flat-playlist.
func (f *Fetcher) fetchChannelTab(tabURL string, limit int) ([]VideoMeta, error) {
	args := []string{
		"--flat-playlist",
		"--dump-json",
		"--playlist-end", fmt.Sprintf("%d", limit),
		tabURL,
	}

	out, err := f.runYtDlpWithTimeout(args, false, 5*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("fetching channel videos: %w", err)
	}

	var videos []VideoMeta
	scanner := bufio.NewScanner(bytes.NewReader(out))
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for scanner.Scan() {
		if limit > 0 && len(videos) >= limit {
			break
		}
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var meta VideoMeta
		if err := json.Unmarshal(line, &meta); err != nil {
			continue
		}
		if meta.ID == "" {
			continue
		}
		videos = append(videos, meta)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading channel video listing: %w", err)
	}

	return videos, nil
}

// FetchPlaylistVideos lists videos from a playlist URL using --flat-playlist.
// Returns video metadata, the playlist title (from yt-dlp's playlist_title field), and error.
// cookieArgs are additional yt-dlp cookie arguments (e.g., from per-playlist config).
func (f *Fetcher) FetchPlaylistVideos(playlistURL string, limit int, cookieArgs []string) ([]VideoMeta, string, error) {
	args := []string{
		"--flat-playlist",
		"--dump-json",
		"--playlist-end", fmt.Sprintf("%d", limit),
	}
	args = append(args, cookieArgs...)
	args = append(args, playlistURL)

	out, err := f.runYtDlpWithTimeout(args, false, 5*time.Minute)
	if err != nil {
		return nil, "", fmt.Errorf("fetching playlist videos: %w", err)
	}

	var videos []VideoMeta
	var playlistTitle string
	scanner := bufio.NewScanner(bytes.NewReader(out))
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for scanner.Scan() {
		if limit > 0 && len(videos) >= limit {
			break
		}
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var meta VideoMeta
		if err := json.Unmarshal(line, &meta); err != nil {
			continue
		}
		if meta.ID == "" {
			continue
		}
		if playlistTitle == "" && meta.PlaylistTitle != "" {
			playlistTitle = meta.PlaylistTitle
		}
		videos = append(videos, meta)
	}
	if err := scanner.Err(); err != nil {
		return nil, "", fmt.Errorf("reading playlist video listing: %w", err)
	}

	return videos, playlistTitle, nil
}

// runYtDlp executes yt-dlp with the given arguments and an optional cookie flag.
// A context timeout of 60 seconds is applied.
func (f *Fetcher) runYtDlp(args []string, useCookie bool) ([]byte, error) {
	return f.runYtDlpWithTimeout(args, useCookie, metadataTimeout)
}

// runYtDlpWithTimeout executes yt-dlp with a custom timeout.
func (f *Fetcher) runYtDlpWithTimeout(args []string, useCookie bool, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	fullArgs := make([]string, 0, len(args)+4)
	if useCookie {
		fullArgs = append(fullArgs, f.cookieArgs()...)
	}
	fullArgs = append(fullArgs, args...)

	cmd := exec.CommandContext(ctx, f.ytdlpPath, fullArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("yt-dlp %v: %w\nstderr: %s", args, err, stderr.String())
	}

	return stdout.Bytes(), nil
}
