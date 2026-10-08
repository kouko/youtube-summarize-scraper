package tui

import (
	"strconv"
	"strings"

	"github.com/kouko/youtube-summarize-scraper/pipeline"
)

// EventKind classifies a parsed slog line.
type EventKind int

const (
	EventOther EventKind = iota
	EventVideoStart
	EventStage
	EventVideoSkipped
	EventVideoFailed
	EventVideoPartial
	EventVideoDone
	EventBatchComplete
	EventWatchIter
	EventFetched
)

// LogEvent is one parsed log line, ready to be applied to AppState.
type LogEvent struct {
	Kind  EventKind
	Video string          // video title (falls back to URL), for VideoStart
	Stage string          // current processing stage
	Error bool            // true for error-level lines
	Iter  int             // watch iteration number, for EventWatchIter
	Count int             // fetched video count, for EventFetched
	Stats *pipeline.Stats // set for EventBatchComplete
	Raw   string          // original line (for the events panel)
}

// ParseLogLine parses one line of slog TextHandler output into a LogEvent.
// Lines that carry no ytss status information become EventOther.
func ParseLogLine(line string) LogEvent {
	ev := LogEvent{Kind: EventOther, Raw: line}
	msg, ok := findAttr(line, "msg")
	if !ok {
		return ev
	}

	switch {
	case strings.Contains(msg, "streaming batch complete"):
		ev.Kind = EventBatchComplete
		ev.Stats = &pipeline.Stats{
			Success: attrInt(line, "success"),
			Skipped: attrInt(line, "skipped"),
			Partial: attrInt(line, "partial"),
			Failed:  attrInt(line, "failed"),
		}

	case strings.HasPrefix(msg, "watch: iteration "):
		// "watch: iteration 3 starting" / "watch: iteration 3 complete, ..."
		ev.Kind = EventWatchIter
		if fields := strings.Fields(msg); len(fields) >= 3 {
			ev.Iter, _ = strconv.Atoi(fields[2])
		}

	case strings.Contains(msg, "total filtered videos across tabs"),
		strings.Contains(msg, "fetched playlist videos"):
		ev.Kind = EventFetched
		if c, ok := findAttr(line, "count"); ok {
			ev.Count, _ = strconv.Atoi(c)
		} else if c, ok := findAttr(line, "total"); ok {
			ev.Count, _ = strconv.Atoi(c)
		}

	case strings.Contains(msg, "streaming: processing channel video"),
		strings.Contains(msg, "streaming: processing playlist video"):
		ev.Kind = EventVideoStart
		ev.Video = attr(line, "title")
		if ev.Video == "" {
			ev.Video = attr(line, "url")
		}
		if ev.Video == "" {
			ev.Video = "(video)"
		}
		ev.Stage = "starting"

	case attr(line, "level") == "ERROR" && strings.Contains(msg, "processing failed"):
		// Covers "video processing failed" and
		// "streaming: channel/playlist video processing failed".
		ev.Kind = EventVideoFailed
		ev.Error = true

	case strings.Contains(msg, "summarization failed, transcription still produced"):
		ev.Kind = EventVideoPartial

	case strings.Contains(msg, "video processing complete"),
		strings.Contains(msg, "resume complete"):
		ev.Kind = EventVideoDone

	case strings.Contains(msg, " - skipped ("):
		ev.Kind = EventVideoSkipped

	default:
		if stage := matchStage(msg); stage != "" {
			ev.Kind = EventStage
			ev.Stage = stage
		}
	}
	return ev
}

// matchStage maps pipeline stage log messages to short stage labels.
func matchStage(msg string) string {
	switch {
	case strings.Contains(msg, "stage 1: generating summary"):
		return "stage 1: generating summary"
	case strings.Contains(msg, "stage 2: extracting keywords"):
		return "stage 2: extracting keywords"
	case strings.Contains(msg, "stage 3: generating mermaid diagrams"):
		return "stage 3: generating mermaid"
	case strings.Contains(msg, "subtitle download failed, attempting whisper transcription"):
		return "transcribing (whisper)"
	case strings.Contains(msg, "whisper transcription succeeded"):
		return "transcription done"
	case strings.Contains(msg, "subtitle download succeeded"):
		return "subtitles downloaded"
	case strings.Contains(msg, "waiting before next item"):
		return "waiting (rate limit)"
	}
	return ""
}

// findAttr returns the value of a top-level slog attribute in line.
// The attribute must start at a word boundary (start of line or after a
// space) so that a value containing "key=" never matches.
func findAttr(line, key string) (string, bool) {
	prefix := key + "="
	start := 0
	for {
		idx := strings.Index(line[start:], prefix)
		if idx < 0 {
			return "", false
		}
		pos := start + idx
		if pos > 0 && line[pos-1] != ' ' && line[pos-1] != '\t' {
			start = pos + len(prefix)
			continue
		}
		return parseAttrValue(line[pos+len(prefix):]), true
	}
}

// parseAttrValue decodes a single slog attribute value: a double-quoted
// string (with backslash escapes) or a bare token up to the next space.
func parseAttrValue(s string) string {
	if s == "" {
		return ""
	}
	if s[0] != '"' {
		if idx := strings.IndexAny(s, " \t"); idx >= 0 {
			return s[:idx]
		}
		return s
	}
	var b strings.Builder
	for i := 1; i < len(s); i++ {
		c := s[i]
		if c == '\\' && i+1 < len(s) {
			i++
			b.WriteByte(s[i])
			continue
		}
		if c == '"' {
			break
		}
		b.WriteByte(c)
	}
	return b.String()
}

// attr is findAttr with an empty-string default.
func attr(line, key string) string {
	v, _ := findAttr(line, key)
	return v
}

// attrInt parses a numeric slog attribute, defaulting to 0.
func attrInt(line, key string) int {
	v, ok := findAttr(line, key)
	if !ok {
		return 0
	}
	n, _ := strconv.Atoi(v)
	return n
}
