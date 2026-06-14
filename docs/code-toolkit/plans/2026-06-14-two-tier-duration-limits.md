# Plan: Two-Tier Video Duration Limits + Terminal `.skipped` Marker

Source brief: docs/code-toolkit/specs/2026-06-14-two-tier-duration-limits.md
Total tasks: 8 (Task 4b added during execution — see Notes)
Critical-path depth: 3 (≤5)
Execution order: parallel-where-possible
Plan-document-reviewer verdict: PASS (2026-06-14)

Dependency levels:
- Level 0 (roots, parallel — disjoint files): Task 1 (config struct, config/), Task 2 (index suffix, output/), Task 4a (subtitle seam, pipeline/)
- Level 1: Task 3 (dep 2), Task 4 (dep 1+2+4a), Task 5 (dep 2), Task 6 (dep 1)
- Tasks 4a, 4, 5 all touch `pipeline/pipeline.go` → sequential to each other.

## Task 1 — Add `whisper.max_duration` config field + default
- Description: Add `MaxDuration int \`yaml:"max_duration"\`` to `WhisperConfig`; set default `7200` in the Whisper defaults block.
- Module: config
- Files touched: config/config.go, config/config_test.go
- Context paths:
  - config/config.go:148 (WhisperConfig struct)
  - config/config.go:316-339 (Whisper defaults block)
  - config/config_test.go:155-178 (existing duration-parse test pattern)
- Acceptance:
  - RED: `TestWhisperConfig_MaxDuration` — default config yields `cfg.Whisper.MaxDuration == 7200`; a yaml fixture `whisper: { max_duration: 1800 }` parses to 1800.
  - GREEN: both assertions pass.
- Dependencies: none
- Independent: true
- Brief item covered: "`whisper.max_duration` = 7200 (new field on WhisperConfig)".

## Task 2 — Index recognizes `.skipped` marker files
- Description: Add `.skipped` to the suffix list `BuildIndex` scans, so `DATE__videoID__.skipped` files register as a known file flag.
- Module: output
- Files touched: output/index.go, output/index_test.go
- Context paths:
  - output/index.go:67-77 (file-suffix scan loop)
  - output/index.go:110-117 (HasFile)
  - output/index_test.go (existing BuildIndex test pattern)
- Acceptance:
  - RED: `TestBuildIndex_SkippedMarker` — a video dir containing `DATE__vid__.skipped` → after `BuildIndex`, `HasFile(vid, ".skipped")` is true.
  - GREEN: assertion passes; existing index tests stay green.
- Dependencies: none
- Independent: true
- Brief item covered: "Add `.skipped` to the index suffix list ... terminal across runs".

## Task 3 — `IsProcessed` treats `.skipped` as terminal
- Description: Make `VideoIndex.IsProcessed` return true when the entry has a `.skipped` flag (in the expected channel dir), in addition to `summary.md`. This is the ONLY terminal-skip guard on the `video <url>` single-command path (`cmd/video.go:45` → `ProcessVideo` → `IsProcessed` at pipeline.go:532), which bypasses the loop pre-checks of Tasks 5.
- Module: output
- Files touched: output/index.go, output/index_test.go
- Context paths:
  - output/index.go:119-132 (IsProcessed)
- Acceptance:
  - RED: `TestIsProcessed_SkippedMarker` — entry with `.skipped` and no `summary.md`, in the expected `@channel` dir → `IsProcessed` true.
  - GREEN: assertion passes; existing IsProcessed test (summary.md path) stays green.
- Dependencies: Task 2 completes first
- Independent: false
- Brief item covered: "the skip checks honor it → never retried" (single-`video`-command coverage).

## Task 4a — Introduce `subtitleDownloader` interface seam
- Description: Add a `subtitleDownloader` interface (one method matching `subtitle.Downloader.Download`'s exact signature) mirroring the existing `videoFetcher interface`; change the `Pipeline.subtitle` field from `*subtitle.Downloader` to `subtitleDownloader`. `NewPipeline` keeps wiring the concrete `*subtitle.Downloader`. This is the injection seam that makes the Whisper-gate branch (Task 4) testable; without it `subtitle` is a concrete type with no way to force the no-subtitle branch in a unit test.
- Module: pipeline
- Files touched: pipeline/pipeline.go, pipeline/pipeline_test.go
- Context paths:
  - pipeline/pipeline.go:27-32 (videoFetcher interface — the pattern to mirror)
  - pipeline/pipeline.go:39 (subtitle field), pipeline/pipeline.go:73+ (NewPipeline wiring)
  - pipeline/pipeline.go:601-608 (subtitle.Download call site — exact signature)
- Acceptance:
  - RED: `TestPipeline_SubtitleSeam` — a test-local fake implementing `subtitleDownloader` is assigned to `Pipeline.subtitle` and `ProcessVideo` exercises it. Before the change the concrete field type rejects the fake (compile error / un-assignable), so the test cannot be written; after, it compiles and runs.
  - GREEN: fake is assignable and used; `NewPipeline` still constructs the concrete downloader; existing pipeline tests stay green.
- Dependencies: none
- Independent: true
- Brief item covered: enabling seam for "Whisper gate inserted before the Transcribe call" (Decision section).

## Task 4 — Whisper gate writes `.skipped` and terminal-skips
- Description: In `ProcessVideo`, in the subtitle-download-failed branch, BEFORE calling `transcriber.Transcribe`, if `whisper.max_duration > 0 && meta.Duration > whisper.max_duration`: write `<filePrefix>.skipped` marker to the video dir, register it via `index.AddFile(id, ".skipped")`, and return `errSkipped` (do NOT transcribe).
- Module: pipeline
- Files touched: pipeline/pipeline.go, pipeline/pipeline_test.go
- Context paths:
  - pipeline/pipeline.go:595-643 (subtitle cascade + whisper fallback branch)
  - pipeline/pipeline.go:1363-1364 (errSkipped sentinel)
  - output/output.go:126-128 (VideoFilePrefix)
  - Task 4a's `subtitleDownloader` fake (force the no-subtitle branch)
- Acceptance:
  - RED: `TestProcessVideo_WhisperGate_TooLong` — fake `subtitleDownloader` returns a "no subtitles" error and `meta.Duration` is over the cap → after `ProcessVideo`: a `*__.skipped` marker file exists on disk, the return error satisfies `IsSkipped`, and NO `*__transcription.md` was written (proves the transcriber path was not taken). No transcriber spy required — the absent transcription.md + IsSkipped return are the observables.
  - GREEN: assertion passes; under-cap no-subtitle case still proceeds to transcription (existing behavior unchanged — guard by an over-cap vs under-cap table case).
- Dependencies: Tasks 1, 2, 4a complete first
- Independent: false
- Brief item covered: "Whisper gate inserted before the Transcribe call ... writes a terminal `.skipped` marker and returns `errSkipped`".

## Task 4b — Mirror the Whisper gate in the playlist path
- Description: `processVideoInPlaylist` has the identical subtitle-failed → `transcriber.Transcribe` branch (pipeline.go:~1081) with NO gate, so playlist runs would still burn local Whisper on over-cap no-subtitle videos. Insert the same gate as Task 4 (write `<filePrefix>.skipped`, `index.AddFile(id,".skipped")`, return `errSkipped`) before that Transcribe call. **Surfaced during Task 4 implementation — coverage gap, not in the original plan.**
- Module: pipeline
- Files touched: pipeline/pipeline.go, pipeline/pipeline_test.go
- Context paths:
  - pipeline/pipeline.go:615-639 (the Task 4 gate in ProcessVideo — the pattern to mirror)
  - pipeline/pipeline.go:~1081 (the playlist subtitle-failed → Transcribe branch)
- Acceptance:
  - RED: `TestProcessVideoInPlaylist_WhisperGate_TooLong` — fake subtitle "no subtitles" + over-cap Duration → `*__.skipped` written, `IsSkipped` returned, no `*__transcription.md`; under-cap does not fire.
  - GREEN: passes; existing playlist tests stay green.
- Dependencies: Tasks 1, 2, 4a, 4 complete first (shares pipeline.go with Task 4)
- Independent: false
- Brief item covered: "無字幕的長片不會拖垮本機 Whisper" — extended to the playlist code path.

## Task 5 — Loop pre-checks (channel + playlist) honor `.skipped`
- Description: Extend the lightweight already-processed check (before full-metadata fetch) in BOTH loops so a video whose index entry has `.skipped` is counted as skipped without dispatching the per-video pipeline. Two identical sites: `processChannelVideos:450` and `processPlaylistVideos:885` (both currently `existingDir != "" && HasFile(id,"summary.md")` → add `|| HasFile(id,".skipped")`).
- Module: pipeline
- Files touched: pipeline/pipeline.go, pipeline/pipeline_test.go
- Context paths:
  - pipeline/pipeline.go:447-458 (channel loop pre-check)
  - pipeline/pipeline.go:883-906 (playlist loop pre-check — same shape)
- Acceptance:
  - RED: `TestLoopPreChecks_SkipMarked` — table test over channel + playlist: a video with a `.skipped` index flag → `stats.Skipped` incremented and the per-video function not entered (assert via no new files written for that video). Both sites covered.
  - GREEN: assertions pass; the existing summary.md skip path stays green for both loops.
- Dependencies: Task 2 completes first
- Independent: false
- Brief item covered: "`processChannelVideos:449` ... `HasFile(id,\"summary.md\") || HasFile(id,\".skipped\")` → next round directly skipped" (extended to the mirrored playlist site).

## Task 6 — Apply config values + document the two-tier split
- Description: Set `config.yaml`: `filter.max_duration: 14400` and add `whisper.max_duration: 7200`. In `config.example.yaml`, add a commented `whisper.max_duration` with a note that `filter.max_duration` is the content ceiling and `whisper.max_duration` bounds local transcription only (invariant `whisper ≤ filter`).
- Module: config (yaml data + docs)
- Files touched: config.yaml, config.example.yaml
- Context paths:
  - config.yaml:22-31 (whisper block), config.yaml:96-100 (filter block)
  - config.example.yaml:27-36 (whisper block), config.example.yaml:124-131 (filter block)
- Acceptance:
  - RED (config exemption — verified by grep, not a Go test): `whisper.max_duration` absent from config.example.yaml; `filter.max_duration` is 10800 in config.yaml.
  - GREEN: config.yaml has `filter.max_duration: 14400` + `whisper.max_duration: 7200`; config.example.yaml adds a commented `whisper.max_duration` + the split rationale (its `filter.max_duration` stays `0` = documented no-limit default — do NOT change the example's filter value); `go test ./config/` stays green (values parse).
- Dependencies: Task 1 completes first
- Independent: false
- Brief item covered: "Set `config.yaml`: `filter.max_duration: 14400`, `whisper.max_duration: 7200`" + "Document the split in `config.example.yaml`".

## Notes
- `min_duration` semantics unchanged (content preference).
- No LLM-side token gate / chunking (Out of Scope per brief — 4h cap keeps subtitled transcripts within cloud context).
- Single `video <url>` command bypasses `filter.max_duration` (FilterVideos not on that path) but DOES hit the Whisper gate (Task 4 lives in ProcessVideo) — intended.
