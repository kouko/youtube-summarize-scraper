# Cross-channel duplicate priority and .skipped vs summary ranking

originator: kouko
kind: engineering
needs-design: no — 索引查詢邏輯變更；不改變輸出格式、CLI 或使用者可見表面；現有單一-ID 選優已在 W1-01 實作；此工作僅擴大至跨頻道與 .skipped vs summary 排序
status: confirmed 2026-11-14
intent_confirmed_sha: 41ebc2f2febeca589c2c16afb7bdcdf226b09c65
publication: automatic — authorized 2026-11-14 by kouko

## Problem
目前 `BuildIndex` 僅保留單一 `entries[videoID]`，導致跨頻道情境下的以下問題：

1. **跨頻道 tie**：同一支 video ID 在兩個不同頻道各有完成的 `summary.md`，索引只能保留其中一個（first-wins tie-break），另一個頻道的 `IsProcessed` 永遠回傳 `false`，導致該頻道的影片每輪都會被重新完整處理（字幕→轉錄→LLM），且永遠不會收斂。

2. **`.skipped` 遮蔽 `summary.md`**：同一頻道同時存在 `.skipped` 與 `summary.md` 時，兩者皆得分 2，視排序而定。如果 `.skipped` 排序在前，`HasFile(id,"summary.md)` 會變成 `false` 雖然實體資料夾裡確實有摘要。雖然 `isSkipMarked` 會短路不進行 LLM，但系統仍會誤判狀態。

**影響人群**：任何同時管理多個頻道、或有 `.skipped` 標記與完成摘要共存的使用者。目前的行為可能導致部分需要的影片被誤判為「未處理」或「永遠無法被跳過」。

## Proposed outcome
- `BuildIndex` 在面對跨頻道相同 video ID 時，為了 `IsProcessed` 的正確性，保留該頻道特有的候選紀錄，讓 `IsProcessed(channelHandle, id)` 在該頻道內任一完成摘要即回傳 true。
- 同一頻道內 `.skipped` 與 `summary.md` 時，`summary.md` 優先級更高（分數 3 > 2），確保 `HasFile(id,"summary.md)` 一律回傳 true 當存在 completed summary 時，`.skipped` 僅在真正無摘要時才佔優。
- 維持 W1-01 的「first-wins tie-break」原則，避免無限期重複資料夾成長。

## Acceptance
1. **跨頻道**：同一 video ID 在 @aaa 與 @bbb 兩個頻道各有 `summary.md`；`IsProcessed("@aaa", id)` true 且 `IsProcessed("@bbb", id)` true。
2. **`.skipped` vs summary**：同一頻道同時存在 `.skipped` 與 `summary.md` 時，`HasFile(id,"summary.md")` 回傳 true（summary 優先），`IsProcessed` 回傳 true。
3. **單一資料夾不受影響**：僅有一個資料夾的情況，行為完全不變。
4. **全套測試通過**：`go test ./...` 全程綠燈。

## Constraints
- 不改變輸出資料夾命名格式（仍為 `DATE__videoID__title`）。
- 不刪除或修改任何 `ytss-output/` 現有資料夾（使用者資料不動）。
- `BuildIndex` 的公開簽名與 `VideoIndex` 的結構變更必須最小化：盡量複用已有的 `entryScore` 與選擇規則結構。
- 動態 `Add`/`AddFile`：不得破壞現有的 resume 與 resume 檢查路徑。

## Out of scope
- 自動清理或合併現有 94 支影片的跨頻道重複資料夾（本次不動使用者資料）。
- 變更 `IsProcessed`/`FindVideoDir`/`HasFile` 的呼叫端邏輯（只在索引層面調整選擇規則）。
- 處理 `.skipped` 僅標記但無任何實體檔案的極端情境。

## Open questions
- none