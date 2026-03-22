package fetcher

// VideoMeta holds metadata for a single YouTube video, as returned by yt-dlp --dump-json.
type VideoMeta struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	Channel        string   `json:"uploader_id"`      // handle like @HighYield
	ChannelName    string   `json:"channel"`           // display name like "High Yield"
	UploadDate     string   `json:"upload_date"`       // YYYYMMDD
	Duration       int      `json:"duration"`          // seconds
	DurationString string   `json:"duration_string"`
	MediaType      string   `json:"media_type"`        // "short" for Shorts, empty for regular
	Language       string   `json:"language"`
	Tags           []string `json:"tags"`
	Categories     []string `json:"categories"`
	Availability   string   `json:"availability"`
	LiveStatus     string   `json:"live_status"`
	URL            string   `json:"webpage_url"`
	Description    string   `json:"description"`
}
