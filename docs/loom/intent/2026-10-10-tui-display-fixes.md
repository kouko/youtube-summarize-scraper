# 2026-10-10-tui-display-fixes
originator: kouko
kind: product
needs-design: no — 修復的是 2026-10-08-ytss-tui/spec.md 已定義的 TUI 行為（REQ-2/REQ-5 按鍵切換、W5-02 表格渲染），無新使用者介面表面
evidence: [docs/loom/2026-10-08-ytss-tui/spec.md]
status: confirmed 2026-10-10
publication: automatic — authorized 2026-10-10 by kouko

## Problem
使用者在 TUI 中按下 c 鍵，預期會在設定面板看到原始 YAML 內容，但畫面沒變化（仍顯示結構化表格）。當設定檔內容包含中文字（如播放清單名稱「稍後觀看」），設定面板的表格邊框會錯位、字元對不齊。

## Proposed outcome
TUI 在使用者按 c 鍵時，能正確在結構化樹圖與原始 YAML 之間切換。設定面板的表格無論內容是否含中文，邊框與分隔線都正確對齊。

## Acceptance
1. 在 TUI 中選擇設定檔後，按 c 鍵會讓左下角設定面板顯示原始 YAML 文字；再按一次 c 鍵回到結構化表格。
2. 設定檔包含中文字（如播放清單名稱「稍後觀看」）時，設定面板表格的所有列寬度都等於邊框寬度，分隔線完整對齊，無錯位。
3. 現有 CLI 行為（`ytss run -c <config>` 等）不受影響。

## Constraints
- 不改動 pipeline 核心邏輯（同 2026-10-08-ytss-tui 的 constraint）。
- 修復範圍限於 tui/ 套件與 cmd/tui.go 的相關 wiring。

## Value case
- 受益者：在終端機直接觀察 ytss 執行狀態、快速切換設定檔、除錯設定的使用者。
- 急迫性：這兩個缺陷讓 TUI 的核心交互（c 鍵切換）與中文顯示（主要使用者語言）失效，降低除錯體驗。
- 現有替代方案：不使用 TUI，改用 CLI + tail log 除錯。
- 被排擠工作：無。

## Out of scope
- 新增 TUI 功能（如暫停/繼續 pipeline、編輯設定值等）。
- 修復 TUI 以外的程式碼。

## Open questions
- none