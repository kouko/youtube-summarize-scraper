# TUI config card: copy path & reveal folder in Finder — plan
intent: 2026-11-01-tui-config-path-actions@9882e8b
spec: docs/loom/2026-11-01-tui-config-path-actions/spec.md@b1aa0d0
charter: 1.1

## Task DAG
**W1-01 Copy/reveal actions, hints, and regression**  after: none  acceptance: 1, 2, 3, 4, 5, 6
- Files: tui/model.go, tui/model_test.go, tui/style_test.go
- Test: A1 positive: copySetsClipboard; boundary: copyFailsReportsError. A2 positive: revealRunsOpenR; boundary: revealFailsReportsError. A3 positive: noConfigShowsHint; negative: noConfigNoSideEffect. A4 positive: outcomeLogged; negative: failureNamesReason. A5 positive: cardHintsShowCopyReveal; boundary: hintsWrapNarrowWidth. A6 positive: existingKeysUnchanged; negative: newKeysInertOutsideCard.
- Risk: seam stubbed so tests never run real pbcopy/open; adding card hint segments must preserve existing hint tests (tui/style_test.go, tui/model_test.go); per spec REQ-1..REQ-6. agent-decided: inline actions, no new files.

## Simplicity check
- tui/actions.go + tui/actions_test.go — taken
- separate W1-02 hint task — taken

## Questions asked
capture-intent ① — what — 「你要的是「不用再來回切視窗」：複製完整設定路徑到剪貼簿，以及——在 Finder 開啟設定檔所在的資料夾。做完後你可以在設定卡有焦點時按一個鍵就把路徑複製到手、按另一個鍵讓 Finder 直接打開那個資料夾；按了沒反應或失敗時畫面會說明原因（不會默默失敗）；還沒選設定檔時按鍵只會提示、不亂動。對嗎？」 (answer: 對)
capture-intent ① — what — 「「開啟」功能希望按鍵做哪種行為？（複製路徑兩種都做，只有開啟需要選）」 (answer: 打開設定檔所在的資料夾 就好)
write-spec ② — behaviour — 「你按複製鍵會看到路徑進了剪貼簿、事件列多一行；你按開啟鍵會看到 Finder 打開設定檔所在的資料夾並選取該檔、事件列多一行。還沒選設定檔時按鍵不會有任何外部動作，只會提示你先選；執行動作失敗時，事件列會說明原因和下一步。對嗎？」 (answer: 對)
write-spec ② — consequence — 「按複製鍵會覆蓋你剪貼簿裡原本的內容（舊內容不會留著）——這正是你要的「複製路徑到剪貼簿」本身（Acceptance #1），沒有替代行為，所以照這樣做。」 (answer: 對)

## Risks
1. Copying overwrites the user's clipboard by design (spec REQ-1); clipboard content has no backup. agent-decided — behaviour was user-requested and disclosed at ②.
2. Non-macOS hosts may lack pbcopy/open; the action fails and reports a way out in Recent Events. agent-decided — no build tags.
3. Tests must never run real pbcopy/open; the command seam is overridden in the suite. agent-decided — single seam replaces exec.Command.
