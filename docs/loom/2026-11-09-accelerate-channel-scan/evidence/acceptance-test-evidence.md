# Accelerate channel scan — acceptance test evidence

Tried on 2026-11-09, in a clean copy of the project at 54a7492 (54a74920f8c17002bb104b7367785c1fb6bd07f6).

## Setup

- Clean git worktree at 54a7492 (detached HEAD), created with `git worktree add`; working tree clean, nothing uncommitted from the change.
- README setup check: `go build ./...` → OK; `go vet ./...` → OK.
- Binary smoke test: `go build -o ytss-smoke . && ./ytss-smoke --help` → help renders; CLI flags surface unchanged (`--dry-run`, `--force`, `-o/--output`, `-c/--config`, `-v` all present). The functional diff touches only `pipeline/pipeline.go` (early checks + a handle-parsing helper) and `pipeline/pipeline_test.go` (new tests) — no CLI, config-schema, output-structure, TUI, embedded-binary, or yt-dlp-argument changes.
- Setup note (environment, not the change): README's `make build` compiles ffmpeg + whisper from source and needs cmake and nasm (README prerequisites). nasm is not installed on this machine, so `make build` cannot complete here. The embedded tool binaries are gitignored build artifacts (`embedded/bin/` is in `.gitignore`); the already-provisioned darwin-arm64 binaries (yt-dlp, ffmpeg, whisper-cli, built 2026-11-09 02:10 in the working repo) were copied into the clean worktree so the Go code builds and tests. The tested source tree is exactly 54a7492.
- The full package suite (`go test ./...`) was NOT run by this test — it is left to finalize-review, which executes it before the change is accepted and blocks on failure. Only the named criterion tests below were run.
- Scratch harness: `pipeline/zz_acc_harness_test.go`, written by the acceptance tester in the clean worktree, never committed with the change. It drives the real Pipeline — real on-disk video index, real `ProcessChannel`/`processChannelVideos`/`ProcessVideo` — with fake fetcher/subtitle/summarizer seams, and captures `log/slog` output at Debug level so the "fetching full metadata" debug line can be asserted absent/present. `FetchVideoMeta` (the only seam that invokes yt-dlp `--dump-json` for per-video metadata) counts every call.
- RED base for comparison: second clean worktree at the base commit e801bba (origin/main), with the new tests from HEAD copied into it.

## 1. Scan or dry-run of an all-processed channel makes no per-video full-metadata fetch and reports all skipped

- How I tried it: harness `TestAcc_A1_ScanAllProcessed_NoFetch_AllSkipped` — output dir seeded with 3 processed videos (each under `@testhandle/…` with a `summary.md`), fake channel listing returns those 3 videos, run `ProcessChannel` twice (once normal scan, once with dry-run):
  ```
  go test ./pipeline -run 'TestAcc_A1' -v
  ```
- What came back: PASS for both subtests. `FetchVideoMeta` calls = 0 in both; stats = skipped 3, success 0, failed 0. Captured run log (Debug level enabled, so the debug fetch line would be visible):
  ```
  msg="fetched channel tab" tab=/videos url=https://www.youtube.com/@testhandle/videos fetched=3 limit=10
  msg="total filtered videos across tabs" count=3
  msg="[1/3] accA1Vid01 - skipped (already processed)" title="Video One"
  msg="[2/3] accA1Vid02 - skipped (already processed)" title="Video Two"
  msg="[3/3] accA1Vid03 - skipped (already processed)" title="Video Three"
  ```
  Zero occurrences of "fetching full metadata". (Dry-run subtest additionally asserts no "would process (dry run)" appears for already-processed videos — they are reported skipped.)
- Committed tests: `TestProcessVideo_EarlySkip/already-processed_skips_metadata_fetch` — at base e801bba: FAIL (`FetchVideoMeta calls = 1, want 0`, error instead of skip) → at HEAD 54a7492: PASS (skipped, 0 fetch calls).
- Evidence: `pipeline/pipeline.go:621-627` (early skip guarded by `!force`), harness assertions and captured log above, RED→GREEN table below.

## 2. New (unprocessed) videos behave as today: full metadata fetch then subtitle/transcription/summary

- How I tried it: harness `TestAcc_A2_NewVideo_FetchThenExistingFlow` — empty output dir, fake channel listing returns 1 unprocessed video whose full-metadata fetch succeeds, fake subtitle download succeeds with SRT content:
  ```
  go test ./pipeline -run 'TestAcc_A2' -v
  ```
- What came back: PASS. `FetchVideoMeta` calls = 1 (exactly one); fake subtitle downloader invoked; captured run log:
  ```
  msg="fetching full metadata" video_id=accA2NewVid
  msg="subtitle download succeeded" video_id=accA2NewVid language=zh type=manual
  msg="stage 1: generating summary" video_id=accA2NewVid
  msg="stage 1 complete" video_id=accA2NewVid provider=fake model=m
  msg="video processing complete" video_id=accA2NewVid output_dir=…/@testhandle/2024-01-01__accA2NewVid__Brand_New_Video
  ```
  Output dir contains `*__subtitle.srt`, `*__transcription.md`, `*__summary.md` (verified by glob); stats success=1, skipped=0.
- Committed tests: `TestProcessVideo_EarlySkip/unindexed_video_fetches_exactly_once` — passes at both base e801bba and HEAD (unchanged behavior: new videos fetch once).
- Evidence: `pipeline/pipeline.go:640-657` (fetch branch unchanged), `pipeline/pipeline.go:734-824` (subtitle → transcription → summary flow unchanged), harness assertions above.

## 3. Dry-run completes with no yt-dlp full-metadata fetch records; output shows only "would process (dry run)"

- How I tried it: harness `TestAcc_A3_DryRun_NoFetch_OnlyWouldProcess` — empty output dir, fake channel listing returns 3 unprocessed videos, `dryRun: true`:
  ```
  go test ./pipeline -run 'TestAcc_A3' -v
  ```
- What came back: PASS. `FetchVideoMeta` calls = 0; captured run log (Debug level):
  ```
  msg="[1/3] accA3Vid01 - processing" title="New One"
  msg="would process (dry run)" video_id=accA3Vid01 title="New One"
  msg="[2/3] accA3Vid02 - processing" title="New Two"
  msg="would process (dry run)" video_id=accA3Vid02 title="New Two"
  msg="[3/3] accA3Vid03 - processing" title="New Three"
  msg="would process (dry run)" video_id=accA3Vid03 title="New Three"
  ```
  Exactly 3× "would process (dry run)", zero "fetching full metadata", stats skipped=3; output dir still empty after the run (nothing created, nothing fetched).
- Committed tests: `TestProcessVideo_EarlySkip/dry-run_skips_metadata_fetch` — at base e801bba: FAIL (`FetchVideoMeta calls = 1, want 0`, error instead of skip — dry-run used to fetch) → at HEAD 54a7492: PASS.
- Evidence: `pipeline/pipeline.go:629-633` (dry-run early return before the fetch branch), harness assertions and captured log above.

## 4. Handle mismatch (renamed channel): no error, no duplicate output; worst case falls back to today's flow

- How I tried it: two harness tests.
  - `TestAcc_A4_RenamedHandle_FetchOnce_PostFetchSkip_NoDuplicate`: output dir has the video processed under `@oldhandle`; config channel URL is `@newhandle`; full-metadata fetch returns metadata reporting `@oldhandle`. Drive `ProcessVideo` directly with the renamed-handle channel config.
  - `TestAcc_A4b_RenamedHandle_ChannelScan_NoErrorNoDuplicate`: the same scenario through the full `ProcessChannel` scan.
  ```
  go test ./pipeline -run 'TestAcc_A4' -v
  ```
- What came back: PASS for both. A4 (ProcessVideo, worst case): early check misses (handle mismatch) → exactly one DEBUG "fetching full metadata" → INFO "skipped (already processed)" (post-fetch backstop):
  ```
  msg="fetching full metadata" video_id=accA4Vid01
  msg="skipped (already processed)" video_id=accA4Vid01
  ```
  `IsSkipped(err)` = true (no error); output dir still contains only `@oldhandle` (no duplicate directory, nothing written). A4b (full scan): skipped=1, failed=0, only `@oldhandle` remains.
- Committed tests: `TestProcessVideo_EarlySkip/renamed_handle_falls_through_then_post-fetch_skips` and `/force_bypasses_early_processed_check` — pass at both base e801bba and HEAD (the fallback and `--force` semantics are preserved, no regression).
- Evidence: `pipeline/pipeline.go:621-627` (early check), `pipeline/pipeline.go:662-668` (post-fetch backstop, untouched), harness assertions and captured log above.

## RED → GREEN (new tests vs the change)

Base worktree at e801bba (pre-change), with the new tests from HEAD copied in:

- `TestExtractHandleFromURL` → build error: `undefined: extractHandleFromURL` (the parser is new in this change) — RED.
- `TestProcessVideo_EarlySkip/dry-run_skips_metadata_fetch` → FAIL: `IsSkipped(err) = false, want true`; `FetchVideoMeta calls = 1, want 0` — RED (dry-run used to fetch).
- `TestProcessVideo_EarlySkip/already-processed_skips_metadata_fetch` → FAIL: `IsSkipped(err) = false, want true`; `FetchVideoMeta calls = 1, want 0` — RED (processed videos used to fetch).
- `TestProcessVideo_EarlySkip/unindexed_video_fetches_exactly_once` → PASS (unchanged-behavior baseline).
- `TestProcessVideo_EarlySkip/renamed_handle_falls_through_then_post-fetch_skips` → PASS (fallback existed before; this change must not break it).
- `TestProcessVideo_EarlySkip/force_bypasses_early_processed_check` → PASS (`--force` semantics existed before).

HEAD 54a7492: all of the above PASS — GREEN.

## Re-run

- n/a — first acceptance run; no fix has been made.
