# Two-Tier Video Duration Limits + Terminal `.skipped` Marker

## Problem
The single `filter.max_duration` cap conflates two unrelated concerns. Its real
purpose was to bound the **local Whisper transcription** resource cost
(audio→text on CPU is slow/expensive). But it runs as a Phase-1 fetch filter,
so it also blocks long videos that have **downloadable subtitles** — which cost
almost nothing regardless of length. Result: subtitled long videos are wrongly
skipped, and the resource concern is enforced at the wrong stage.

Separately, when a video gets a transcript but summarization keeps failing
(e.g. transcript too long for the model), the resume logic re-attempts it every
watch cycle → a **permanent partial** the user explicitly does not want.

## Users
The maintainer (kouko) running YTSS in `--watch` mode (cron/server, unattended)
and ad-hoc (`channel` / `video`). Wants long subtitled videos processed, the
expensive local path bounded, and no zombie items that retry forever.

## Smallest End State
Two independent, coexisting duration caps + a persisted terminal-skip marker:

| Cap | Stage | Blocks |
|---|---|---|
| `filter.max_duration` = **14400 (4h)** | Phase 1 fetch (existing `FilterVideos`) | Outer ceiling: video skipped entirely, **regardless of subtitles** |
| `whisper.max_duration` = **7200 (2h)** (new) | Phase 2, **only** the no-subtitle→Whisper fallback | Inner cap: protect local transcription |

Invariant: `whisper.max_duration ≤ filter.max_duration`.

Decision flow:
```
fetch:   duration > filter.max_duration (4h)?  → skip entirely (regardless of subs)
process: subtitles available?                  → summarize (length bounded by 4h cap)
         else duration > whisper.max (2h)?      → write .skipped marker, terminal skip
         else                                   → Whisper → summarize
```

Terminal `.skipped` marker:
- File `DATE__videoID__.skipped` (same prefix convention as `__summary.md`).
- Written when the Whisper gate trips. Classified as **skipped** (reuse
  `errSkipped`), not partial/failed.
- Across runs: `BuildIndex` indexes it; the skip checks honor it → never
  retried. `--force` overrides.

## Current State Evidence
- **Forward (entry → behavior):** Fetch-time filter `fetcher.FilterVideos`
  called at `pipeline/pipeline.go:380,408,866`; enforces Min/MaxDuration in
  `fetcher/filter.go:16-21`. Whisper fallback at `pipeline/pipeline.go:609-628`
  (only when `subtitle.Download` errors at :609).
- **Reverse (config SSOT):** `FilterConfig.Min/MaxDuration` at
  `config/config.go:247-248`; `WhisperConfig` struct at `config/config.go:148`,
  defaults block at `config/config.go:316-339` (no duration field yet). User
  config `config.yaml` (`filter.max_duration: 10800`, `whisper:` block).
- **Error/skip path:** `errSkipped` sentinel `pipeline/pipeline.go:1364`;
  `classifyResult` (`:1400`) buckets via `IsSkipped`/`IsPartial`. Skip checks:
  `processChannelVideos` lightweight check `pipeline/pipeline.go:449`
  (`HasFile(id,"summary.md")`, pre full-metadata); `ProcessVideo`
  `IsProcessed` check `:531-535`; resume block `:561-583`.
- **Data (index):** `output/index.go` — `BuildIndex` suffix list `:72`
  (`summary.md`/`transcription.md`/`subtitle.srt`); `HasFile` `:111`;
  `IsProcessed` `:121`; `AddFile` `:174`. `VideoFilePrefix` →
  `DATE__videoID__` (`output/output.go:126-128`).
- **Boundary:** Whisper gate lives in `ProcessVideo` → applies to all three
  commands (`run`/`channel`/`video`). The 4h `filter.max_duration` applies only
  to list paths; single `video <url>` bypasses `FilterVideos` (explicit intent).

## Decision
Add `whisper.max_duration` (default 7200) to `WhisperConfig`; insert a gate
before the Whisper call that, when `meta.Duration > whisper.max_duration`,
writes a `.skipped` marker and returns `errSkipped`. Add `.skipped` to the
index suffix list and to the two skip checks so it is terminal across runs.
Set `config.yaml`: `filter.max_duration: 14400`, `whisper.max_duration: 7200`.

## Out of Scope
- LLM-side pre-flight token gate / map-reduce chunking / truncation — the 4h
  outer cap keeps subtitled transcripts (~50k tokens) within cloud-provider
  context, so over-length summarization is structurally prevented, not handled.
- Circuit-breaker misclassification hardening (separate concern).
- Changing `min_duration` semantics (unchanged; content preference).

## What Becomes Obsolete
The resource-protection meaning of `filter.max_duration`. After this change it
is purely a content ceiling; the resource role moves to `whisper.max_duration`.
Document the split in `config.example.yaml`.
