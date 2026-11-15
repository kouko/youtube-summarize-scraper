# Widen per-video metadata fetch timeout

originator: kouko
kind: engineering
needs-design: no — 內部逾時常數調整；不改變輸出格式、CLI 或使用者可見表面；無多狀態行為需要 spec
status: confirmed 2026-11-15
intent_confirmed_sha: ee18608be154e94bb91f2dc901b9d46f7df6f821
publication: automatic — authorized 2026-11-15 by kouko

## Problem

YouTube 近期對 ytss 的抓取請求節流，單支影片的完整 metadata 抓取（`yt-dlp --dump-json --no-download watch?v=...`）實測需約 26 秒（正常 1–3 秒），負載高時常超過 ytss 內建的 60 秒逾時，逾時一到 yt-dlp 子程序直接被 SIGKILL（log 顯示 `signal: killed`），影片重試 5 次全滅後被標記失敗。

實際影響（ytss-output/logs/ytss.log）：
- 11-14 有 427 次失敗、11-15 已有 106 次失敗，**全部**為同一原因 `full metadata fetch failed and no channel info available`
- 每日處理完成量從高峰 154 支（11-10）跌到 50（11-14）、20（11-15，至 15:45）
- 受影響的是所有需要完整 metadata 才能處理的影片，不只特定頻道

## Proposed outcome

單支影片的 metadata 抓取在節流造成的延遲下不再被逾時誤殺：逾時放寬到足以涵蓋節流期間的實際耗時，使原本被判失敗的影片能正常進入處理流程，處理完成量恢復。

## Acceptance

1. `fetcher` 套件的 metadata 逾時值放寬為 180 秒，且有一個測試鎖定該值 ≥ 120 秒（防止日後被縮回緊繃值而重蹈覆轍）。
2. 同一支影片在逾時放寬後不會再因為節流延遲（>60 秒但 <180 秒的抓取）被判失敗；以單元測試模擬慢速抓取驗證逾時邏輯生效（測試不等待 180 秒，驗證逾時值本身）。
3. 既有測試全部通過（`go test ./...`）。
4. 真實情境：重新編譯後，原本 `full metadata fetch failed` 的影片在下一輪掃描能取得 metadata 並進入處理，不再整批失敗。

## Constraints

- 不改變失敗後的重試次數（維持 5 次）、字幕/轉錄/LLM 各階段的逾時或 provider 選擇行為。
- 不改變任何使用者可見表面（CLI/TUI 參數、輸出格式、資料夾命名）。
- 不刪除或修改 ytss-output/ 下任何現有資料。

## Out of scope

- 修復 LLM 摘要 provider 的失效問題（qwen-code 免費層已終止、claude-code 登入到期）——由使用者另行處理。
- 調整掃描頻率、請求間隔或 YouTube 節流本身的策略。
- 把逾時值做成可設定參數（本次只放寬常數，不加新設定面）。

## Open questions
- none
