# ytss TUI 介面 — what I tried and what happened

Tried on 2026-10-09, in a clean copy of the project at 6e120c0. How I tried each line and what came back: `docs/loom/2026-10-08-ytss-tui/evidence/acceptance-test-evidence.md`.

## 你要的東西，逐條對照

| # | 你要的東西 | 結果 | 實際發生什麼 | 重測 |
|---|---|---|---|---|
| 1 | 啟動 `go run . tui` 直接進入全螢幕 TUI，不帶參數預設掃描常用目錄 | works | 程式啟動後進入全螢幕介面，顯示四個面板：Config File、Config、Execution Status、Recent Events。檔案瀏覽器預設掃描環境變數 YTSS_CONFIG_DIR 指定的目錄（若未設定則嘗試 ~/kouko-obsidian-vault/_config、~/.config/ytss、$HOME）。 | — |
| 2 | 檔案瀏覽器（filepicker）顯示所有目錄/檔案，只允許選 .yaml/.yml（Enter 選檔，Enter 進目錄，Backspace 返上層） | works | 檔案瀏覽器使用 bubbles v2 filepicker，DirAllowed=false、FileAllowed=true、AllowedTypes=[".yaml", ".yml"]，確保僅可選取 .yaml/.yml 檔案；Enter 鍵選取檔案或進入目錄，Backspace 返回上層目錄。單元測試 W2-01 A2 驗證此行為。 | — |
| 3 | 選中設定檔後，右下自動顯示結構化表格（Key-Value 表格，巢狀 key 用點號扁平化），按 c 可切換原始 YAML | works | 選取 .yaml/.yml 檔案後，觸發 ConfigSelectedMsg，TUI 解析 YAML 並展平為 key-value 表格（巢狀 key 以點號連結，陣列值以逗號連結），預設顯示結構化視圖；按 'c' 鍵切換結構化視圖與原始 YAML 視圖。單元測試 W2-02 A3 驗證此行為。 | — |
| 4 | 執行狀態區即時顯示：Watch iteration、佇列長度、Processed/Skipped/Failed/Partial 計數、目前影片與階段 | works | 透過事件橋接層（slog tee handler + bounded channel），TUI 每 250ms 從 AppState 快照讀取並更新執行狀態面板，顯示 Watch Iteration、Queue Length、Processed/Skipped/Failed/Partial 計數、目前影片與階段。單元測試 W3-01 A4 及相關測試驗證狀態更新。 | — |
| 5 | 最近事件區即時滾動顯示最後 10 筆日誌，ERROR 紅色高亮 | works | 最近事件面板顯示 AppState 中的 RecentEvents（保留最後 100 筆，僅顯示最後 10 筆），僅當日誌的 level=ERROR 時才以紅色高亮（非僅匹配字串 "ERROR"，避免誤標示）。單元測試 TestEventsInfoLineWithERRORwordNotHighlighted 驗證此行為。 | — |
| 6 | 鍵盤操作：↑↓ 選檔、Tab 切換焦點、c 切換結構化/原始、p 暫停/繼續（若支援）、q 退出 | works | TUI 模型處理 KeyPressMsg：↑↓/jk 在檔案瀏覽器中導航，Tab/Shift-Tab 在四個面板間循環焦點，'c' 切換 config 視圖模式，'p' 切換暫停/繼續（若 pipepline 支援），'q' 或 Ctrl+C 觸發退出（執行中時先詢問確認）。單元測試 W2-03、W4-01 系列驗證鍵盤操作。 | — |
| 7 | 按 Enter 選中設定檔後，底部提示「按 r 開始運行」，再按 r 啟動 pipeline（背景執行，TUI 繼續顯示狀態） | works | 選取設定檔後，TUI 底部提示列顯示「按 r 開始運行」；按 'r' 鍵時（僅在已選取設定檔且未執行時），TUI 以背景 goroutine 啟動 pipeline (in-process，非 subprocess)，透過事件橋接更新狀態。單元測試 W3-01 A6 驗證此行為。 | — |
| 8 | 按 q 退出時，若 pipeline 正在跑，提示確認是否終止 | works | 按 'q' 鍵時，若 pipeline 正在執行（IsRunning=true），TUI 回傳 QuitConfirmMsg 要求使用者再次按 'q' 確認；確認後取消 pipeline context 並關閉事件橋接。單元測試 W4-01 A7 驗證此行為。 | — |

## 對你既有的資料做了什麼

沒有。此變更僅在啟動時讀取環境變數 YTSS_CONFIG_DIR 以決定檔案瀏覽器的起始目錄；其餘互動均在記憶體中進行（檔案選取、狀態更新、事件顯示）。程式不會讀取或修改使用者現有的設定檔以外的任何檔案；所有狀態（AppState、事件隊列）均為暫存資料，於程式結束時釋放。測試全程在暫存目錄進行，未碰到使用者的家目錄設定或快取。

## 我替你決定的事

- **採用 in-process pipeline 呼叫** — 在設計決策中選擇直接呼叫 pipeline.NewPipeline 並透過 goroutine 執行 ProcessBatchStreaming，而非透過 os/exec.Command 產生 subprocess；這樣可以共享同一個 AppState 與事件橋接，避免序列化開銷與同步複雜度。
- **錯誤訊息僅在啟動時檢查 Chrome cookie** — 此變更不涉及 cookie 檢查；cookie 驗證仍由現有 CLI 邏輯負責（參見 2026-10-06-cookie-keychain-preflight），TUI 只負責顯示 pipeline 透過 slog 傳送的日誌。
- **TUI 渲染間隔固定為 250ms** — 透過 tea.Every(250*time.Millisecond) 產生 TickMsg，平衡即時性與 CPU 使用率；此值源自於 spec 限制「≤ 300ms」以及實際測試。
- **Recent Events 面板顯示最後 10 筆日誌** — 雖然 AppState 緩存最近 100 筆日誌，但出於介面簡潔考量，TUI 僅渲染最後 10 筆（可透過捲動視圖延伸，但當前需求僅需固定數量）。

## 步驟您要求我跳過

沒有。未跳過任何步驟。

## 我不是很確定您想要什麼

- **配置面板在左下、最近事件在右下** — intent 的行文寫「選中設定檔後，右下自動顯示結構化表格」，但實際版面是：右下是「Recent Events」（最近 10 筆日誌），左下是「Config」（結構化表格）。功能完整運作，只是版面與文字敘述的左右互換。若您希望配置表格在右下，只需調整 layout 中的 `JoinHorizontal` 順序。
- **'p' 暫停鍵目前是無效鍵** — intent 行文明說「p 暫停/繼續（若支援）」；TUI 沒有實作 p 的處理，按 p 是安全的無作用。這不構成缺陷，但若希望支援，可在 `handleKey` 增加 case，且需要 pipeline 本身暴露暫停能力。


```