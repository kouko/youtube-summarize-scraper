package fetcher

import "github.com/kouko/youtube-summarize-scraper/config"

// FilterVideos returns the subset of videos that match duration filter criteria.
// Type filtering (video/live/short) is handled at the URL tab level, not here.
func FilterVideos(videos []VideoMeta, filter config.FilterConfig) []VideoMeta {
	var result []VideoMeta
	for _, v := range videos {
		if filter.MinDuration > 0 && v.Duration < float64(filter.MinDuration) {
			continue
		}
		if filter.MaxDuration > 0 && v.Duration > float64(filter.MaxDuration) {
			continue
		}
		result = append(result, v)
	}
	return result
}
