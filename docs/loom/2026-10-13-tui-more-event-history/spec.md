# TUI event history retention — spec
intent: 2026-10-13-tui-more-event-history@920b10f
confirmed-behavior: 2026-10-13 @bfb0d89
pre-build-review: not-required — a bounded constant raise in an in-memory display buffer; no security, privacy, data, contract, or cross-system surface, and the requirements are unambiguous.

## Requirements
REQ-1 — History beyond the previous limit
  WHEN the Recent Events panel has accumulated more than one hundred lines in a single TUI run, the user shall be able to scroll up and still see lines older than the hundredth → Acceptance #1
REQ-2 — Fixed larger history cap
  The Recent Events panel shall retain at most a fixed cap of about one thousand lines — approximately ten times the previous limit (user-chosen: "固定大上限（約 1000 行）") — dropping the oldest lines beyond the cap rather than growing without bound → Acceptance #2
REQ-3 — Bottom pinning preserved
  WHEN new event lines arrive, the Recent Events panel shall stay pinned to the newest line at the bottom unless the user has scrolled up → Acceptance #3

## Design decision
- Raise the in-memory Recent Events retention from 100 to 1000 lines (named constant, single place). agent-decided — 1000 matches the user-chosen carried detail (~10x) and is trivially adjustable later, so it is not a one-way door.
- Raise the EventBridge queue capacity from 100 to 1000 to match. agent-decided — with the buffer at 1000 and the queue at 100, a burst would still drop lines at the bridge before the panel ever sees them, so the queue is raised to keep the retention cap the binding limit, not the transport.
- Do not add persistence, log-level controls, panel layout changes, or a dropped-count indicator. agent-decided — all are out of scope in the intent.
- Memory at 1000 lines is negligible (strings of ~100–300 bytes), and rendering rebuilds the panel content each frame, so a fixed cap (not unbounded growth) keeps the per-frame render cost bounded. agent-decided — render cost was the reason for preferring a cap over keep-everything.

## Alternatives considered
- Keep-everything (unbounded) retention: rejected — the user chose a fixed large cap, and per-frame content rebuild cost grows with every line over a long run.
- Raise only the retention buffer, leave the queue at 100: rejected — the queue would remain the tighter bottleneck and drop lines during bursts.
- Configurable cap via a config key: rejected — scope creep beyond the confirmed intent; a named constant is sufficient and simpler.

## Current state evidence
- Forward: slog output → EventBridge (cmd/tui.go:26 sets the default logger to a TextHandler writing to the bridge) → applyEvent (tui/apply.go:5) → AppState.AddRecentEvent (tui/state.go:171, discards beyond 100 at :175).
- Reverse: renderEvents (tui/model.go:1428) reads StateSnapshot.RecentEvents (tui/state.go:49) into a scrollable viewport; NewAppState preallocates capacity 100 (tui/state.go:35).
- Error: queue full → line dropped and counted (tui/bridge.go:123-128, exposed by Dropped at :107); buffer beyond 100 → oldest silently discarded (tui/state.go:175-178).
- Data: AppState.RecentEvents []string with a 100-line cap (tui/state.go:25, 175); EventBridge channel capacity 100 from cmd/tui.go:25 (default at tui/bridge.go:39).
- Boundary: this change touches only the retention cap and the matching bridge queue capacity; log levels, persistence, panel geometry, and the dropped-counter UI stay out of scope.

## UI flows
Surface: Recent Events panel (bottom-right).

| case | what the user does | what they see |
|---|---|---|
| Empty | opens the panel before any events | `(no events)` (unchanged) |
| Fewer than 100 lines | scrolls up | every line since the run started (unchanged) |
| More than 100 lines | scrolls up | lines older than the hundredth are still there (new) |
| Very long run, past ~1000 lines | scrolls to the top | the oldest retained line is ~1000 lines back; older lines are gone |
| New events, panel at the bottom | does nothing | newest line stays visible, pinned to the bottom (unchanged) |
| New events, scrolled up | does nothing | the view stays where it was; it jumps only when the user scrolls back down (unchanged) |

No irreversible step: retention is in memory only and resets when the TUI exits.
