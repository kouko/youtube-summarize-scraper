# Accelerate channel scan — plan
intent: 2026-11-09-accelerate-channel-scan@ddaac35fe35e14dde2bb73888aec8e121950d975
charter: 1.1

## Current State Evidence
- Forward: `pipeline/pipeline.go:614-650` ProcessVideo fetches full metadata (lines 620-637) before the post-fetch IsProcessed check (lines 643-648) that returns errSkipped.
- Reverse: `pipeline/pipeline.go:560-568` processChannelVideos already skips terminal videos by id before calling ProcessVideo, but only for channel scans; the dry-run and ytss video paths go straight to ProcessVideo.
- Error: `pipeline/pipeline.go:625-633` a failed full-metadata fetch restores approximate dates and continues with partial data.
- Data: `output/index.go:122-133` IsProcessed(handle, videoID) requires summary.md or .skipped plus a `/@<handle>/` path prefix; the index is keyed by videoID from `DATE__videoID__title` directories.
- Boundary: `fetcher/types.go:4-22` flat-playlist entries carry id/title/duration/approximate upload_date only — no uploader_id, tags or language.

## Task DAG

**W0-01 — Early skip in ProcessVideo for dry-run and already-processed**  after:   acceptance: 1,2,3,4
- Files: `pipeline/pipeline.go`, `pipeline/pipeline_test.go`
- Test: A1 positive: zero fetch; negative: unindexed fetches once. A2 positive: summary done; negative: fetch failure continues. A3 positive: dry-run zero fetch; negative: lists without fetch. A4 positive: skip; boundary: renamed-handle fetches once post-fetch skips.
- Risk: reorders an existing check; default keeps the post-fetch fallback so no video can regress to duplicate output; spec REQ-1, REQ-3.

## Simplicity check
- none found

## Questions asked
① — what — 你要的是：掃描頻道時，dry-run 快速列出影片而不抓完整 metadata；已處理影片在 ProcessVideo 最開頭就判斷跳過，不再跑完整的 yt-dlp metadata fetch；新影片照常完整處理。做完後你可以：dry-run 秒級完成、掃描時已處理影片不再重複跑 metadata、新影片照常完整處理、頻道改 handle 退回現狀不出錯。對嗎？
① — consequence — 回覆「對」等於授權 closing-review 與發佈檢查通過後自動 push 分支並開 Ready PR（merge 仍是另一個獨立決定）；你也可以此時明確說不要自動發佈。
① — what — 你已同意的執行細節（S1+S2）：dry-run 跳過 metadata fetch；已處理前置檢查用 config handle + video id 查 index；handle 從 config URL 解析；檢查語意沿用 IsProcessed；前置失敗退回現狀補抓後置檢查；不動 yt-dlp 參數、embedded binary、config schema、輸出結構、TUI、playlist 路徑。對嗎？

## Risks
1. Handle mismatch (renamed channel) makes the early check miss, costing one fetch per known video — bounded by the untouched post-fetch fallback; intent REQ-4.
2. URL forms without `@handle` (`/channel/UC…`, `/c/…`, `/user/…`) skip the early check entirely — those channels keep today's behavior, no correctness change.
3. Second vendor notice: none (second-vendor: suggest mode, non-blocking).
