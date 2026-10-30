# TUI watch 模式批次錯誤不中斷 + 預設開啟 watch — plan
intent: 2026-10-30-tui-watch-continue-on-error@c75b8cc
charter: 1.1

## Current State Evidence
- Forward: `tui/model.go:1789-1791` watch loop 單輪失敗時回傳 `err`，直接結束整個迴圈，畫面回到 Stopped 狀態。
- Forward: `config/config.go:402` `DefaultConfig()` 回傳 `Batch: BatchConfig{RandomOrder: true, WatchInterval: 10}`，`Watch` 為 Go 零值 `false`。
- Reverse: `cmd/run.go:93-107` CLI watch 迴圈失敗只記 `slog.Error`、不回傳、sleep interval 後 `continue` 至下一輪——這就是「對齊的目標」。
- Boundary: `tui/model_test.go:73-87` TestModelFocusMarkerOnActivePanel，與此次變更無關；`config/config_test.go:117-170` TestDefaultConfig_WatchMode 斷言 `Watch: expected false by default`（目前預設）。

## Task DAG
**W1-01 Watch loop error handling + config 預設值**  after: none  acceptance: 1, 2, 3, 4, 5, 6
- Files: tui/model.go, config/config.go
- Test: A1 positive: watchErrorContinues; negative: singleBatchEndsOnError. A2 positive: configWatchTrueDefault; negative: explicitWatchFalseStaysSingle. A3 positive: existingSuiteGreen; negative: regressions. A4 positive: CLIwatchBehaviourUnchanged; negative: unintendedCLIChange. A5 positive: allTestsPass; negative: raceFailures. A6 positive: tuiWatchStartsAutomatically; boundary: firstIterationWatchIter.
- Risk: 兩處改動皆單獨、互不影響。錯誤標記 `AddRecentEvent` 已存在，不新增機制；預設值變更為 bool 零值互補，不觸碰其他欄位。agent-decided：兩處改動皆無交集、各自有完整測試覆蓋。

## Simplicity check
- none found

## Questions asked
capture-intent ① — what — 「你想要的兩件事（合併在同一變更）：1. TUI watch 模式錯誤不中斷：自動循環模式下，單輪批次發生錯誤時，顯示錯誤並繼續等待下一輪（與 CLI 行為一致），只在使用者主動停止或離開時結束。2. watch 預設值改 `true`：config 的 `batch.watch` 預設從 `false` 改為 `true`，`watch_interval` 維持 10 分鐘。未在 config 設定 watch 的使用者，升級後會自動進入自動循環模式；可在 config 設 `watch: false` 回到單次批次。」 (answer: 對)
capture-intent ① — consequence — 「你回答「對」即代表授權之後自動發佈（push + 準備好 PR 後合併仍會另行問你）；你可以在發佈前隨時明確退出。」 (answer: 對)
capture-intent ② — what — 「watch key 從 2026-03-25 commit 3a951c7 引入，預設一直是 false；目前所有 config（含 dotfiles 備份）皆未設定 watch，所以一律是單次批次模式。現在想把預設改成 true，讓升級後自動進入循環，但使用者仍能設 `watch: false` 回單次。」 (answer: 對)

## Risks
1. TUI watch loop 結構變更：若 `return err` 改為 `continue`，確保沒有其他 break/return 路徑誤觸發。agent-decided：只在單一點（line 1789-1791）改、保持其餘迴圈結構不動。
2. Config 預設值變更影響所有未設定 watch 的使用者：若有人已手動維護 config（無 watch key），升級後會自動進入循環。agent-decided：可在 config 設 `watch: false` 回單次；變更有明文說明與退路。