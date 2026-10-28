# TUI 三區塊版面與單一版面來源 — spec
intent: 2026-10-25-tui-three-panel@6773c4a
confirmed-behavior: 2026-10-27 @bba1f14
pre-build-review: not-required — only internal layout restructuring; no new UI interaction, no product surface change beyond layout direction, no security, privacy, data, or cross-system surface.

## Requirements
REQ-1 — Layout direction change
  WHEN the TUI starts or the terminal width changes, the layout shall be two rows: upper row with two focused panels (Settings path display & selection, Execution status), lower row with a full-width content area that displays the content corresponding to the upper‑row focus → Acceptance #1
REQ-2 — Focus-driven content
  WHEN focus shifts between the upper two panels, the lower content area shall display settings content or execution log accordingly, preserving each panel's scroll position → Acceptance #2
REQ-3 — Single source of layout geometry
  THE layout geometry (panel rectangles, viewport dimensions, hint row counts) shall be computed from one source of truth; both rendering and mouse‑hit testing shall read from it → Acceptance #3
REQ-4 — Existing key bindings preserved
  KEY bindings (s/q/ctrl+c, focus cycle, Enter on hints, file picker, config editor) and mouse interaction (panel hints, file‑picker overlay, config editor popup) shall retain their current behaviour → Acceptance #4
  Exception: a click on the config card opens the picker popup only when the card's panel already holds focus; a first click on an unfocused card only focuses it (user-decided 2026-10-27).

## Design decision
- Replace the current four‑panel grid (upper‑left config card, upper‑right status, lower‑left file picker area, lower‑right events panel) with a two‑row layout: upper row has two panels, lower row has one full‑width panel. The focus target set drops from 4 to 2. The layout geometry lives in a single shared structure read by both the viewport renderer and the mouse‑hit test helper. The lower‑row content area simply switches its rendered view based on which upper‑row panel has focus, without any duplicated coordinate calculations. agent-decided — this eliminates the duplicated‑copy problem that caused the 2026‑10‑11 double‑click regression and future visual‑drift bugs. No DESIGN.md or ui-flows document already covers this surface, so `needs-design: yes` is set per spec template. agent-decided — the change is purely structural (panel count and geometry); all existing behaviours (key bindings, mouse hit testing, scroll position retention) are preserved unchanged.
- Do not add a config‑path picker or execution‑log‑only mode; those are future work outside this intent. agent-decided — scope limited to the layout direction change and single‑source geometry.
- Keep the existing `FocusedPanel` enum values and the panel‑specific hint constants; only the panel count and the shared geometry source change. agent-decided — avoids breaking the test expectations for the existing enum and constants.

## Alternatives considered
- Keep the four‑panel grid and only change the visible content — rejected: the duplicated‑copy problem stays; every visual tweak still needs two syncs. No net reduction in code surface.
- Add a config switch to toggle between four‑panel and two‑panel — rejected: adds a new runtime flag and its associated code path; scope exceeds the confirmed intent.
- Only remove the duplicated mouse‑hit copies without reducing panel count — rejected: still leaves four‑panel grid with two geometry sources; the root cause (two copies) remains, and future bugs are likely.

## Current state evidence
- Before: four panels (`PanelFilePicker`, `PanelConfig`, `PanelStatus`, `PanelEvents`) arranged in a 2×2 grid; each panel has its own rectangle computation (`panelRects`) and hint‑hit logic (`hintHitTest`, `pickerPopupRect`, `editPopupRect`, `panelHintRows`, `configTableRowAt`). The shared geometry is split between the viewport renderer and the mouse‑hit helpers, causing the 2026‑10‑11 double‑click bug and every later visual tweak to need two‑copy maintenance.
- After: two upper panels and one full‑width lower panel; layout geometry is computed once per frame and lives in a single source that both the renderer and the mouse‑hit helpers read. The four `FocusedPanel` enum values survive (only the active focus set changes from 4 to 2). Function signatures used by rendering and hit‑testing may change to accept the shared geometry; key handling and panel identifiers stay the same.
- The existing test suite (`go test ./tui/...`) passes on the before state; the after‑state acceptance checks (#1‑#5) will be verified by the guard tests written in build.

## UI flows
Surface: TUI main view (upper row: two panels; lower row: full‑width content area).

| case | what the user does | what they see |
|---|---|---|
| Fresh start / terminal resize | watches the layout | two upper panels side by side, lower area fills the rest; layout resizes without gaps or overflow |
| Shift focus from Settings to Execution (or vice‑versa) | presses focus‑cycle key or clicks the other upper panel | the lower area immediately shows the new content (settings card or execution log); each panel’s scroll position stays where it was |
| Click the config card while the status panel is focused | clicks the config card once | the config card's panel takes focus only; the file‑picker popup does not open |
| Click the config card again while it is focused | clicks the card (or its [Enter] hint) | the file‑picker popup opens |
| Terminal width changes (wider / narrower) | resizes the terminal | both rows stretch/fit; lower area remains full‑width; no panels disappear or squash |
| Normal use (no layout change) | uses the TUI as today | identical behaviour to today, except the lower area’s content now follows focus instead of being fixed |

No irreversible step: layout is purely in‑memory and resets when the TUI exits.