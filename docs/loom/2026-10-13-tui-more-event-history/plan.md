# TUI event history retention — plan
intent: 2026-10-13-tui-more-event-history@920b10f
spec: docs/loom/2026-10-13-tui-more-event-history/spec.md@bfb0d89
charter: 1.1

## Task DAG
<!-- When a spec requirement changes after this commit, the un-landed
     tasks it touches are replaced and the reason is named in the commit
     message. Landed tasks stay as they are. -->

Wave 1 — event history cap (buffer constant, bridge queue wiring, regression guard)

**W1-01 Raise event history cap to 1000**  after: none  acceptance: 1, 2
- Files: tui/state.go, cmd/tui.go, tui/state_test.go, tui/bridge_test.go
- Test: A1 positive: 150 lines all retained beyond the old 100 limit. A2 boundary: 1100 lines cap at 1000, oldest dropped.
- Risk: cap as one exported constant in tui so the queue call site passes the same value; queue raised to match so bursts do not drop before the buffer — agent-decided, single source of truth (spec REQ-2).

**W1-02 Bottom-pinning regression guard**  after: W1-01  acceptance: 3
- Files: tui/model_test.go
- Test: A3 positive: new events keep a pinned view at the bottom; negative: after scrolling up, re-render does not jump to the bottom.
- Risk: pinning logic untouched (spec REQ-3 is preserved behaviour); test only guards against future regressions — agent-decided.

## Simplicity check
<!-- one line each: `- <simpler shape> — taken` | `- <simpler shape> — declined: <reason>`; or only `- none found` -->

## Questions asked
capture-intent ① — what — 想保留多少歷史記錄？目前事件面板只保留最後 100 行，更早的會被丟掉。 (answer: 固定大上限（約 1000 行）)
capture-intent ① — behaviour — 你要的是：TUI 的事件面板不再只保留最後 100 行記錄，而是保留約 1000 行（約 10 倍）…對嗎？ (answer: 對)
write-spec ② — behaviour — 產生超過 100 行 log 後往上捲，第 100 行以前的記錄還在；跑超過約 1000 行後捲到最頂，更早的仍被丟棄；面板貼底與捲上行為不變——對嗎？ (answer: 對)

## Risks
1. A taller buffer does not change the visible panel height; older lines still need scrolling to reach — expected, matches the confirmed behaviour.
