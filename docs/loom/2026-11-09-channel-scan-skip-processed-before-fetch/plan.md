# Channel scan skip processed before fetch — plan (SUPERSEDED)

> Status: withdrawn 2026-11-09. The plan-lens review proved the planned mechanism
> is dead code: `processChannelVideos` (pipeline/pipeline.go:560-568) already skips
> terminal videos by id before `ProcessVideo` and before any full-metadata fetch.
> All four Acceptance lines already hold on HEAD. Kept as investigation evidence for
> the follow-up "accelerate channel scan" intent.

intent: 2026-11-09-channel-scan-skip-processed-before-fetch@37b4dbae0f4efb246432bd0761dfb6e14da50b6d
spec: docs/loom/2026-11-09-channel-scan-skip-processed-before-fetch/spec.md@6982d93
charter: 1.1

## Task DAG

**W0-01 — Early skip before full-metadata fetch in ProcessVideo**  after:   acceptance: 1,2,3,4
- Files: `pipeline/pipeline.go`, `pipeline/pipeline_test.go`
- Test: A1 positive: processed-only scan makes zero fetch calls; negative: unindexed video fetches once. A2 positive: new video completes summary; negative: fetch failure continues. A3 boundary: renamed handle skips post-fetch, no duplicate. A4 positive: playlist tests stay green; negative: playlist fetches.
- Risk: reorders an existing check; default keeps the post-fetch fallback so no video can regress to duplicate output; spec Design decision 1-5; skips are observable via existing log wording.

## Simplicity check

- Delete the W0-01 mechanism entirely — taken: the plan-lens reviewer proved `pipeline/pipeline.go:560-568` already skips terminal videos by id before `ProcessVideo`; an empirical run against the real `ytss-output` index reported `listed=10 terminal=10 → skipped=10 metadata calls=0`. All four Acceptance lines already hold on HEAD, so the planned early skip would be unreachable code.
- Test-only pinning task — declined: it would preserve an intent whose Problem statement is factually wrong (already-processed videos are not re-fetched today); the follow-up intent records the real bottlenecks instead.

## Questions asked
① — what — 你要的是：掃描頻道時，先跳過已經處理過的影片，不再對每一支影片都花一次 yt-dlp 抓完整資料。做完後你可以：快速掃描/dry-run、新影片照常完整處理、頻道改 handle 也不出錯。對嗎？
① — consequence — 回覆「對」等於授權 closing-review 與發佈檢查通過後自動 push 分支並開 Ready PR（merge 仍是另一個獨立決定）；你也可以此時明確說不要自動發佈。
① — what — 你已同意的執行細節（方案 A）：判斷主鍵是 flat-playlist 回傳的影片 id；handle 從 config 網址取得；檢查語意沿用 summary.md/.skipped；前置沒命中就照現狀補抓、後置檢查保底；不動 yt-dlp 參數、config、輸出結構與 playlist 路徑。對嗎？

## Risks
1. Withdrawn: the confirmed premise (processed videos re-fetch metadata) was wrong; the real costs are per-new-video metadata fetches, per-tab listing startup, and dry-run fetching metadata it does not need.
2. Handle mismatch (renamed channel) and non-`@` URL forms are moot for this plan; they stay relevant to the follow-up intent only if it touches handle scoping.
3. Second vendor notice: none (second-vendor: suggest mode, non-blocking).
