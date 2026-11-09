# Accelerate channel scan — use external yt-dlp

intent: 2026-11-09-external-yt-dlp-path@9c28969d5c867482ab284a9f66dea2c8f454c56b
charter: 1.1

## Current State Evidence
- **Forward:** 程式預設僅使用 `embedded.ExtractAll()` 從內建 binary 讀取 yt-dlp（位於 `~/.ytss/bin/yt-dlp`），每次處理新影片需啟動兩次（metadata + 字幕），實測約 6–7 秒/次。
- **Reverse:** 系統 PATH 中存在同版本（2026.10.26）的 `yt-dlp` 時，實測僅 0.17–0.32 秒/次；存在但無法執行時應明確回退內建版本。
- **Error:** 若使用者手動指定無效 yt-dlp 路徑，應明確說明回退原因，不靜默失敗；無設定檔時預設優先系統 yt-dlp、找不到再回退內建。
- **Data:** `embedded/embedded.go` 的 `ExtractAll()` 負責從 Go `embed` FS 讀取並寫入 `~/.ytss/bin/`；`fetcher/fetcher.go` 的 `NewFetcher` 目前唯一使用 `f.runYtDlp`，無預設檢查 PATH。
- **Boundary:** `config.yaml` 結構未變更；TUI 設定表格為動態解析 YAML，無新增固定鍵；clip 邊界為「不新增設定鍵」→行為全靠 PATH 偵測與回退機制。

## Task DAG
**W1-01 — 切換 yt-dlp 來源（系統優先、無則回退內建）**  after: acceptance: 1,2,3,4,5
- **Files:** `fetcher/fetcher.go`, `pipeline/pipeline.go`（僅讀取 YtDlp path），`embedded/embedded.go`（保留回退）
- **Test:** A1 positive: 有系統 yt-dlp → 使用系統版；A2 positive: 無系統 yt-dlp → 使用內建；A3 positive: 系統 yt-dlp 無法執行 → 回退並記錄原因；A4 positive: 日誌顯示來源與版本；A5 positive: 單支新影片抓取延遲由 6–7 秒降至 <1 秒。
- **Risk:** 1. 系統 yt-dlp 版本舊於內建時的行為差異（實測版本相同 2026.10.26，無差異）。2. 無系統 yt-dlp 時的回退路徑必須正確不遺漏任何影片。3. 系統 yt-dlp 存在但無法執行（權限、相容性）時的優雅降級。

## Questions asked
① — what — 複雜度檢查顯示原提案比需要的大：是否改用零設定（不加設定鍵），一律優先系統 yt-dlp、找不到回退內建？（使用者選擇 A）
① — what — 使用者於決策點①接受的方案描述：不加設定鍵；一律優先系統 yt-dlp，找不到回退內建

## Risks
1. 系統 yt-dlp 與內建版本行為差異（若版本不匹配，可能影響抓取結果）——實測版本相同 2026.10.26，風險極低。
2. 無系統 yt-dlp 時的回退機制必須正確：程式必須偵測 PATH 中的 yt-dlp 是否可執行，否則回退內建；遺漏回 fallback 會導致 yt-dlp 命令失敗。
3. 執行日誌未記錄使用的 yt-dlp 來源與版本，違背「Never fail silently」原則。

## Acceptance (mapped to intent)
1. 有系統 yt-dlp → 程式使用系統 yt-dlp 完成抓取，產出一致。
2. 無系統 yt-dlp → 程式自動回退內建 yt-dlp，行為一致。
3. 系統 yt-dlp 存在但無法執行 → 程式回退使用內建 yt-dlp，並於日誌說明原因，不靜默失敗。
4. 執行日誌會記錄本次使用的 yt-dlp 來源（系統或內建）與版本。
5. 單支新影片的抓取啟動延遲由每次約 6–7 秒降至每次低於 1 秒（乾淨環境實測對比）。