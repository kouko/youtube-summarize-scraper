# Brief: Add `antigravity-cli` summarizer backend (keep `gemini-cli`)

Date: 2026-05-31
Topic: New LLM backend wrapping Google Antigravity CLI (`agy`) headless mode

## Problem
(Axis 1 — JTBD)
When [Google shuts off free/Pro/Ultra Gemini CLI on 2026-06-18], kouko wants
[an `agy`-based backend already wired into YTSS as a selectable alternative],
so he can [keep a Google-model summarization path without losing the existing
`gemini-cli` backend that enterprise/paid-API-key users still rely on].

The job is *optionality*, not replacement: add `antigravity-cli` alongside
`gemini-cli`, change nothing about the existing backend.

## Users
(Axis 2)
- kouko, on macOS (zsh), `agy` 1.0.3 already installed (`/opt/homebrew/bin/agy`).
- Runs `ytss` to batch-summarize YouTube transcripts locally.
- Distribution: Homebrew tap → macOS + Linux. **Not Windows** (the known
  `agy -p` stdout bug is Windows-only — confirmed gemini-cli#27466).

## Smallest End State
(Axis 3)
A new provider `antigravity-cli` that mirrors the existing `gemini-cli` headless
invocation, selectable via config `llm.provider` or `--llm antigravity-cli`:
- `summarizer/antigravity.go`: `AntigravityCLISummarizer` — exec `agy`,
  prompt via **stdin**, args `--print-timeout <t> -p`, read **stdout**,
  reuse `StripThinkingTags` + `isQuotaMessage`.
- `config.AntigravityCLIConfig{ Path, Timeout }` + `LLMConfig` field
  `yaml:"antigravity-cli"`.
- Factory `case "antigravity-cli"` in `newSingleProvider`.
- Docs: `config.example.yaml`, README backend list, `--llm` flag help.

Existing `gemini-cli` path untouched. Fallback chain + circuit breaker come
free via the `Summarizer` interface.

## Current State Evidence
- **Forward (how a backend is born):** `summarizer/summarizer.go:86` `newSingleProvider`
  switch; `gemini-cli` case at `summarizer.go:118-127` reads
  `cfg.GeminiCLI.{Model,Path,Timeout}`, default timeout 15m.
- **Reverse (who owns the result shape):** all backends return
  `SummarizeResult{Text,Provider,Model}` (`summarizer.go:22-26`); pipeline +
  fallback consume the interface `Summarizer.Summarize` (`summarizer.go:29-31`).
  No distribution/sync script — single Go module, factory is SSOT for provider names.
- **Error path:** `gemini.go:58-64` wraps exec failure; `isQuotaMessage(combined)`
  → `&QuotaError{}` drives circuit-breaker skip (`summarizer.go:48-61`).
- **Data:** config struct `GeminiCLIConfig` at `config/config.go:186-190`
  (`Model/Path/Timeout`), registered in `LLMConfig` at `config.go:159`.
- **Boundary:** `cmd/root.go` `--llm` flag enumerates valid provider strings
  (README:106); `config.example.yaml` documents each `llm.<provider>` block.
- Evidence paths: summarizer/summarizer.go, summarizer/gemini.go,
  config/config.go, README.md, config.example.yaml, cmd/root.go.

## Decision
Build a faithful mirror of the `gemini-cli` backend named `antigravity-cli`,
reading the response from **stdout** (works on macOS+Linux, our only targets).
Reuse `StripThinkingTags` and `isQuotaMessage`. Do **not** add a `Model` config
field — `agy` has no per-call `--model` flag (model is set interactively via
`/model` and persists); a silent-no-op field would mislead. Document the
limitation in `config.example.yaml`.

## Alternatives Considered
(Axis 4 — grounded in this session's empirical tests + official docs + GitHub issues)
1. **stdout read (CHOSEN)** — mirror `gemini.go`. Pros: matches existing pattern,
   ~70 LOC, tested working on macOS 1.0.3. Cons: would break on Windows headless
   (gemini-cli#27466) — irrelevant, we don't ship Windows.
2. **transcript.jsonl read** (parse `~/.gemini/antigravity-cli/brain/<id>/.../transcript.jsonl`,
   `source=MODEL,status=DONE`). Pros: survives the Windows stdout bug; what the
   Claude-Code-Antigravity bridge does. Cons: couples to undocumented internal
   file layout, fragile across versions, more code. Rejected — over-engineering
   for our targets.
3. **Skip agy, build Gemini API (HTTP) backend.** Pros: official, model-selectable,
   pure function, no quota-regression. Cons: different task (the user explicitly
   asked for an agy backend now). Deferred to a separate future change.

My take: #1 now (satisfies the explicit ask, smallest), keep #3 on the roadmap
as the durable path.

## What Becomes Obsolete
(Axis 5)
Nothing is removed — this is purely additive (a new provider alongside existing
ones). That additivity is justified by the explicit "keep gemini-cli" requirement,
not YAGNI: it is a hedge against the 6/18 shutdown, not speculative generality.

## Out of Scope
- Removing / deprecating `gemini-cli`.
- Gemini API (HTTP) backend (alternative #3) — separate future change.
- `--model` / model-selection support for agy (no flag exists; out of our control).
- `--sandbox` / `enableTerminalSandbox` wiring — agy print mode doesn't invoke
  tools on plain summarization input; sandbox doesn't prevent its file-state
  writes anyway. Can add a `Sandbox bool` knob later if needed.
- transcript.jsonl fallback for Windows.
- Empty-output / agentic-derailment guard — defer unless real transcripts trigger it.

## Open Questions
- **Quota error wording**: `isQuotaMessage` matches Gemini-CLI phrasing; `agy`'s
  weekly-quota error string may differ. Reuse as-is for now; tune if a real quota
  error doesn't trip the circuit breaker. (Generic exec error still surfaces.)
- **Default model quality**: agy headless uses its persisted/default model
  (reportedly Gemini 3.5 Flash). Acceptable for a hedge backend; noted for the user.
