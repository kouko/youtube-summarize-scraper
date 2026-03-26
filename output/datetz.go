package output

import "time"

// ConvertUploadDate converts a yt-dlp upload date from UTC to the target timezone.
// If timestamp is available (non-zero), it uses the precise time for conversion.
// Otherwise, it falls back to parsing the YYYYMMDD string as UTC midnight.
// Returns formatted date string "YYYY-MM-DD" in the target timezone.
func ConvertUploadDate(uploadDate string, timestamp int64, loc *time.Location) string {
	if loc == nil {
		loc = time.UTC
	}

	if timestamp > 0 {
		return time.Unix(timestamp, 0).In(loc).Format("2006-01-02")
	}

	if len(uploadDate) == 8 {
		t, err := time.Parse("20060102", uploadDate)
		if err == nil {
			return t.In(loc).Format("2006-01-02")
		}
	}

	return formatDate(uploadDate)
}

// FormatUploadTime formats a Unix timestamp as a timezone-aware datetime string.
// Returns "2006-01-02T15:04:05+08:00" format in the target timezone.
// Returns "" if timestamp is 0 (unavailable).
func FormatUploadTime(timestamp int64, loc *time.Location) string {
	if timestamp == 0 {
		return ""
	}
	if loc == nil {
		loc = time.UTC
	}
	return time.Unix(timestamp, 0).In(loc).Format(time.RFC3339)
}
