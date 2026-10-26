# TUI 三區塊版面與單一版面來源 — plan
intent: 2026-10-25-tui-three-panel@6773c4a
spec: docs/loom/2026-10-25-tui-three-panel/spec.md@2d07715
charter: 1.1

## Task DAG
<!-- When a spec requirement changes after this commit, the un-landed
     tasks it touches are replaced and the reason is named in the commit
     message. Landed tasks stay as they are. -->

Wave 1 — one coherent layout refactor, then preservation and suite

**W1-01 Two-row layout, focus-driven content, one geometry source**  after: none  acceptance: 1, 2, 3
- Files: tui/model.go, tui/style.go, tui/model_test.go
- Test: A1 positive: two upper panels, full-width lower at default size; boundary: resize in bounds. A2 positive: focus cycle swaps lower content, restores scroll; boundary: one focus target. A3 positive: render and hit-test share one geometry; boundary: no duplicate rect math.
- Risk: future edits may add a second geometry copy; review checklist calls out single source; scroll restoration reuses existing viewports.

**W1-02 Keys, mouse and full suite preserved**  after: W1-01  acceptance: 4, 5
- Files: tui/model.go, tui/model_test.go
- Test: A4 positive: existing key and mouse tests pass unchanged; boundary: picker and edit popups still clickable. A5 positive: go test ./... -race green; boundary: inspection finds no second coordinate copy.
- Risk: hit-test coordinates may shift with new rects; guarded by existing popup tests.

## Simplicity check
- keep the four-panel grid and only change visible content — declined: the duplicated-copy problem stays; every visual tweak still needs two syncs. No net reduction in code surface.
- add a config switch to toggle between four-panel and two-panel — declined: adds a new runtime flag and its associated code path; scope exceeds the confirmed intent.
- only remove the duplicated mouse-hit copies without reducing panel count — declined: still leaves four-panel grid with two geometry sources; the root cause (two copies) remains, and future bugs are likely.

## Questions asked
capture-intent ① — consequence — 自動發佈還是手動把關？ (answer: 自動發佈，但合併前要使用者確認)
capture-intent ① — behaviour — TUI 從四格改成兩排三區塊，上排焦點切換下排內容、只保留一份座標計算。對嗎？ (answer: 對)
write-spec ② — behaviour — 兩列三區塊、焦點切換下排內容並保留捲動、按鍵與滑鼠互動不變、設定檔內容與日誌不再同時可見。對嗎？ (answer: 對)

## Risks
1. The shared geometry structure is the only place layout edits go; a future edit that accidentally adds a second copy would re-introduce the drift bug. The code-review checklist explicitly notes this.
2. The four `FocusedPanel` enum values survive unchanged; no test or production code references are removed, only the active focus set shrinks from 4 to 2.
