# Widen per-video metadata fetch timeout — plan

intent: 2026-11-15-widen-metadata-timeout@ee18608be154e94bb91f2dc901b9d46f7df6f821
charter: 1.1

## Current State Evidence
- Forward: `fetcher/fetcher.go:17` `const metadataTimeout = 60 * time.Second`，由 `runYtDlp`（:190）套用於每支影片的完整 metadata 抓取。
- Reverse: `runYtDlpWithTimeout`（:194）以 `exec.CommandContext` 執行 yt-dlp，逾時即 SIGKILL，錯誤訊息呈現 `signal: killed`。
- Error: 實測節流下單支抓取 26 秒；log 顯示 11-14 失敗 427 次、11-15 失敗 106 次，全部 `full metadata fetch failed`，且完成量由 154/日跌至 20/日。
- Data: `fetcher/fetcher_test.go` 覆蓋 FetchChannelTab/FetchPlaylistVideos 與 filter；無 metadataTimeout 相關測試。
- Boundary: 逾時只影響 metadata 抓取路徑；字幕/轉錄/LLM 階段與重試次數（pipeline 內 5 次）不在 fetcher 層。

## Task DAG
**W1-01 Widen metadataTimeout to 180s with regression guard**  after:  acceptance: 1,2,3,4
- Files: `fetcher/fetcher.go`, `fetcher/fetcher_test.go`
- Test: A1 positive: metadataTimeoutAtLeast120s; boundary: below60WouldFail. A2 positive: widenedValueAppliesToRunYtDlp; boundary: shortTimeoutStillKills (fake ytdlp). A3 positive: fullSuiteGreen; boundary: fetcherTestsUnchanged. A4 positive: fullSuiteGreen; boundary: gofmtClean.
- Risk: agent-decided: 只改常數並加地板測試（≥120s），不做成可設定參數（intent Out of scope）；行為測試以 package var 短逾時驗證 kill 邏輯而非等待 180 秒。180 秒選擇：實測 26 秒 ×6 餘裕，遠低於重試浪費的成本。

## Simplicity check
- 把逾時做成設定檔參數讓使用者自行調整 — declined: intent Out of scope 明確排除新增設定面
- 只放寬常數並以測試鎖定（本方案） — taken

## Questions asked
① — what — 這個修復方案（metadata 逾時 60s → 180s）確認可以嗎？——的回答：是，內容正確（2026-11-15）

## Risks
1. 行為面測試以 package-level var 覆寫逾時值驗證 kill 邏輯；常數本身以地板測試鎖定。兩者合計鎖住「值放寬」與「逾時邏輯生效」。
2. 180 秒偏長的取捨：若 YouTube 完全封鎖（>180 秒）仍會失敗，但那是另一類問題（節流策略，Out of scope）。
3. 無 ARCHITECTURE.md（standing WARN 已記錄），無架構規則適用。
