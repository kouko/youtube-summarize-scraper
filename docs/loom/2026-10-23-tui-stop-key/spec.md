# TUI 加入停止鍵 — spec
intent: 2026-10-23-tui-stop-key@858f5e9
confirmed-behavior: 2026-10-23 @f57a283
pre-build-review: not-required — 操作僅在 TUI 層新增鍵位綁定，無法直接影響 pipeline 邏輯、不改寫檔案、不變更 batch.random_order；屬於純 UI 行為調整

## Requirements
REQ-1 — 停止鍵綁定
  當 pipeline 正在執行時，按 `s` 鍵會觸發停止信號 → Acceptance #1
REQ-2 — 停止後畫面狀態
  pipeline 停止後，狀態面板顯示從 Running 變為 Stopped → Acceptance #2
REQ-3 — 停止後可重新啟動
  按 `r` 鍵可以重新啟動 pipeline → Acceptance #3

## Design decision
- 新增鍵位 `s` 綁定 `StopRunMsg`，功能邏輯已在 `model.go:457-461` 存在，只需接上 key binding。
- 不改變現有 `q` / `ctrl+c` 離開行為（含執行中的雙重確認）。
- 不影響 `batch.random_order` 或批次處理邏輯。
- 代碼變更極小（3 行 hint、case "s"、dispatch），無法回退的範圍極少，標記為 `agent-decided`。
- 「停止」按鈕僅在 pipeline 執行中顯示 `[s] Stop`，未執行時不顯示，屬於純 UI 提示調整。

## Alternatives considered
- 僅在 q/ctrl+c 中加入簡單退出：拒絕，因為使用者想要「停止 pipeline 但保持介面開啟」，而不是結束程式。
- 使用確認對話方塊再停止：拒絕，因為功能邏輯已有 `StopRunMsg`，僅需鍵位即可，不需要額外對話方塊增加複雜度。

## Current state evidence
- Forward: tui/model.go:457-461 (StopRunMsg handler)
- Reverse: N/A
- Error: N/A
- Data: N/A — pipeline status is internal bool `isRunning` and statusView render
- Boundary: tui/model.go `handleKey` case "s" → StopRunMsg

## UI flows
- TUI 執行中，按 `r` 鍵：畫面顯示「Status: Running」，提示列顯示 `[r] Run`
- TUI 執行中，按 `s` 鍵：畫面狀態從 Running 變為 Stopped，提示列顯示從 `[r] Run` 變為 `[s] Stop`，且可以再按 `r` 重新啟動
- TUI 未執行時，按 `s` 鍵：無任何反應
