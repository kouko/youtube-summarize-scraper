# Event bridge keeps latest events under burst — plan
intent: 2026-10-24-bridge-keep-latest-events@a33539d
spec: docs/loom/2026-10-24-bridge-keep-latest-events/spec.md@e2daf55
charter: 1.1

## Task DAG
<!-- When a spec requirement changes after this commit, the un-landed
     tasks it touches are replaced and the reason is named in the commit
     message. Landed tasks stay as they are. -->

Wave 1 — queue transport rewrite (drop-new → drop-oldest, deterministic window)

**W1-01 Mutex deque with drop-oldest eviction**  after: none  acceptance: 1, 2
- Files: tui/bridge.go, tui/bridge_test.go
- Test: A1 positive: 110 writes at cap 100 retain lines 10–109 in order; boundary: any consumer speed keeps the contiguous newest suffix. A2 positive: Applied+Dropped equals written count; boundary: 1000 writes at cap 1 never block.
- Risk: channel cannot evict its oldest element; a non-blocking receive races the consumer and can shift the evicted line by one at the window edge — mutex deque serializes detect-full/evict/append (agent-decided, simplest race-free shape).

**W1-02 Regression guard**  after: W1-01  acceptance: 3, 4
- Files: tui/bridge_test.go
- Test: A3 positive: existing bridge tests (multi-line, fragment, notify, Close) stay green; boundary: writes after Close are counted as dropped. A4 positive: CI test job green; boundary: PR #72 rerun turns green.
- Risk: guard only; queue rewrite isolated to W1-01 files — no product surface beyond the retention window.

## Simplicity check
- keep the channel, evict on full with a second non-blocking receive — declined: detect-full and evict are not atomic, so the consumer's concurrent receive can shift the evicted line by one and drop a line from the newest-capacity window (spec Alternatives).
- raise the queue capacity until bursts fit — declined: any fixed capacity can be exceeded; the bug is eviction semantics, not size (spec Alternatives).

## Questions asked
capture-intent ① — what — 佇列滿時丟哪一行？現況丟新的，面板凍在爆量那批的開頭。 (answer: 方案 A — 修橋，保留最新)
capture-intent ① — what — 「爆量」指短時間內日誌超過面板上限 1000 行、佇列必須開始丟行的情境；修成丟最舊、保留最新，對嗎？ (answer: 對)
write-spec ② — behaviour — 爆量時面板永遠顯示最新的活動（最新行一直出現、被丟的是最舊行）；沒有爆量時行為完全不變——對嗎？ (answer: 對，就是這個)

## Risks
1. The mutex sits on the pipeline's log-write path — single producer, lock hold is one slice append/trim plus a cond signal (nanoseconds); non-blocking semantics are preserved because a full queue evicts instead of blocking (spec REQ-3).
2. Close now deterministically abandons queued lines where the old select chose randomly between done and a line — the bridge's own doc comment already documents "Lines queued but not consumed are abandoned", and no existing test pins the random behaviour.
