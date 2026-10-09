# ytss TUI — plan
intent: 2026-10-08-ytss-tui@37fe688
spec: docs/loom/2026-10-08-ytss-tui/spec.md@amend2
charter: 1.1

## Task DAG
<!-- When a spec requirement changes after this commit, the un-landed
     tasks it touches are replaced and the reason is named in the commit
     message. Landed tasks stay as they are. -->

Wave 1 — event bridge (parse slog lines + bounded non-blocking queue + AppState)
Wave 2 — TUI core (bubbletea v2 model: file picker, config display, layout, keymap)
Wave 3 — pipeline integration (in-process runner + live status)
Wave 4 — polish & edge cases (quit confirm, resize, empty dirs)
Wave 5 — spec amend1: config card + popup picker + sectioned config tree (2026-10-09, user-approved)

**W1-01 Log-line parser**  after: none  acceptance: 3, 4
- Files: tui/parse.go, tui/parse_test.go
- Test: A3 positive: processing line yields EventVideoStart with title; negative: unrelated line yields EventOther. A4 positive: ERROR-level failure line flagged; boundary: escaped quotes and non-matching url= substring parsed correctly.
- Risk: message-text matching couples TUI to log wording; agent-decided — no structured hook exists in pipeline, text match with tests is the minimal change.

**W1-02 Event bridge (bounded queue + AppState writer)**  after: W1-01  acceptance: 4
- Files: tui/bridge.go, tui/bridge_test.go, tui/apply.go, tui/state.go
- Test: A4 positive: INFO line reaches RecentEvents; boundary: full queue drops line, Write never blocks (1000 writes < 200ms).
- Risk: bridge goroutine leaks on program exit; default Close() on quit — agent-decided.

**W1-03 AppState thread-safe snapshot**  after: none  acceptance: 3
- Files: tui/state.go, tui/state_test.go
- Test: A3 positive: concurrent SetX and Snapshot under -race passes; negative: mutating a Snapshot copy leaves state unchanged.
- Risk: copying AppState copies its mutex; default StateSnapshot struct without mutex — agent-decided.

**W2-01 File picker integration**  after: W1-01  acceptance: 1, 2
- Files: tui/filepicker.go, tui/filepicker_test.go
- Test: A1 positive: env var wins as start dir; boundary: missing dirs fall through candidate chain. A2 positive: yaml/yml selectable; boundary: other extensions not selectable.
- Risk: bubbles v2 filepicker API drift; default pin to installed v2.2.1 signatures — agent-decided.

**W2-02 Config display (structured + raw toggle)**  after: W2-01  acceptance: 3
- Files: tui/config.go, tui/config_test.go
- Test: A3 positive: nested YAML flattens with dot keys, arrays comma-joined; boundary: invalid YAML renders error text.
- Risk: unbounded table height on large configs; default render inside fixed-height panel (clipped) — agent-decided.

**W2-03 TUI layout (four panels + keymap)**  after: W2-02  acceptance: 4, 5
- Files: tui/model.go, tui/view.go, tui/update.go, tui/model_test.go
- Test: A4 positive: four panel titles rendered; negative: focus marker only on focused panel. A5 positive: q with IsRunning shows confirm prompt; boundary: q when idle quits immediately.
- Risk: v2 View() returns tea.View not string; default NewView + AltScreen=true — agent-decided.

**W3-01 Pipeline runner + status wiring**  after: W1-02, W1-03, W2-03  acceptance: 4, 6
- Files: tui/runner.go, tui/runner_test.go, cmd/tui.go, tui/model.go
- Test: A4 positive: batch-complete line updates Stats counts; boundary: second start while running is ignored. A6 positive: r key with config selected emits StartRunMsg; negative: r without config does nothing.
- Risk: pipeline goroutine outlives TUI; default cancel context and Shutdown() on quit — agent-decided.

**W4-01 Quit confirm & edge cases**  after: W3-01  acceptance: 7
- Files: tui/model.go, tui/model_test.go
- Test: A7 positive: q while running shows confirm, second q quits after cancel; boundary: q when idle quits immediately; boundary: resize updates layout.
- Risk: confirm flow loses pipeline exit event; default keep goroutine posting SetRunning(false) — agent-decided.

**W5-01 Config card + popup picker**  after: W4-01  acceptance: 1, 2 (amend1)
- Files: tui/model.go, tui/model_test.go, tui/filepicker.go
- Test: A1 positive: card shows current path (or "(no config selected)"), card renders at content height (short card, top-aligned); A2 positive: Enter on focused card opens popup, Enter on a .yaml closes popup and loads it; boundary: Esc closes popup without selecting; boundary: popup owns keys while open.
- Risk: popup overlay covers the frame; default render popup over the frozen frame with lipgloss.Place, keys routed to picker while open — agent-decided.

**W5-02 Sectioned config tree**  after: W4-01  acceptance: 3 (amend1)
- Files: tui/config.go, tui/config_test.go
- Test: A3 positive: top-level sections get heading lines; nested keys render indented; playlists/channels arrays render as numbered items with name/url/count and indented sub-maps (cookie/copy_to); boundary: no raw map[...] dumps anywhere in the output; boundary: invalid YAML renders error text.
- Risk: deep recursion on exotic YAML; default depth-cap + generic fallback rendering — agent-decided.

**W5-03 Live redraw**  after: W5-02  acceptance: 8 (amend2 REQ-6)
- Files: tui/bridge.go, tui/bridge_test.go, cmd/tui.go, tui/model.go
- Test: SetNotify fires after each applied event; the model redraws via RefreshMsg (Program.Send on its own goroutine — Send blocks on a busy unbuffered queue).
- Risk: bridge goroutine stalls on Program.Send; default own-goroutine poke — agent-decided.

**W5-04 Viewport scrolling**  after: W5-02  acceptance: 9 (amend2 REQ-7)
- Files: tui/model.go, tui/model_test.go
- Test: wheel over a bottom panel scrolls that panel without changing focus; keyboard ↑↓ scrolls the focused panel; events viewport pinned to newest unless scrolled up.
- Risk: wheel target ambiguity; default pointer-position routing over the bottom band — agent-decided.

**W5-05 Config value editing**  after: W5-04  acceptance: 10 (amend2 REQ-8)
- Files: tui/config.go, tui/config_test.go, tui/model.go, tui/model_test.go
- Test: Enter on a value row opens the editor prefilled; commit writes the YAML file (yaml.Node round-trip preserves order/comments) and reloads the tree; Esc cancels; collapsed list items toggle expand/collapse instead.
- Risk: file corruption on malformed edits; default SetValue rejects non-scalars, write failures surface as ERROR events — agent-decided.

**W5-06 Table display polish**  after: W5-05  acceptance: 3, 9 (user feedback 2026-10-09)
- Files: tui/config.go, tui/config_test.go, tui/model.go
- Test: bordered two-column table with separators before sections/items; list items collapsed to one line (Enter expands); name/channel_name-first summaries; every row truncates to the panel width (no wrapping).
- Risk: ambiguous-width glyphs wrap rows; default ASCII markers + fixed-width table — agent-decided.

## Simplicity check
- bubbletea built-in filepicker — taken
- separate tui/ package, no main-package pollution — taken
- reuse config.Load + yaml.Unmarshal for display — taken
- in-process pipeline call (spec Design decision), no subprocess — taken
- scrollable event viewport (bubbles viewport component) — declined: spec says last N lines, plain slice suffices

## Questions asked
① — consequence — 上面需求和產品原則都正確嗎？（含：TUI 是新介面，不影響既有 CLI；回答「對」授權審查通過後自動推送並開 Ready PR，合併另議）（answer: 對）
<!-- ② ran at write-spec; its durable record is the spec's confirmed-behavior line -->

## Risks
1. Log-text coupling: pipeline rewording breaks status parsing. Default: parser tests pin the exact strings; change both together.
2. High CPU render loop. Default: 250ms tick + v2 renderer diffing; no lipgloss cache.
3. Goroutine leak on quit mid-run. Default: cancel + Shutdown in quit path; runner_test covers stop.
4. File picker start dir wrong. Default: env > vault > ~/.config/ytss > home fallback chain with tests.
