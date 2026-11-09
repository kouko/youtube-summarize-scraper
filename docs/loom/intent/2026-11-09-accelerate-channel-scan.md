# Accelerate channel scan — skip processed before metadata fetch
originator: kouko
kind: engineering
needs-design: no — 僅在 ProcessVideo 最開頭加入已處理前置檢查 + dry-run 跳過 metadata fetch；無新 CLI、config 結構、輸出檔案格式或 TUI 變更；既有設計文件 docs/loom/specs/ytss-design.md 已涵蓋管線行為
evidence: [pipeline/pipeline.go, output/index.go, docs/loom/specs/ytss-design.md]
status: confirmed 2026-11-09
intent_confirmed_sha: 748ba9ba00d66b0d60f3413a5b42501f93b9ead1
publication: automatic — authorized 2026-11-09 by kouko

## Problem
掃描頻道時，即使影片已經處理過（輸出資料夾已有 summary.md 或 .skipped），程式仍在 ProcessVideo 裡進行完整的 metadata fetch（yt-dlp --dump-json），才進行已處理的判斷；dry-run 同樣會對每一部影片都發出完整的 metadata 抓取呼叫，導致執行時間不必要地拉長。受影響的是使用者本人：每次掃描或 dry-run 都會浪費時間去抓已處理影片的完整資料，且新影片的 metadata fetch 也沒有任何快取或檢查機制。

## Proposed outcome
掃描頻道時，在 ProcessVideo 最開頭先用影片 id 與該頻道的 handle 判斷影片是否已處理（index 裡有 summary.md 或 .skipped 且目錄路徑符合 `/@{handle}/`）；已處理的直接回報為 skipped、不再發出 yt-dlp 完整 metadata 抓取呼叫；dry-run 模式同樣只在最開頭檢查一次，不對任何影片進行 metadata fetch；未處理的影片則完整進行 metadata、subtitle、transcription 與摘要產生。

## Acceptance
1. 對一個清單內影片全部已處理的頻道做掃描或 dry-run，整個過程不對任何影片發出逐支的完整 metadata 抓取呼叫（執行日誌中不出現逐支的「fetching full metadata」），且所有影片回報為已跳過。
2. 掃描中仍存在的（未處理的）新影片，行為與現狀一致：補抓完整 metadata 後照現有流程產出字幕、逐字稿與摘要。
3. dry-run 完成後不產生任何 yt-dlp 的完整 metadata fetch 紀錄，且輸出只顯示「would process (dry run)」而不額外進行任何抓取。
4. 頻道的 handle 與輸出資料夾名稱不一致時（例如頻道改過 handle），不出錯、不重複產出：最壞情況退回現狀流程（檢查失敗 → 照現狀補抓 → 後置檢查保底）。

## Constraints
- 前置檢查的 channel handle 固定來自被掃描頻道的 config 網址（使用者已確認的 A 方案），不來自影片清單資料。
- 前置檢查語意沿用現有 IsProcessed：該頻道資料夾下有 summary.md 或 .skipped 且目錄路徑符合 `/@{handle}/` 才算已處理。
- 保留既有「補抓後」的檢查作為保底；前置檢查未命中時一律照現狀補抓。
- 不改動 yt-dlp 呼叫參數、embedded binary、config schema（不新增欄位）、輸出資料夾結構或 TUI 介面。
- 僅在 pipeline/pipeline.go ProcessVideo 函數最開頭加入條件判斷，其他檔案皆不動。

## Value case
beneficiary: 定期掃描多頻道的使用者（使用者本人）。
urgency: 高——使用者明確反映掃描太慢、dry-run 曾超過 3 分鐘完成不了；環境修復（清 _MEI*）已生效，9.6s→啟動成本更底下去。
existing alternative: 等待；或縮小 count 減少掃描量。
displaced work: 無。
GO — 改動僅為 ProcessVideo 最開頭的條件判斷（兩點：dry-run 跳過 + 已處理前置檢查），效益直接（dry-run 從「幾分鐘」變成「秒級」；已處理影片不再重複跑 metadata）。

## Out of scope
- S3 homebrew yt-dlp path 覆寫（另開 intent）。
- R1 並行 metadata 抓取、R2 --write-info-json、R3 --extractor-args player_skip。
- 清理系統暫存目錄的 PyInstaller 殘留（環境問題，與本變更分開處理）。

## Open questions
- none