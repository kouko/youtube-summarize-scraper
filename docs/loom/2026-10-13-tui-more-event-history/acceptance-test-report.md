# 2026-10-13 TUI 事件面板保留更多歷史記錄 — acceptance test report

Tried on 2026-10-13, in a clean copy of the project at c6b8b41 (intent confirmed). How I tried each line and what came back: `docs/loom/2026-10-13-tui-more-event-history/evidence/acceptance-test-evidence.md`.

## 你要的東西，逐條對照

| # | 你要的東西 | 結果 | 實際發生什麼 | 重測 |
|---|---|---|---|---|
| 1 | 產生超過 100 行 log 後，往上捲仍看得到第 100 行以前的記錄 | works | 狀態層測試 `TestRecentEventsRetainBeyondOldLimit` 驗證 150 行全數保留；對抗測試 `TestAdversaryEventsPanelScrollsBeyondOld100Limit` 驗證可透過 TUI 介面捲動看到超過舊上限的記錄（如 event-049）。 | — |
| 2 | 可回看的記錄有固定上限約 1000 行（約現在的 10 倍），超過就丟最舊的，不會無限成長 | works | 狀態層測試 `TestRecentEventsCapDropsOldest` 驗證恰好 1000 行時保留完整，1001 行時捨棄最舊；對抗測試 `TestAdventEventsPanelScrollsOverNew1000Cap` 驗證 1100 行時最舊 100 行被捨棄，最舊可見為 event-0100。 | — |
| 3 | 新記錄進來時面板照樣貼底，除非你已經往上捲 | works | 現有測試 `TestModelViewportKeyboardScroll`、`TestModelWheelScrollsPanelUnderPointer`；新增迴歸測試 `TestEventsNewEventsStayPinned`、`TestEventsScrolledUpDoesNotJump`；對抗測試 `TestAdversaryEventsPinnedBottomRapidNewEvents`、`TestAdversaryEventsScrolledUpManyNewEventsStaysPut`、`TestAdversaryEventsScrolledToTopNewEventStays`、`TestAdversaryEventsScrolledUpThenDownToBottomRepins`。 | — |

## 對你既有的資料做了什麼

沒有。此變更僅調整 TUI 事件面板的記憶體保留上限（100 → 1000 行）與對應的 EventBridge 佇列容量。程式不會讀取或修改使用者現有的設定檔、輸出目錄、快取或任何持久化資料；記憶體緩衝在 TUI 離開時自動釋放。

## 我替你決定的事

- **採用單一常數 `MaxRecentEvents = 1000`** — 同時用於 `AppState` 預分配/上限與 `EventBridge` 佇列容量，確保兩者一致，避免傳輸層成為瓶頸。常數置於 `tui/state.go`，`cmd/tui.go` 引用之。
- **`RecentEvents` slice 以容量而非長度預分配** — `make([]string, 0, MaxRecentEvents)` 避免預填空字串，保證第一筆事件落在索引 0。
- **不新增持久化、log 等級控制、面板佈局變更、丟棄計數顯示** — 均屬 Out of scope，維持最小改動範圍。

## 步驟您要求我跳過

沒有。未跳過任何步驟。

## 我不是很確定您想要什麼

沒有。所有 Acceptance 條件都已逐條驗證通過。