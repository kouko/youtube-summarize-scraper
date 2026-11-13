# Fix video index losing to empty older duplicate directory

originator: kouko
kind: engineering
needs-design: no — 內部索引資料結構行為變更；不改變輸出格式、CLI 或使用者可見表面；既有測試已涵蓋 BuildIndex 各情境，此為修正重複資料夾情境下的選擇規則
status: confirmed 2026-11-13
intent_confirmed_sha: 91e713c2900394c61e5a9b3bb23ccabdade48c0c
publication: automatic — authorized 2026-11-13 by kouko

## Problem
當同一支 video ID 存在多個輸出資料夾（例如舊的失敗嘗試、重新跑過的成品）時，`output/index.go` 的 `BuildIndex` 按資料夾名稱排序並逐個覆寫 `idx.entries[videoID]`，導致「排序最後」的資料夾成為該影片的索引代表。若該資料夾是空的（沒有 summary.md），則 `FindVideoDir` 回傳它 → `HasFile("summary.md")` 為 false → `processPlaylistVideos` 判定未處理，於是**每一輪掃描都重新完整處理同一支影片**（字幕→轉錄→LLM 摘要），並再產生一個新的重複資料夾。

實測影響：播放清單影片 `xJ__cqXlboc`（平面設計紀錄片 #47）在 11/13 當天已被完整重跑 **12 次**，庫中累積 22 個重複資料夾；排序最後的代表是一個完全空殼的舊資料夾，永遠無法被跳過。全庫目前約 94 支影片存在重複資料夾，屬於同類風險。

## Proposed outcome
`BuildIndex` 在面對同一 video ID 的多個資料夾時，優先選擇**已完整處理（有 summary.md）**的那一個作為索引代表；若都沒有 summary，再依固定規則（如選最新日期）做決定，不讓空殼資料夾佔位。已處理過的影片無論有無重複資料夾都能被正確識別並跳過，不再重複跑 LLM。

## Acceptance
1. 建立同一 video ID 的兩個資料夾：一個有 summary.md（完整處理）、一個是空殼；`BuildIndex` 之後 `FindVideoDir` 回傳有 summary 的那個，`IsProcessed` 回傳 true，掃描判定為已處理並跳過。
2. 當該 video ID 的所有重複資料夾都沒有 summary.md 時，`BuildIndex` 仍回傳一個穩定、可預測的資料夾（不會每次都跳回空殼），行為以測試鎖定。
3. 真實情境：庫中現有的 `xJ__cqXlboc`（22 個重複資料夾，其中 `2021-11-13` 那一個有 summary.md）在下一輪掃描被判定為已處理並跳過，不再產生新的重複資料夾。
4. 既有測試全部通過，且一般單一資料夾（無重複）的行為維持不變。

## Constraints
- 不改變輸出資料夾命名格式（仍為 `DATE__videoID__title`）。
- 不刪除使用者現有的任何重複資料夾；只改變索引選擇規則。
- `BuildIndex` 的公開簽名不變，呼叫端不需要修改。

## Out of scope
- 自動清理或合併現有的 94 支影片的重複資料夾（本次不動使用者資料）。
- 修正其他重複處理原因（例如日期漂移造成的新資料夾）；本次只解決索引選擇。
- 變更 `IsProcessed`、`FindVideoDir` 的呼叫端邏輯。

## Open questions
- none
