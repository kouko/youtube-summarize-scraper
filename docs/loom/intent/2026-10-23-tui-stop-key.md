# TUI 加入停止鍵
originator: kouko
kind: product
needs-design: yes — TUI 操作表面變更（狀態面板鍵位與提示），倉庫沒有 DESIGN.md 或 ui-flows 文件涵蓋該表面
status: confirmed 2026-10-23
publication: automatic — authorized 2026-10-23 by kouko

## Problem
TUI 目前只有「Run (r)」鍵可以啟動 pipeline，沒有對應的「Stop」鍵讓使用者主動停止正在執行的批次。現有的離開方式（離開程式的熱鍵）是結束整個程式，不是優雅地停止 pipeline 後繼續留在介面。

## Proposed outcome
使用者可以在 TUI 中按 `s` 鍵主動停止 pipeline 的執行；畫面狀態顯示切換為 Stopped，且可以再按 `r` 重新啟動。

## Acceptance
1. pipeline 正在執行時，按 `s` 鍵會停止 pipeline（狀態面板顯示從 Running 變為 Stopped）。
2. pipeline 未在執行時，按 `s` 鍵不會發生任何事（不會啟動、不會報錯）。
3. 停止後，按 `r` 鍵可以重新啟動 pipeline。
4. 提示列在執行中顯示 `[s] Stop`。

## Constraints
- 不改變現有 `q` / `ctrl+c` 離開行為（含執行中的雙重確認）。
- 不影響 `batch.random_order` 或批次處理邏輯。
- 停止 pipeline 的內部邏輯已存在（`StopRunMsg` 處理器），只需接上鍵位綁定與提示。

## Value case
- 受益者：使用 TUI 檢視與控制 pipeline 的使用者。
- 急迫性：中——目前只能透過離開程式來停止 pipeline。
- 既有替代方案：`q` / `ctrl+c`（結束整個 TUI，不是優雅停止）。
- 排擠的工作：無。
- GO。

## Out of scope
- 改變現有 `q` / `ctrl+c` 離開與確認行為。
- pipeline 部分完成後的續跑/resume 功能。
- 其他 TUI 面板或鍵位的調整。

## Open questions
- none
