# Use external yt-dlp to cut per-video startup cost

originator: kouko
kind: product
needs-design: no — config 表面已由既有設計文件 docs/loom/specs/ytss-design.md 的 Config File 章節涵蓋；本變更不新增設定鍵，TUI 設定表格不需改動；行為非多狀態（單一解析規則：外部 yt-dlp 優先、內建回退）
evidence: [docs/loom/specs/ytss-design.md, embedded/embedded.go, fetcher/fetcher.go, ytss-output/logs/ytss.log]
status: confirmed 2026-11-09
intent_confirmed_sha: 9c28969d5c867482ab284a9f66dea2c8f454c56b
publication: automatic — authorized 2026-11-09 by kouko

## Problem
每次處理一部新影片時，程式要啟動兩次內建的抓取工具 yt-dlp（一次取影片資訊、一次取字幕）。內建版本是以 PyInstaller 打包的二進位檔，每次啟動實測約 6–7 秒，等於每部新影片固定多付約 13 秒的啟動等待。受影響的是在本機長時間批次執行 ytss 的使用者：這些固定等待在每次執行中累積。使用者機器上已有同版本（2026.10.26）的系統安裝 yt-dlp，啟動實測僅 0.17–0.32 秒，但目前程式寫死只使用內建的那一份，沒有任何方式改用外部版本。

## Proposed outcome
ytss 優先使用系統上可用的 yt-dlp（PATH 中可找到且可執行），找不到時自動回退內建版本；不新增任何設定鍵。改用外部 yt-dlp 後，每部新影片的抓取啟動成本從約 13 秒降到低於 1 秒，且抓取結果與現狀一致；內嵌版本保留為回退，不移除。

## Acceptance
1. 在系統上有可用 yt-dlp 的乾淨環境執行一部新影片的處理，程式使用系統的 yt-dlp 完成影片資訊與字幕抓取，產出與使用內建版本時一致。
2. 在系統上沒有 yt-dlp 的環境執行同一流程，程式自動回退使用內建 yt-dlp，行為與現狀一致。
3. 系統上的 yt-dlp 存在但無法執行時，程式回退使用內建 yt-dlp，並在執行日誌說明回退原因，不靜默失敗。
4. 執行日誌會記錄本次使用的 yt-dlp 來源（系統或內建）與版本。
5. 對單部新影片，抓取階段的工具啟動延遲由每次約 6–7 秒降至每次低於 1 秒（乾淨環境實測對比）。

## Constraints
- 不新增設定鍵、不改變現有 YAML 設定檔結構（使用者已確認的最小方案 A）。
- 內嵌 yt-dlp 保留為回退機制，不移除 embedded 套件。
- 不改變輸出檔案格式、資料夾結構、CLI 參數與 playlist 路徑行為。
- yt-dlp 的呼叫參數（metadata、字幕下載）與現狀一致。
- 沿用產品原則「Never fail silently」：使用的 yt-dlp 來源與版本在啟動日誌明確記錄。

## Value case
- 受益者：在本機批次執行 ytss 的使用者（目前每次執行處理數部新影片，每部固定多付約 13 秒啟動等待）。
- 緊急性：中等——不影響正確性，只影響等待時間；使用者每天長時間執行，累積效果明顯。
- 現有替代：目前無任何方式切換 yt-dlp 來源，只能忍受內建版本的啟動成本。
- 排擠的工作：無重大排擠。
- GO/NO-GO：GO — 使用者機器上實測同版本系統 yt-dlp 啟動 0.17–0.32 秒（內建 6.3–7.3 秒），差異明確、可驗證、低風險。

## Out of scope
- 影片層並行處理（前述方案 A）。
- 摘要與關鍵字階段的 LLM 加速（前述方案 C/D）。
- yt-dlp 路徑的設定鍵（使用者已選擇不加；若日後需要指定特定路徑再另開 intent）。
- 重新打包內嵌 yt-dlp 以支援 PyInstaller 解壓快取（原方案 B）。
- 跨次執行的 metadata 快取（--write-info-json）。
- 清理使用者系統上的 PyInstaller 暫存資料夾。

## Open questions
- none
