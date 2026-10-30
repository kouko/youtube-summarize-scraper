# TUI 下方內容面板焦點樣式一致 — plan
intent: 2026-10-29-tui-lower-panel-focus-style@1e5c372
charter: 1.1

## Current State Evidence
- Forward: `tui/model.go:1560` renderConfig passes `m.focus == PanelFilePicker` to RenderPanelWithSegments, so the lower Settings panel renders active only when its upper card is focused.
- Forward: `tui/model.go:1613` renderEvents passes a constant `false`, so the lower Recent Events panel never renders active, regardless of focus.
- Reverse: `tui/model.go:1425-1428` View selects the lower content by `lowerContentKind()` (focus); the events panel exists only while PanelStatus is focused.
- Error: `tui/style.go:181-195` the active flag only toggles title background and border color; there is no error path involved.
- Data: `tui/model.go:52-54,150` focus holds PanelFilePicker or PanelStatus; `tui/model.go:106-108` lowerContentKind maps focus to PanelConfig or PanelEvents.
- Boundary: `tui/model_test.go:73-87` TestModelFocusMarkerOnActivePanel asserts the focus marker sits on the focused upper panel only and excludes "Recent Events".

## Task DAG
One task: mirror the config side's focus flag in the events panel render.

**W1-01 Style the lower events panel on status focus**  after: none  acceptance: 1, 2, 3, 4
- Files: tui/model.go, tui/model_test.go
- Test: A1 positive: statusFocusLogActive; negative: configFocusLogAbsent. A2 positive: configLowerStillActive; negative: statusFocusConfigHidden. A3 positive: cycleStyleTransfers; boundary: exactlyOneActive. A4 positive: existingSuiteGreen; boundary: interactionUntouched.
- Risk: Style flag only; no geometry, focus or input change. Extend the existing focus-marker test (tui/model_test.go:73-87) without narrowing it. agent-decided: mirror `m.focus == PanelStatus`.

## Simplicity check
- none found

## Questions asked
capture-intent ① — what — 「你要的是：切到「執行狀態」時，下方的日誌面板也顯示跟「設定檔」那一側相同的焦點樣式；兩側行為一致，焦點切過去樣式就出現、切走就消失。除此之外焦點切換、捲動、滑鼠點擊都不動，只是把日誌面板的樣式修對。對嗎？」 (answer: 對)
capture-intent ① — consequence — 「你回答「對」即代表授權之後自動發佈（push + 準備好 PR，合併仍會另行問你）；你可以在發佈前隨時明確退出。」 (answer: 對)

## Risks
1. Single style-flag change with no layout or input paths touched; plan-wide risk is render regression, covered by W1-01's test pair. agent-decided: mirror the config side's flag.
