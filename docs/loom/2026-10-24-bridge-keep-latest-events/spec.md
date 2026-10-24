# Event bridge keeps latest events under burst — spec
intent: 2026-10-24-bridge-keep-latest-events@a33539d
confirmed-behavior: 2026-10-24 @e2daf55
pre-build-review: not-required — an internal queue eviction semantics change in an in-memory display buffer; no security, privacy, data, contract, or cross-system surface, and the requirements are unambiguous.

## Requirements
REQ-1 — Latest lines survive overflow
  WHEN log lines arrive faster than the queue drains and overflow forces drops, the lines retained for the Recent Events panel shall be the most recently written ones, not the earliest → Acceptance #1
REQ-2 — Newest lines never dropped by overflow
  WHEN a burst of more than the queue capacity lines is written, every line among the last capacity lines written shall be retained (never dropped), so the panel ends with the contiguous newest suffix in write order → Acceptance #1
REQ-3 — Non-blocking writes, exact accounting
  Write shall never block on a full queue, and every written line shall be either applied or dropped exactly once, so Applied + Dropped equals the number of written lines → Acceptance #2
REQ-4 — Existing bridge behavior preserved
  Partial-line buffering (a fragment waits for its newline), the post-apply notify callback, idempotent Close that waits for the consumer to exit, and queue capacity matching the panel cap shall all behave as before → Acceptance #3

## Design decision
- Replace the queue's internal transport from a buffered channel to a mutex-protected deque (slice with front-trim), with a sync.Cond for the consumer's wait. agent-decided — a channel cannot remove its oldest element, so drop-oldest on a channel requires a second non-blocking receive whose eviction target can race the consumer's concurrent receive; the mutex serializes the detect-full/evict/append decision atomically, making the retained window provably deterministic.
- On overflow, evict the queue front (oldest) and count it as dropped, then append the newest line. agent-decided — this is the user-chosen option A ("最新優先").
- Determinism argument: with capacity C and writes e-0..e-(N-1), a line e-j can be evicted only when the queue holds exactly C lines ending at the just-written line, which forces j = k - C for the writing index k; since k <= N-1, j <= N-1-C, so the last C written lines are never evicted — the panel's keep-latest cap then retains exactly the contiguous suffix, regardless of consumer speed.
- Keep Write's partial-line buffering, the notify callback, and Close semantics (idempotent, waits for consumer, abandons queued lines) unchanged. agent-decided — the intent limits scope to the eviction mechanism.
- Do not fix the pre-existing unsynchronized Write-side pending buffer under concurrent writers (multi-goroutine Write races on the shared fragment buffer). agent-decided — it is a separate latent issue outside this intent; production writes from a single pipeline goroutine, and changing it is not needed for the CI failure. Flagged as a follow-up observation.

## Alternatives considered
- Keep the channel; on a failed send, do a non-blocking receive to evict the front, then retry the send: rejected — between the failed send and the evict receive, the consumer can concurrently receive, shifting the evicted index by one (j can become k-C+1); at the window edge this can drop the oldest line of the newest-C suffix, so the retained window is not provably exact and the probe stays timing-sensitive.
- Weaken the failing probe to expect the earliest-C window (matching current drop-new behavior): rejected by the user — the panel's semantic is "recent events", so the latest lines are the correct ones to keep.
- Increase the queue capacity so bursts never overflow: rejected — any fixed capacity can be exceeded by a long enough burst; the bug is the eviction semantics, not the size.

## Current state evidence
- Forward: slog output → EventBridge.Write (tui/bridge.go:73) splits complete lines → enqueue (tui/bridge.go:123) does a non-blocking send → consume (tui/bridge.go:53) applies via applyEvent (tui/apply.go:5) → AppState.AddRecentEvent (tui/state.go:178) keeps the latest MaxRecentEvents = 1000 (tui/state.go).
- Reverse: renderEvents reads StateSnapshot.RecentEvents for the panel; the panel already keeps the newest N — the loss happens upstream in the bridge queue.
- Error: enqueue's full-queue branch drops the NEW line and counts it (tui/bridge.go:124-128). Under a 1500-line burst with capacity 1000, if the consumer keeps pace for the first ~1031 lines then falls behind, the final ~469 lines are dropped and the panel retains e-31..e-1030 instead of e-500..e-1499 — exactly the CI failure in run 39882460214.
- Data: queue is `ch chan string` (tui/bridge.go:17) with capacity from cmd/tui.go (matches MaxRecentEvents); counters `applied`/`dropped` are atomics (tui/bridge.go:25-26).
- Boundary: this change touches only the queue transport and eviction branch in tui/bridge.go; Write's line splitting, the consumer's apply+notify path, Close semantics, and all panel code stay unchanged.

## UI flows
Surface: Recent Events panel (bottom-right) under a log burst (more lines than the panel cap in a short window).

| case | what the user does | what they see |
|---|---|---|
| Burst, queue keeps up | watches the panel | every line appears; nothing dropped (unchanged) |
| Burst, queue overflows | watches the panel | the newest lines keep appearing; the oldest un-shown lines are the ones discarded (NEW — previously the newest lines were silently dropped and the panel froze on the oldest lines of the burst) |
| After a long run past the cap | scrolls to the top | the oldest retained line is the cap-many lines back (unchanged) |
| Normal (no burst) | uses the TUI | identical to today (unchanged) |

No irreversible step: retention is in memory only and resets when the TUI exits.
