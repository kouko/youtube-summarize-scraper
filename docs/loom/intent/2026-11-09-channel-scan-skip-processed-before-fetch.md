# 頻道掃描：已處理影片先跳過，不再逐支補抓完整 metadata
originator: kouko
kind: engineering
needs-design: no — 不改變任何 CLI 參數、config 結構、輸出檔案格式或 TUI 介面；只調整內部檢查順序（先查已處理、後補 metadata），既有設計文件 docs/loom/specs/ytss-design.md 已涵蓋掃描與管線行為
evidence: [fetcher/types.go, fetcher/fetcher.go, pipeline/pipeline.go, output/index.go, docs/loom/specs/ytss-design.md]
status: withdrawn — 前提與現況程式行為不符：頻道掃描迴圈（pipeline/pipeline.go:560-568）已在補抓前按影片 id 跳過已處理影片，四條驗收標準於現況已成立；改以「加速掃描」新 intent 處理實際瓶頸（新影片逐支補抓、清單啟動成本、dry-run 補抓）
intent_confirmed_sha: 37b4dbae0f4efb246432bd0761dfb6e14da50b6d
publication: automatic — authorized 2026-11-09 by kouko

## Problem
掃描頻道影片時，即使影片早已處理完成（輸出資料夾已有摘要或 .skipped 標記），程式仍對清單中的每一部影片另外啟動一次 yt-dlp 抓取完整 metadata，之後才判斷可否跳過。這使得「只是確認有沒有新影片」的掃描（含 dry-run）要等數分鐘；影片數越多、新影片越少，等待越沒有意義。受影響的是使用者本人：執行掃描或 dry-run 時的等待時間被浪費，也拖慢排程批次。

## Proposed outcome
掃描頻道時，在補抓完整 metadata 之前，先用清單回傳的影片 id 與該頻道的 handle 判斷影片是否已處理；已處理的直接跳過、不再發出 yt-dlp 抓取。未處理的影片才補抓 metadata 並照現有流程處理。

## Acceptance
1. 對一個清單內影片全部已處理的頻道掃描（含 --dry-run），整個過程不對任何影片發出逐支的完整 metadata 抓取呼叫（執行日誌中不出現逐支的「fetching full metadata」），且所有影片回報為已跳過。
2. 掃描中仍存在的（未處理的）新影片，行為與現狀一致：補抓完整 metadata 後照現有流程產出字幕、逐字稿與摘要。
3. 頻道的 handle 與輸出資料夾名稱不一致時（例如頻道改過 handle），不出錯、不重複產出：最壞情況退回現狀流程（補抓後由既有檢查正確跳過）。
4. playlist 掃描路徑的行為與訊息維持不變。

## Constraints
- 前置跳過檢查的 channel handle 固定來自被掃描頻道的 config 網址（使用者已確認的 A 方案），不來自影片清單資料（清單項不帶 uploader_id）。
- 前置檢查語意沿用現有 IsProcessed：該頻道資料夾下有 summary.md 或 .skipped 才算已處理。
- 保留既有「補抓後」的檢查作為保底；前置檢查未命中時一律照現狀補抓。
- 不改動 yt-dlp 呼叫參數、embedded binary、config schema 或輸出資料夾結構。

## Value case
beneficiary: 定期掃描多頻道的使用者（使用者本人）。
urgency: 高——使用者明確反映掃描太慢，dry-run 曾超過 3 分鐘完成不了。
existing alternative: 等待；或縮小 count 減少掃描量。
displaced work: 無。
GO — 改動僅為檢查順序重排，效益直接（重複呼叫 yt-dlp 次數從 N 降到接近 0）。

## Out of scope
- R1 並行 metadata 抓取。
- R2 以 --write-info-json 在字幕下載階段順帶取得 metadata。
- R3 yt-dlp --extractor-args player_skip 加速。
- 改用系統（如 homebrew）yt-dlp 或採用 YouTube Data API。
- 清理系統暫存目錄的 PyInstaller 殘留（環境問題，與本變更分開處理）。

## Open questions
- none
