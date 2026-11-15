# Classify members-only metadata failures as skips, not errors

originator: kouko
kind: engineering
needs-design: no — reclassifies one existing failure bucket for logging and counters only; no persisted state, no new user-facing surface, no multi-state flow
status: confirmed 2026-11-15
intent_confirmed_sha: ee9cff018b5fb31999e09415051e1b4b245aa6ed
publication: automatic — authorized 2026-11-15 by kouko

## Problem

Videos restricted to channel members (e.g. 老李玩錢's "小君" tier) cannot be fetched with the current account: yt-dlp returns "This video is available to this channel's members … Join this channel …" both with and without browser cookies (verified: the configured Chrome account holds no membership). The pipeline treats this as an ordinary failure, so every scan logs `video processing failed` at ERROR level and adds the video to the failed count.

Observed impact (ytss-output/logs/ytss.log, 2026-11-15): 5 members-only videos from one channel produced hundreds of identical ERROR lines in a single day (video `eYOcdhR-72c` alone failed 190 times). Real failures are buried in that noise and the failure counters are meaningless.

The video must keep being re-checked on every scan: the user's token may be temporarily expired, and the user may subscribe to the channel later, in which case the next scan should process it. So no persisted marker and no cool-down are wanted.

## Proposed outcome

A metadata fetch that fails because the video is members-only is reported as a skip for that scan instead of a failure: it is logged at a non-error level naming the reason, and it does not count as failed. The video is left unprocessed, so the next scan attempts it again and processes it normally as soon as access works.

## Acceptance

1. A members-only metadata failure produces a non-ERROR log line naming the reason (members-only) and is not counted in the scan's failed total.
2. No marker or other persistent state is written for the video; the next scan attempts it again (visible in logs).
3. If a later attempt succeeds, the video is processed normally with no special handling.
4. Any other metadata failure class (network error, video unavailable, channel gone) still logs ERROR and still counts as failed.
5. Unit tests cover: members-only failure classified as skip (not failed, no marker file written), other failure classes still classified as failed.
6. Real scenario: after rebuild, the five currently failing members-only videos produce no ERROR lines and no failed-count increase in the next scan.

## Constraints

- Every scan still re-attempts the video; no skip marker, no cool-down, no persisted state.
- Do not change retry behaviour or classification for any other failure class.
- No new configuration options.
- Do not alter `.skipped` ranking semantics (summary.md above .skipped) established in the index.

## Out of scope

- Accessing members-only content (no membership purchase, no cookie/profile changes).
- Reducing the yt-dlp time spent on each re-attempt.
- Other failure classes (private/deleted videos, geo-blocked, channel 404s) — add later if observed.
- Changes to retry counts or the retry budget of ordinary failures.

## Open questions

- none
