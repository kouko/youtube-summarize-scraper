# ytss TUI — plan
intent: 2026-10-08-ytss-tui@37fe688
spec: docs/loom/2026-10-08-ytss-tui/spec.md@f230298
charter: 1.1

## Task DAG
<!-- When a spec requirement changes after this commit, the un-landed
     tasks it touches are replaced and the reason is named in the commit
     message. Landed tasks stay as they are. -->

Wave 1 — event bridge (parse slog lines + bounded non-blocking queue + AppState)
Wave 2 — TUI core (bubbletea v2 model: file picker, config display, layout, keymap)
Wave 3 — pipeline integration (in-process runner + live status)
Wave 4 — polish & edge cases (quit confirm, resize, empty dirs)

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
