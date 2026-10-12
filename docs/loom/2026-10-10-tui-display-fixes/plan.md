# TUI display fixes (c-toggle + CJK table alignment) — plan
intent: 2026-10-10-tui-display-fixes@eb6ddb2
charter: 1.1

## Current State Evidence
- Forward: `tui/model.go` renderConfig (L731) always renders RenderTable; Mode() is never read — 'c' flips the mode flag but the raw YAML view never appears.
- Reverse: `tui/config.go` visibleWidth (L454), `tui/model.go` truncateANSI (L853) and ansiTail (L1092) count one column per rune; CJK runes occupy two, so CJK rows overflow the border.
- Error: RenderTable measures columns with lipgloss.Width (L375) but pads via rune-count padToWidth (L445); the two width systems disagree on wide runes.
- Data: collapsed playlist rows (`config.go` walkSequence L210 → itemInline L221) carry zh-Hant names such as 稍後觀看 — the primary CJK content users see.
- Boundary: github.com/mattn/go-runewidth v0.0.27 already sits in go.sum as indirect (go.mod L26); promoting it to direct adds no new module.

## Task DAG

Wave 1 — rendering fixes (sequential: both tasks edit tui/model.go)

**W1-01 Display-width aware clipping and padding**  after: none  acceptance: 2
- Files: tui/config.go, tui/model.go, tui/config_test.go, tui/model_test.go, go.mod, go.sum
- Test: A2 positive: CJK rows equal border display width; boundary: truncateANSI drops a two-column rune exceeding the budget and appends the style reset.
- Risk: measurement, padding, truncation must share one rule — use runewidth.RuneWidth inside the existing ANSI-skipping walkers; agent-decided, go-runewidth already in go.sum.

**W1-02 Render the raw YAML view on 'c'**  after: W1-01  acceptance: 1
- Files: tui/model.go, tui/model_test.go
- Test: A1 positive: 'c' shows the file's YAML text without the table header, second 'c' restores the table; boundary: raw-mode Enter opens no editor, ↑↓ scrolls.
- Risk: stale content after an in-TUI edit — render from AppState ConfigContent, which commitEdit refreshes, not the initial parse; agent-decided.

Wave 2 — regression guard

**W2-01 CLI-unchanged guard and suite**  after: W1-01, W1-02  acceptance: 3
- Files: docs/loom/2026-10-10-tui-display-fixes/evidence/probes/
- Test: A3 positive: suite green; boundary: diff restricted to tui/ and docs/loom paths.
- Risk: none beyond the suite; the guard is evidence-only with no product code; agent-decided.

## Simplicity check
- none found
## Questions asked
① — what — 你要的是：TUI 按 c 鍵能正確切換結構化樹與原始 YAML；中文內容時表格正確對齊；CLI 行為不變。對嗎？（含自動發布授權說明，使用者可 opt out）

## Risks
1. Rendering-only diff; the log-text parser, event bridge, and pipeline contracts stay untouched.
2. No one-way doors: no new data writes or interface changes — this restores behaviour the 2026-10-08-ytss-tui spec already defined.
