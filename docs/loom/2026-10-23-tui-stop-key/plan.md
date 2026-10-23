# TUI 加入停止鍵 — plan
intent: 2026-10-23-tui-stop-key@858f5e9
spec: docs/loom/2026-10-23-tui-stop-key/spec.md@f57a283
charter: 1.1

## Task DAG
<!-- When a spec requirement changes after this commit, the un-landed
     tasks it touches are replaced and the reason is named in the commit
     message. Landed tasks stay as they are. -->

Wave 1 — 新增停止鍵與狀態提示

**W1-01 新增 `s` 鍵綁定與狀態提示**  after: none  acceptance: 1, 2, 3, 4
- Files: tui/model.go
- Test: A1 positive: 運行中按 s 停止，狀態變 Stopped; boundary: 未運行時按 s 無反應。A2 positive: 狀態面板顯示 Stopped; boundary: 停止後仍留在介面。A3 positive: 停止後按 r 重啟; boundary: r 仍需 ConfigPath 非空。A4 positive: 執行中提示列顯示 [s] Stop; boundary: 未執行時不顯示 [s] Stop。
- Risk: 新增 key binding（hint、case "s"、dispatch StopRunMsg），不動現有 q/ctrl+c 行為、不動 batch.random_order。StopRunMsg 處理器已存在，純接線（spec REQ-1/REQ-2）。

## Simplicity check
- none found

## Questions asked
capture-intent ① — behaviour — 你要的是在 TUI 中加上「停止」功能，做完後你可以按 `s` 鍵主動停止 pipeline，畫面會顯示已停止狀態。對嗎？ (answer: 對，就是這個)
write-spec ② — behaviour — 可見行為讀回：1. pipeline 正在執行時，按 s 鍵：畫面狀態從 Running 變為 Stopped，提示列顯示從 [r] Run 變為 [s] Stop 2. 停止後，按 r 鍵：可以重新啟動 pipeline，畫面狀態恢復 Running 3. TUI 未執行時，按 s 鍵：無任何反應。對嗎？ (answer: 對，確認無誤)

## Risks
1. 純新增鍵位綁定與提示，StopRunMsg 停止邏輯已存在；不做任何 pipeline 邏輯變更，batch.random_order 不受影響（spec REQ-1/REQ-2）。
2. 無 one-way doors；全部變更可 git revert 回滾；無用戶數據改寫、無付費服務、無平台鎖定。
