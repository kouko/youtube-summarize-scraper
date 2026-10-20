# TUI 設定顯示改回原始排列並停止存檔弄亂設定檔
originator: kouko
kind: product
needs-design: yes — TUI 設定顯示/編輯表面變更，倉庫沒有 DESIGN.md 或 ui-flows 文件涵蓋該表面
status: confirmed 2026-10-21
publication: automatic — authorized 2026-10-21 by kouko

## Problem
先前加入的 TUI 隨機顯示功能把設定畫面最上層 section 的排列打亂，這個隨機排列對使用者沒有意義——使用者要的是依設定檔原始順序顯示。此外該功能會動到寫回檔案用的文件，導致在 TUI 編輯並儲存任一個設定值時，實際設定檔的最上層順序被永久打亂、註解與開關設定遺失；且開關設為 false 也會被誤判成開啟。

## Proposed outcome
TUI 設定畫面依照設定檔的原始排列顯示，不再隨機排序；在 TUI 編輯儲存設定值時，設定檔的順序與註解保持原樣。先前為此功能加入的最上層 `random_order` 設定一併移除（包括實際使用的設定檔）。

## Acceptance
1. TUI 設定畫面最上層 section 的順序與設定檔中的順序一致（不再隨機）。
2. 在 TUI 編輯任一設定值並儲存後，實際設定檔的最上層順序與註解保持原樣（目前會被永久打亂）。
3. 設定檔不再帶有最上層的隨機顯示開關；`batch.random_order`（批次處理順序）不受影響、維持有效。

## Constraints
- `batch.random_order`（控制批次處理時頻道/播放清單的先後）維持不變。
- 所有既有測試持續通過。

## Value case
- 受益者：用 TUI 檢視與編輯設定檔的使用者。
- 急迫性：高——目前只要在 TUI 編輯儲存一次，設定檔就會被弄亂。
- 既有替代方案：無。
- 排擠的工作：無。
- GO。

## Out of scope
- 改變 `batch.random_order`（批次處理順序）的行為或預設值。
- 其他與設定顯示順序無關的 TUI 調整。

## Open questions
- none
