package tui

import (
	"testing"
)

const testTs = "time=2026-10-09T10:00:00.000+08:00"

func TestParseVideoStart(t *testing.T) {
	line := testTs + ` level=INFO msg="streaming: processing channel video" url="https://www.youtube.com/watch?v=abc" title="My Video"`
	ev := ParseLogLine(line)
	if ev.Kind != EventVideoStart {
		t.Fatalf("Kind = %v, want EventVideoStart", ev.Kind)
	}
	if ev.Video != "My Video" {
		t.Errorf("Video = %q, want %q", ev.Video, "My Video")
	}
	if ev.Stage != "starting" {
		t.Errorf("Stage = %q, want %q", ev.Stage, "starting")
	}
	if ev.Error {
		t.Error("Error = true, want false")
	}
}

func TestParseVideoStartURLFallback(t *testing.T) {
	line := testTs + ` level=INFO msg="streaming: processing playlist video" url="https://youtu.be/x"`
	ev := ParseLogLine(line)
	if ev.Kind != EventVideoStart {
		t.Fatalf("Kind = %v, want EventVideoStart", ev.Kind)
	}
	if ev.Video != "https://youtu.be/x" {
		t.Errorf("Video = %q, want URL fallback", ev.Video)
	}
}

func TestParseStages(t *testing.T) {
	cases := []struct {
		msg       string
		wantStage string
	}{
		{"stage 1: generating summary", "stage 1: generating summary"},
		{"stage 2: extracting keywords", "stage 2: extracting keywords"},
		{"stage 3: generating mermaid diagrams", "stage 3: generating mermaid"},
		{"subtitle download failed, attempting whisper transcription", "transcribing (whisper)"},
		{"subtitle download succeeded", "subtitles downloaded"},
		{"whisper transcription succeeded", "transcription done"},
		{"waiting before next item", "waiting (rate limit)"},
	}
	for _, c := range cases {
		line := testTs + ` level=INFO msg="` + c.msg + `" video_id=x`
		ev := ParseLogLine(line)
		if ev.Kind != EventStage {
			t.Errorf("%q: Kind = %v, want EventStage", c.msg, ev.Kind)
		}
		if ev.Stage != c.wantStage {
			t.Errorf("%q: Stage = %q, want %q", c.msg, ev.Stage, c.wantStage)
		}
	}
}

func TestParseSkips(t *testing.T) {
	msgs := []string{
		"[1/5] abc - skipped (already processed)",
		"[2/5] def - skipped (marked .skipped)",
		"[3/5] ghi - skipped (complete)",
	}
	for _, msg := range msgs {
		line := testTs + ` level=INFO msg="` + msg + `" title="T"`
		if ev := ParseLogLine(line); ev.Kind != EventVideoSkipped {
			t.Errorf("%q: Kind = %v, want EventVideoSkipped", msg, ev.Kind)
		}
	}
}

func TestParseFailures(t *testing.T) {
	cases := []string{
		"video processing failed",
		"streaming: channel video processing failed",
		"streaming: playlist video processing failed",
	}
	for _, msg := range cases {
		line := testTs + ` level=ERROR msg="` + msg + `" video_id=x error="boom"`
		ev := ParseLogLine(line)
		if ev.Kind != EventVideoFailed {
			t.Errorf("%q: Kind = %v, want EventVideoFailed", msg, ev.Kind)
		}
		if !ev.Error {
			t.Errorf("%q: Error = false, want true", msg)
		}
	}
}

func TestParsePartialAndDone(t *testing.T) {
	partial := testTs + ` level=WARN msg="summarization failed, transcription still produced" video_id=x`
	if ev := ParseLogLine(partial); ev.Kind != EventVideoPartial {
		t.Errorf("partial: Kind = %v, want EventVideoPartial", ev.Kind)
	}
	for _, msg := range []string{"video processing complete", "resume complete"} {
		line := testTs + ` level=INFO msg="` + msg + `" video_id=x`
		if ev := ParseLogLine(line); ev.Kind != EventVideoDone {
			t.Errorf("%q: Kind = %v, want EventVideoDone", msg, ev.Kind)
		}
	}
}

func TestParseBatchComplete(t *testing.T) {
	line := testTs + ` level=INFO msg="streaming batch complete" success=3 skipped=2 partial=1 failed=4`
	ev := ParseLogLine(line)
	if ev.Kind != EventBatchComplete {
		t.Fatalf("Kind = %v, want EventBatchComplete", ev.Kind)
	}
	if ev.Stats == nil {
		t.Fatal("Stats = nil, want non-nil")
	}
	if ev.Stats.Success != 3 || ev.Stats.Skipped != 2 || ev.Stats.Partial != 1 || ev.Stats.Failed != 4 {
		t.Errorf("Stats = %+v, want {3 2 1 4}", *ev.Stats)
	}
}

func TestParseOtherLineNoEvent(t *testing.T) {
	line := testTs + ` level=INFO msg="fetched channel tab" tab=videos url="https://x" fetched=5`
	ev := ParseLogLine(line)
	if ev.Kind != EventOther {
		t.Errorf("Kind = %v, want EventOther", ev.Kind)
	}
	if ev.Error {
		t.Error("Error = true, want false")
	}
}

func TestFindAttrBareValue(t *testing.T) {
	got, ok := findAttr(testTs+` level=INFO msg=built duration=1s`, "msg")
	if !ok || got != "built" {
		t.Errorf("findAttr bare = %q, %v; want built, true", got, ok)
	}
}

func TestFindAttrEscapes(t *testing.T) {
	line := testTs + ` level=INFO msg="streaming: processing channel video" url="u" title="say \"hi\" now"`
	ev := ParseLogLine(line)
	if ev.Video != `say "hi" now` {
		t.Errorf("Video = %q, want %q", ev.Video, `say "hi" now`)
	}
}

func TestFindAttrNoFalseSubstringMatch(t *testing.T) {
	// "url=" must not match inside "purl=".
	_, ok := findAttr(`x purl=1 url=2`, "purl")
	if !ok {
		t.Fatal("purl attr not found")
	}
	v, _ := findAttr(`x purl=1 url=2`, "purl")
	if v != "1" {
		t.Errorf("purl = %q, want 1", v)
	}
}

// I3: watch iteration line drives WatchIter.
func TestParseWatchIter(t *testing.T) {
	line := testTs + ` level=INFO msg="watch: iteration 3 starting"`
	ev := ParseLogLine(line)
	if ev.Kind != EventWatchIter {
		t.Fatalf("Kind = %v, want EventWatchIter", ev.Kind)
	}
	if ev.Iter != 3 {
		t.Errorf("Iter = %d, want 3", ev.Iter)
	}
}

// I3: channel fetch line drives queue accounting.
func TestParseFetchedChannel(t *testing.T) {
	line := testTs + ` level=INFO msg="total filtered videos across tabs" count=5`
	ev := ParseLogLine(line)
	if ev.Kind != EventFetched {
		t.Fatalf("Kind = %v, want EventFetched", ev.Kind)
	}
	if ev.Count != 5 {
		t.Errorf("Count = %d, want 5", ev.Count)
	}
}

// I3: playlist fetch line drives queue accounting.
func TestParseFetchedPlaylist(t *testing.T) {
	line := testTs + ` level=INFO msg="fetched playlist videos" url="https://x" name="WL" total=7`
	ev := ParseLogLine(line)
	if ev.Kind != EventFetched {
		t.Fatalf("Kind = %v, want EventFetched", ev.Kind)
	}
	if ev.Count != 7 {
		t.Errorf("Count = %d, want 7", ev.Count)
	}
}
