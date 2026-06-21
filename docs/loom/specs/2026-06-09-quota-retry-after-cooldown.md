# Retry-After-aware, two-tier provider cooldown

## Problem
When an LLM provider returns a quota/rate-limit error, the circuit breaker
opens for one **fixed** cooldown (default 300s) regardless of *why* it failed.
A per-minute rate limit (recovers in ~60s) and a daily quota exhaustion
(recovers in hours) map to the same `QuotaError` and the same cooldown — so
the breaker either probes too eagerly (wasting calls against a still-exhausted
provider every 5 min all day) or too conservatively (idling 240s after a
60s throttle). The server often *tells* us the right wait via `Retry-After`,
but that signal is discarded.

## Users
The CLI batch pipeline (`pipeline.go`, semaphore-bounded **concurrent** worker
pool) summarizing many YouTube videos. One shared `FallbackSummarizer` +
in-memory circuit breakers across all workers. Providers are two kinds:
HTTP (claude-api / openai-compat / ollama — have `Retry-After` headers + status
codes) and CLI (gemini-cli / qwen-code / antigravity-cli / claude-code — only
stderr/stdout text).

## Smallest End State
1. `QuotaError` carries `Kind` (RateLimit | Exhausted) and `RetryAfter`
   (server-advised wait; 0 = unknown).
2. A single tested detector `quotaErrorFrom(...)` replaces the 7 duplicated
   per-provider `if isQuotaMessage(...) { &QuotaError{...} }` sites — parses
   status code (429/529), Retry-After header (seconds or HTTP-date), and
   message text; classifies Kind; broadens quota patterns (#4).
3. Circuit breaker cooldown becomes per-open: honor `RetryAfter` (capped at
   1h) when present; else a tier default — RateLimit → `rateLimitCooldown`,
   Exhausted → existing `cooldown`.
4. `RecordQuotaFailure(retryAfter, kind)` carries the policy; `RecordFailure()`
   delegates with `(0, Exhausted)` so all existing tests stay green.
5. Config gains `rate_limit_cooldown_seconds` (default 60); wired in
   `summarizer.go`. Backward-compat: `newCircuitBreaker` defaults
   `rateLimitCooldown = cooldown`, so existing unit tests' single-cooldown
   semantics are unchanged; two-tier only activates via production config.

## Current State Evidence
- Forward: `fallback.go:43` `IsQuotaError(err)` → `RecordFailure()` (no cooldown arg).
- Reverse: cooldown fixed at `newCircuitBreaker` construction (`summarizer.go:49-61`,
  `circuit_breaker.go:50-58`); `Allow()` open-branch reads `cb.cooldown` (`:69`).
- Error: 7 providers build `&QuotaError{Provider, Err}` only — no Kind/RetryAfter
  (`gemini.go:61`, `claude.go:89`, `openai_compat.go:97`, `ollama.go:94`,
  `qwen_code.go:66`, `claude_code.go:74`, `antigravity.go:61`).
- Data: `quotaPatterns` substring list (`errors.go:31-39`); HTTP providers also
  branch on `resp.StatusCode == 429 / 529`.
- Boundary: `pipeline.go:78` builds one Summarizer; breakers shared across
  concurrent workers — all mutations already mutex-guarded.

## Decision
Build Retry-After-aware two-tier cooldown + a consolidated, broadened quota
detector. Do NOT add same-provider exponential-backoff retry — for a
multi-provider chain, immediate failover usually beats waiting in place
(user-confirmed scope). Jitter (industry practice for backoff) is N/A without
in-provider retry; the cooldown itself is deterministic per server hint.

## Out of Scope
- Same-provider retry loop / exponential backoff / jitter (#3 — deferred).
- Persisting breaker state across process restarts (#6).
- Early-exit when the whole chain is open (#5).

## Alternatives Considered (Axis 4, WebSearch EN+JA — agree)
1. Respect `Retry-After` header first (RFC 9110) — chosen. Server knows its
   window; more precise than client guessing.
2. Pure exponential backoff + jitter — the no-header fallback; folded into the
   tier-default cooldown (no in-provider retry, so no jitter needed here).
3. Fixed single cooldown (status quo) — rejected: conflates the two failure
   regimes, the root cause.

Sources: zuplo.com/learning-center/http-429-too-many-requests-guide (EN);
sophiate.co.jp 429リトライ&バックオフ設計 (JA) — both endorse the
"Retry-After尊重 + 指数バックオフ + ジッター + 最大回数" four-piece set.
