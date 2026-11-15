package pipeline

// concern: signature overclaim — the "available to this channel's members"
// recognition surface misses its own wording when the level line carries a
// typographic apostrophe (U+2019, YouTube copy) and yt-dlp does not append the
// hyphenated "Join this channel … members-only" hint, so a members-only video
// silently reverts to the pre-change ERROR/failed noise this change removes.
// A case-insensitive matcher must not be ASCII-quote-sensitive.

import (
	"errors"
	"testing"
)

// TestIsMembersOnlyError_TypographicApostropheLevelOnly_True pins the second
// signature against the typographic-apostrophe form of the same wording: the
// level line without the appended "Join this channel … members-only" sentence,
// with the apostrophe as U+2019. Case-insensitive matching must extend to this
// quote form; a miss reverts members-only videos to the failed classification
// (ERROR log + failed count) — never data loss, but exactly the noise the
// change removes.
func TestIsMembersOnlyError_TypographicApostropheLevelOnly_True(t *testing.T) {
	err := errors.New("ERROR: [youtube] eYOcdhR-72c: This video is available to this channel\u2019s members on level: 小君 (or any higher level).")
	if !isMembersOnlyError(err) {
		t.Errorf("isMembersOnlyError(%q) = false, want true", err.Error())
	}
}
