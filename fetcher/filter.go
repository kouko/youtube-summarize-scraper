package fetcher

import "github.com/kouko/youtube-summarize-scraper/config"

// FilterVideos returns the subset of videos that match filter criteria.
// Upcoming and currently live streams are always excluded (cannot be downloaded).
// Type filtering (video/live/short) is handled at the URL tab level, not here.
func FilterVideos(videos []VideoMeta, filter config.FilterConfig) []VideoMeta {
	var result []VideoMeta
	for _, v := range videos {
		// Skip upcoming and currently live streams (cannot be downloaded).
		if v.LiveStatus == "is_upcoming" || v.LiveStatus == "is_live" {
			continue
		}
		if filter.MinDuration > 0 && v.Duration < float64(filter.MinDuration) {
			continue
		}
		if filter.MaxDuration > 0 && v.Duration > float64(filter.MaxDuration) {
			continue
		}
		if isExcludedAvailability(v.Availability, filter.ExcludeAvailability) {
			continue
		}
		result = append(result, v)
	}
	return result
}

// isExcludedAvailability returns true if the video's availability matches
// any value in the exclude list.
func isExcludedAvailability(availability string, excludeList []string) bool {
	if availability == "" || len(excludeList) == 0 {
		return false
	}
	for _, ea := range excludeList {
		if availability == ea {
			return true
		}
	}
	return false
}
