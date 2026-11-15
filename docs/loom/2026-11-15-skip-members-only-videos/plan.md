# Classify members-only metadata failures as skips, not errors — plan

intent: 2026-11-15-skip-members-only-videos@ee9cff018b5fb31999e09415051e1b4b245aa6ed
charter: 1.1

## Current State Evidence

- Forward: `pipeline/pipeline.go:645-655` returns a failed error when metadata fetch fails and channel info is absent; `classifyResult` (:1635) maps it to `bucketFailed`, logging ERROR and incrementing `stats.Failed`.
- Reverse: `errSkipped` (:1596) is the sentinel `classifyResult` maps to `bucketSkipped`; callers at :564 and :399 count it silently, with no new stats surface.
- Error: yt-dlp stderr carries two observed signatures: "available to this channel's members on level:" and "Join this channel to get access to members-only content"; `FetchVideoMeta` wraps the final stderr.
- Data: the playlist path (:1116) keeps partial data on metadata failure and is unaffected; `needsCookie`/`cookieArgs` are untouched.
- Boundary: this change writes no marker file; `.skipped` ranking in `output/index.go` and the Whisper gate (:1186) stay unchanged.

## Task DAG

**W1-01 Classify members-only metadata failure as skip**  after:  acceptance: 1,2,3,4,5,6
- Files: `pipeline/pipeline.go`, `pipeline/pipeline_test.go`
- Test: A1 positive: membersOnlyIsSkip; boundary: noMarkerFile. A2 positive: nextScanReattempts; boundary: notCountedFailed. A3 positive: successAfterEarlierFailure; boundary: noSpecialHandlingAfterSuccess. A4 positive: otherFailureStillFails; boundary: otherFailureNotSkipped. A5 positive: bothSignaturesMatch; boundary: nearMissNoMatch. A6 positive: fullSuiteGreen; boundary: fetcherTestsUnchanged.
- Risk: agent-decided — a `pipeline`-local string helper instead of a typed error from `fetcher`, keeping the exported API unchanged; `ProcessVideo` logs INFO with the reason and returns `errSkipped`. No ARCHITECTURE.md exists (standing WARN recorded).

## Simplicity check

- Persisted cool-down marker / `.skipped` state machine — declined: user explicitly chose "re-check every scan", no persisted state
- New stats counter or provider surface — declined: `errSkipped` already buckets the outcome
- One helper plus one return-site change in `ProcessVideo` (this plan) — taken

## Questions asked

① — what — 這些會員限定影片（老李玩錢頻道）你沒有要處理吧？——的回答：沒訂閱，標 .skipped (2026-11-15)
① — what — 會員限定影片改為永久 .skipped 的修復方案確認可以嗎？——的回答：我不想要永久 skip 因為有可能是當前 token 暫時失效 也有可能之後我會開始訂閱該頻道 (2026-11-15)
① — what — 會員限定影片失敗後，多久自動重試一次？（其餘驗收內容如前，確認請選此項）——的回答：我覺得就簡單一點 不需要冷卻期 每次碰到都重新確認一次吧 (2026-11-15)
① — what — 會員限定影片改為「非 ERROR、計 skipped、每輪重試」的簡化方案確認可以嗎？——的回答：是，內容正確 (2026-11-15)

## Risks

1. Stderr string matching is the recognition surface; a wording change silently reverts to the pre-change failed behaviour (ERROR plus failed count), never data loss, and tests pin both signatures.
2. Re-checking every scan keeps the current yt-dlp cost per attempt; the user accepted this explicitly in Questions asked.
3. No ARCHITECTURE.md (standing WARN recorded verbatim to the user), so no architecture rule applies.
