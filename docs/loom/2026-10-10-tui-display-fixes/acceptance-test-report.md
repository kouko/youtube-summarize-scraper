# 2026-10-10 TUI Display Fixes — acceptance test report

Tried on 2026-10-10, in a clean copy of the project at eb6ddb2 (intent confirmed). How I tried each line and what came back: `docs/loom/2026-10-10-tui-display-fixes/evidence/acceptance-test-evidence.md`.

## 你要的東西，逐條對照

| # | 你要的東西 | 結果 | 實際發生什麼 | 重測 |
|---|---|---|---|---|
| 1 | TUI 中按 c 鍵，左下角設定面板在結構化樹與原始 YAML 之間切換 | works | 選擇設定檔後，按 c 顯示原始 YAML 文字（含 `llm:`、`provider: claude-api`）；再按 c 回到結構化表格。單元測試 TestModelCToggleRawYAML 驗證此行為。 | — |
| 2 | 設定檔包含中文字（如播放清單名稱「稍後觀看」），設定面板表格所有列寬度等於邊框寬度，分隔線完整對齊 | works | 含 CJK 字元的表格列寬度（60）等於邊框列寬度（60），分隔線完整對齊，無錯位。單元測試 TestModelConfigTableCJKWidth 驗證此行為。 | — |
| 3 | 現有 CLI 行為（`ytss run -c <config>` 等）不受影響 | works | 全套測試 `go test ./...` 通過，包含 cmd、config、fetcher、lang、output、pipeline、subtitle、summarizer、transcriber、tui 等所有套件。 | — |

## 對你既有的資料做了什麼

沒有。此變更僅修復 TUI 顯示層的兩個缺陷：`c` 鍵切換原始 YAML 視圖、CJK 字元的表格寬度計算。程式不會讀取或修改使用者現有的設定檔、輸出目錄、快取或任何持久化資料。

## 我替你決定的事

- **採用 go-runewidth 計算顯示寬度** — 已在 go.sum 為間接依賴，提升為直接依賴無新增下載。`visibleWidth`、`truncateANSI`、`ansiTail` 三處寬度計算統一改用 `runewidth.RuneWidth`，保留 ANSI escape code 跳過邏輯。
- **renderConfig 依 Mode() 分流** — 結構化模式走現有 `RenderTable()`，原始模式走 `ConfigView.Render()`（回傳 `raw` 字串），不新增重複渲染邏輯。

## 步驟您要求我跳過

沒有。未跳過任何步驟。

## 我不是很確定您想要什麼

沒有。所有 Acceptance 條件都已逐條驗證通過。