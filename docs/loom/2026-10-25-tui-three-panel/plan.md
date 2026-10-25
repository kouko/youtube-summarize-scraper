# TUI 三區塊版面與單一版面來源 — plan
intent: 2026-10-25-tui-three-panel@6773c4a
spec: docs/loom/2026-10-25-tui-three-panel/spec.md@f5dba5a
charter: 1.1

## Task DAG
<!-- When a spec requirement changes after this commit, the un-landed
     tasks it touches are replaced and the reason is named in the commit
     message. Landed tasks stay as they are. -->

Wave 1 — layout geometry single source (panel count 4→2, shared geometry)

**W1-01 Shared layout geometry + focus set shrink**  after: none  acceptance: 1, 2, 3
- Files: tui/model.go, tui/style.go, tui/bridge_test.go
- Test: A1 positive: upper row has 2 focused panels (Settings, Execution) after focus cycle; negative: upper row had 4 before. A2 positive: focus cycle between the two panels preserves each panel's scroll position; boundary: any terminal width change keeps layout valid. A3 positive: layout geometry (panel rectangles, viewport dims, hint row counts) computed from one source; both rendering (`renderConfig`, `renderEvents`) and mouse‑hit (`hintHitTest`, `pickerPopupRect`) read the same source; boundary: no duplicated coordinate math survives after change. A4 positive: existing key bindings (focus cycle, s/q/ctrl+c, Enter on hints) and mouse interaction (panel hints, file‑picker overlay, config editor popup) retain current behaviour; boundary: all 1845 lines of model.go compile and pass `-race`.
- Risk: removing the second geometry source means any future visual‑tweaks that once needed two copies now have one; the risk is that a developer might reintroduce a second copy unintentionally — agent-decided, documented in spec Design decision; the shared geometry structure is the only place to edit, and the code‑review checklist includes “single source of truth for layout”.

## Simplicity check
- keep the four `FocusedPanel` enum values — declined: removing them would break the existing test expectations for panel identifiers and would require touching every `switch`/`case` on panel IDs across the code base; not needed for the layout change.
- keep the existing `panelRects` return type — declined: the function still returns 4 rectangles internally; only the focus set changes from 4 to 2, so the return type stays compatible.

## Questions asked
capture-intent ① — what — 佇列滿時丟哪一行？現況丟新的，面板凍在爆量那批的開頭。 (answer: 方案 A — 修橋，保留最新)
capture-intent ① — what — 「爆量」指短時間內日誌超過面板上限 1000 行、佇列必須開始丟行的情境；修成丟最舊、保留最新，對嗎？ (answer: 對)
write-spec ② — behaviour — 爆量時面板永遠顯示最新的活動（最新行一直出現、被丟的是最舊行）；沒有爆量時行為完全不變——對嗎？ (answer: 對，就是這個)
capture-intent ① — what — TUI 版面從四格改成兩排三區塊，上排焦點切換下排內容跟著變、只保留一份座標計算。對嗎？ (answer: 對，就是這個)
write-spec ② — behaviour — 爆量時面板永遠顯示最新的活動（最新行一直出現、被丟的是最舊行）；沒有爆量時行為完全不變——對嗎？ (answer: 對，就是這個)

## Risks
1. The shared geometry structure is the only place layout edits go; a future edit that accidentally adds a second copy would re‑introduce the drift bug. The code‑review checklist explicitly notes this; agent‑decided.
2. The four `FocusedPanel` enum values survive unchanged; no test or production code references are removed, only the active focus set shrinks from 4 to 2.