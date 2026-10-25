# TUI 三區塊版面與單一版面來源 — plan
intent: 2026-10-25-tui-three-panel@6773c4a
spec: docs/loom/2026-10-25-tui-three-panel/spec.md@f5dba5a
charter: 1.1

## Task DAG
<!-- When a spec requirement changes after this commit, the un-landed
     tasks it touches are replaced and the reason is named in the commit
     message. Landed tasks stay as they are. -->

Wave 1 — layout geometry single source (panel count 4→2, shared geometry)

**W1-01 Shared geometry source**  after: none  acceptance: 1, 2, 3
- Files: tui/model.go
- Test: layout geometry single source; both rendering and mouse‑hit code read the same shared structure; after change, no duplicated coordinate math survives.
- Risk: future edits might re‑introduce a second geometry copy; code‑review checklist explicitly notes this.

**W1-02 Focus set shrink (4→2)**  after: W1-01  acceptance: 4
- Files: tui/model.go
- Test: focus cycle only touches the two upper panels; focus cycle between Settings and Execution preserves each panel's scroll position; all 1845 lines of model.go compile and pass `-race`.
- Risk: none beyond W1-01.

## Simplicity check
- keep the four-panel grid and only change visible content — declined: the duplicated-copy problem stays; every visual tweak still needs two syncs. No net reduction in code surface.
- add a config switch to toggle between four-panel and two-panel — declined: adds a new runtime flag and its associated code path; scope exceeds the confirmed intent.
- only remove the duplicated mouse-hit copies without reducing panel count — declined: still leaves four-panel grid with two geometry sources; the root cause (two copies) remains, and future bugs are likely.

## Questions asked
capture-intent ① — what — TUI 版面從四格改成兩排三區塊，上排焦點切換下排內容、只保留一份座標計算。對嗎？ (answer: 對)
write-spec ② — behaviour — 爆量時面板永遠顯示最新的活動（最新行一直出現、被丟的是最舊行）；沒有爆量時行為完全不變——對嗎？ (answer: 對，就是這個)