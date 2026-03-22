package fetcher

import "github.com/kouko/youtube-summarize-scraper/config"

// FilterVideos returns the subset of videos that match the given filter criteria.
func FilterVideos(videos []VideoMeta, filter config.FilterConfig) []VideoMeta {
	typeSet := make(map[string]bool, len(filter.Types))
	for _, t := range filter.Types {
		typeSet[t] = true
	}

	var result []VideoMeta
	for _, v := range videos {
		if !matchesType(v, typeSet) {
			continue
		}
		if filter.MinDuration > 0 && v.Duration < filter.MinDuration {
			continue
		}
		if filter.MaxDuration > 0 && v.Duration > filter.MaxDuration {
			continue
		}
		result = append(result, v)
	}
	return result
}

// matchesType checks whether a video matches any of the allowed type categories.
//
//   - "video":  live_status is "not_live" or empty
//   - "live":   live_status is "was_live", "is_live", or "post_live"
//   - "short":  duration < 60 and live_status is not a live variant
func matchesType(v VideoMeta, typeSet map[string]bool) bool {
	if len(typeSet) == 0 {
		return true
	}

	isLive := v.LiveStatus == "was_live" || v.LiveStatus == "is_live" || v.LiveStatus == "post_live"
	isShort := v.MediaType == "short"

	if typeSet["video"] && !isLive && !isShort {
		return true
	}
	if typeSet["live"] && isLive {
		return true
	}
	if typeSet["short"] && isShort {
		return true
	}

	return false
}
