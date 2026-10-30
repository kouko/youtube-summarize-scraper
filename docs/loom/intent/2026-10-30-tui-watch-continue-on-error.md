# TUI watch 模式批次錯誤不中斷循環 + 預設開啟 watch
originator: kouko
kind: product
needs-design: no — TUI run/stop 與 watch 表面已由 docs/loom/2026-10-08-ytss-tui、docs/loom/2026-10-23-tui-stop-key 的 spec 涵蓋；CLI 對應行為（批次錯誤記 log 後續跑下一輪）為既有釘死行為；config 預設值變更為介面變更但已有現行 spec 涵蓋
status: confirmed 2026-10-30
publication: automatic — authorized 2026-10-30 by kouko

## Problem
開啟自動循環模式的 TUI，某一輪批次發生錯誤（例如整個頻道抓取失敗）時，會直接結束整個執行、畫面回到停止狀態，等使用者手動重跑。同樣的錯誤在命令列版本只會記一筆錯誤、等下一輪自動重試。無人值守時，一次暫時性的抓取失敗就讓整晚的自動循環中斷，且沒有人發現。

此外，watch 模式目前預設關閉（`watch: false`），使用者若不在 config 明確設定 `watch: true`，就只能跑單次批次。這與「無人值守持續運作」的產品預期不符。

## Proposed outcome
1. TUI 的自動循環模式在單輪批次發生錯誤時，顯示該錯誤並照常等待下一輪，與命令列版本行為一致；循環只在使用者主動停止或離開時結束。
2. config 的 `batch.watch` 預設值改為 `true`，`watch_interval` 維持 10 分鐘。未在 config 設定 watch 的使用者，升級後會自動進入自動循環模式（可在 config 設 `watch: false` 關閉）。

## Acceptance
1. watch 模式執行中，某一輪批次發生批次級錯誤時，執行不結束：錯誤出現在事件區，經過設定間隔後自動開始下一輪。
2. 使用者主動停止或離開時，循環照常停止，行為不變。
3. 未開 watch（單次批次）時，批次結束（含錯誤）後回到停止狀態，行為不變。
4. 命令列版本的行為不變（仍為 `watch: false` 預設，由使用者決定是否加 `--watch` 或 config 開啟）。
5. 新建 config 或未設 watch 的現有 config，預設行為變為自動循環（watch: true，interval 10 分鐘）；可在 config 設 `watch: false` 回到單次批次。
6. TUI 啟動時若 config 為 watch: true，狀態面板顯示 Watch Iteration 並開始自動循環。

## Constraints
- 單支影片錯誤的現行處理（計入 failed、繼續下一支）不變。
- watch 間隔、循環控制鍵與停止行為不變。
- 命令列版本的程式碼與預設值不動。

## Value case
- 受益者：無人值守跑批次的使用者。
- 急迫性：中——一次暫時性錯誤會中斷整晚自動循環，違反產品原則「Unattended runs keep working」與「不能靜默失敗」。
- 既有替代方案：人工發現後按 r 重跑（違反無人值守目的）；在 config 手動加 watch: true（門檻較高）。
- 排擠的工作：無——小範圍行為對齊 + 預設值變更。
- GO：與命令列行為一致、符合既有產品原則、降低自動循環門檻。

## Out of scope
- 錯誤重試策略（退避、重試次數上限）。
- 命令列版本的預設值或行為調整。

## Open questions
- none
