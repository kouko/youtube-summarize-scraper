# TUI 執行紀錄:把日誌寫成輸出資料夾裡的純文字檔 — acceptance test evidence

Tried on 2026-11-08, in a clean `git worktree` of `deb866f` (branch
`feat/2026-11-08-tui-log-file`). Embedded tool binaries were copied from the
main checkout into the gitignored `embedded/bin` and never committed.

Setup check: `go build ./...` succeeds in the clean worktree; `go run . tui`
launches and the picker/config/run/quit flow below works.

The probes that exercised the real run path lived in an untracked file,
`tui/zz_acceptance_probe_test.go`, deleted after the run; they wired logging
exactly as `cmd/tui.go:27-42` does (`NewEventBridge` → `tui.LogTarget` →
`slog.NewTextHandler(tui.LogTarget, nil)`, `subtitle.DefaultStderr =
io.Discard`, `transcriber.DefaultStderr = io.Discard`, `log.SetOutput(io.Discard)`)
and called `runPipelineWithConfig` directly, which is the same entry point the
TUI's `r` key uses (`tui/model.go:535`).

Raw captures kept next to this file: `raw-probe-runs.log`,
`raw-probe-redline.log`, `raw-runlog-tests.log`, `raw-model-tests.log`,
`raw-tui-drive.log`, `run-a/`, `run-c/`, `run-tui-smoke/`, `drive_tui.py`.

## 1. 執行一次批次(或單支影片)後,輸出資料夾的 logs 子目錄內出現日誌檔,內容包含該次執行的處理訊息(例如影片處理的各階段)。

- How I tried it: a real single-video run through the TUI entry point, with the
  real embedded `yt-dlp` and a real Ollama summarizer:
  `ACCEPTANCE_EVIDENCE_DIR=… go test ./tui -run TestAcceptanceProbeRealVideoRuns -v -count=1`
  Config: one playlist `https://www.youtube.com/watch?v=dQw4w9WgXcQ`, count 1,
  `llm.provider: ollama`, `model: qwen3.5:9b`, output dir a fresh directory.
  Before the run: `logs/` does not exist.
- What came back: `PROBE_BEFORE_RUN logs-dir-exists=false`;
  `PROBE_RUN1 duration=1m23.228685167s log-lines=12 mid-run-file-had-content=true`;
  the file is at `<output_dir>/logs/ytss.log` and holds the run's stage records,
  e.g.:
  - `msg="built video index"`
  - `msg="streaming: starting producers" playlists=1 channels=0 concurrency=1`
  - `msg="[1/1] dQw4w9WgXcQ - processing" title="Rick Astley - Never Gonna Give You Up (Official Video) (4K Remaster)"`
  - `msg="subtitle download succeeded" video_id=dQw4w9WgXcQ language=en type=manual`
  - `msg="stage 1: generating summary" video_id=dQw4w9WgXcQ`
  - `msg="stage 2: extracting keywords" video_id=dQw4w9WgXcQ`
  - `msg="video processing complete" video_id=dQw4w9WgXcQ`
  - `msg="streaming batch complete" success=1 skipped=0 partial=0 failed=0`
  `find` shows `<output_dir>/logs/ytss.log` beside the run's playlist output.
  The file was also observed growing during the run
  (`mid-run-file-had-content=true`, first mid-run line
  `msg="built video index"`).
- Evidence: `raw-probe-runs.log`; `run-a/ytss-output/logs/ytss.log`.
  A second, real TUI-process run (pty, keys `Enter` `Enter` `r` `q`) created
  `/tmp/ytss-smoke-out/logs/ytss.log` on first use: `raw-tui-drive.log`,
  `run-tui-smoke/ytss.log`.
- Named test: `TestRunLogFileMatchesRecentEvents` (PASS).

## 2. 日誌檔是純文字,用一般編輯器開啟後可以正常選取並複製內容。

- How I tried it: on the real run's file (`run-a/ytss-output/logs/ytss.log`):
  `file`, `wc -lc`, an ANSI-escape count, a NUL-byte count, and a clipboard
  round-trip with the general text tools `pbcopy`/`pbpaste` plus `cmp`.
- What came back:
  - `file`: `Unicode text, UTF-8 text, with very long lines (366)`
  - `wc -lc`: `18 2757`
  - ANSI escape count: `0`; NUL-byte count: `2757` (file size unchanged, i.e. no NULs)
  - `cat … | pbcopy && pbpaste | cmp -s - …` → `pbpaste identical to file`
- Evidence: commands above; the file itself under `run-a/`.
- Named tests: `TestRunLogFileIsPlainText`, `TestRunLogFileLinesAreSlogText`
  (PASS; asserts plain UTF-8 slog text lines, LF endings, and no ANSI escapes).

## 3. 連續執行兩次,日誌檔保留兩次的紀錄;第二次執行不會清除或覆寫第一次的內容。

- How I tried it: the same config and output dir were run twice back to back
  (`TestAcceptanceProbeRealVideoRuns`), then the two contents were compared
  byte-wise; a separate named test restarts the whole handler/model stack
  between runs (a TUI process restart). A watch-loop iteration test also covers
  repeated runs.
- What came back:
  - `PROBE_RUN2 duration=17.699321333s log-lines=18`
  - `PROBE_RUN2 first-run-content-preserved=yes run1-lines=12 run2-lines=18`
  - `bytes.HasPrefix(run2, run1)` held; the 6 new lines are the second run's
    (`built video index` … `[1/1] dQw4w9WgXcQ - skipped (complete)` …
    `streaming batch complete success=0 skipped=1`).
  - The file is opened append-only (`O_CREATE|O_APPEND|O_WRONLY`, no `O_TRUNC`,
    `tui/logwriter.go:71`).
- Evidence: `raw-probe-runs.log`; `run-a/ytss-output/logs/ytss.log`
  (lines 13-18 are the second run's records).
- Named tests: `TestRunLogFileAppendsAcrossRuns`,
  `TestRunLogFileAppendsAcrossRestart`, `TestRunLogWatchLoopAppendsAcrossIterations`,
  `TestRunLogWatchReloadKeepsWriterPath` (all PASS).

## 4. 日誌檔內容只來自程式自己的日誌;外部抓取與轉錄工具的原始進度輸出不會出現在檔案中。

- How I tried it: on the real run's file: every non-empty line was checked to
  be a slog record (`time=` prefix and ` msg=`), valid NUL-free UTF-8, and free
  of external-tool progress markers (`[download]`, `[youtube]`,
  `whisper_print`). The probe's real `yt-dlp` calls produced their normal
  progress on the discarded sinks. A named test additionally injects a raw
  `[download] 45.2% …` progress line into the subprocess stderr sink and proves
  it never reaches the file.
- What came back:
  - `PROBE_RUN1 all-12-lines-program-slog-records=yes`
  - `grep -cv '^time=' ytss.log` → `0`
  - `grep -cE '\[download\]|\[youtube\]|whisper_print|%\|' ytss.log` → `0`
- Evidence: `raw-probe-runs.log`; `run-a/ytss-output/logs/ytss.log`.
- Named tests: `TestRunLogFileHoldsOnlyProgramRecords`,
  `TestRunLogFileExcludesSubprocessStderr` (PASS).

## 5. 日誌檔中不出現 cookie、keychain 內容或 API key 值(掃描檔案內常見機密格式即可判別)。

- How I tried it: the real run's config carried a cookie file with the literal
  value `COOKIE_SECRET_7d1c9a_acceptance` and a `claude-api` key
  `sk-live-0123456789abcdefghij`; the loaded config was checked to actually
  contain both before scanning the file. The file was scanned for those two
  literals and for common credential shapes: `sk-[A-Za-z0-9_-]{16,}`,
  `AIza[0-9A-Za-z_-]{20,}`, `(password|secret|api[_-]?key)\s*[=:]\s*\S+`,
  `cookie:\s*\S+=\S+`. A named test additionally proves a configured cookie
  value never passes through the tee.
- What came back:
  - `PROBE_SECRETS configured-cookie="COOKIE_SECRET_7d1c9a_acceptance" configured-api-key="sk-live-0123456789abcdefghij" log-hits=[]`
- Evidence: `raw-probe-runs.log`; `run-a/ytss-output/logs/ytss.log`.
- Named tests: `TestRunLogFileHasNoSecretPatterns`,
  `TestRunLogFileOmitsConfiguredCookieValue` (PASS).

## 6. TUI 畫面(事件面板、按鍵)的行為不變。

- How I tried it: (a) a real pty session of the built binary
  (`YTSS_CONFIG_DIR=… ytss tui`, 120x30): startup shows the two upper panels
  (`Config File`, `Execution Status`) and the lower config panel, and the hint
  bar `↑↓ Navigate  Tab Switch panel  c Toggle config view  r Run  q Quit`;
  `Enter` opened the picker popup, `Enter` loaded the config, `r` started the
  run (status `Running: 1`), `q` quit. (b) The committed model/smoke tests that
  cover panels, focus, Tab, `r`, `s`, `c`, `q`/confirmation, scrolling and the
  end-to-end smoke drive. (c) The event-bridge tests proving file-lines are a
  superset-free mirror of the panel records on success, and that a successful
  run adds no writer error event.
- What came back: the pty capture shows the three panels and the key hints;
  the run started and produced `logs/ytss.log`; `go test ./tui -run
  'TestModel|TestEndToEndSmoke|TestEvents|TestFilePicker|TestConfigPanel|TestClipLines'`
  → `70` PASS, `0` FAIL. `PROBE_RUN1 file-lines-missing-from-recent-events=0`.
- Evidence: `raw-tui-drive.log`; `raw-model-tests.log`.
- Named tests: `TestRunLogKeepsEventBridgeContent`,
  `TestRunLogNoWriterErrorOnSuccess`, the `TestModel*` / `TestEndToEndSmoke`
  set (all PASS).

## UI flows from the spec

### 7. empty (before any run) — looks in the output folder
- How I tried it: real run probe, `os.Stat(<output_dir>/logs)` before the run.
- What came back: `PROBE_BEFORE_RUN logs-dir-exists=false` — nothing is written
  until a run starts.
- Evidence: `raw-probe-runs.log`.

### 8. in progress — starts a batch run
- How I tried it: the probe polled `logs/ytss.log` every 150 ms while the real
  run was in flight; the TUI pty session watched the panel while `r` ran.
- What came back: `mid-run-file-had-content=true`, first mid-run line
  `msg="built video index"`; pty capture shows `Running: 1` and `[s] Stop`.
- Evidence: `raw-probe-runs.log`, `raw-tui-drive.log`.

### 9. success — opens logs/ytss.log in any editor
- How I tried it: `file`, `wc`, ANSI/NUL scan and `pbcopy`/`pbpaste` round-trip
  (see acceptance 2).
- What came back: `Unicode text, UTF-8 text`; clipboard copy byte-identical.
- Evidence: `raw-probe-runs.log`; `run-a/ytss-output/logs/ytss.log`.

### 10. repeat — runs a second batch
- How I tried it: second real run over the same output dir (see acceptance 3).
- What came back: `first-run-content-preserved=yes run1-lines=12 run2-lines=18`.
- Evidence: `raw-probe-runs.log`.

### 11. error (file unwritable) — runs with the output folder read-only
- How I tried it: the named test makes the output dir read-only (real
  `chmod`, real run through `runPipelineWithConfig`) and asserts exactly one
  `ERROR: log file` event naming the path and `permission denied`, no file
  created, and that a later `batch complete:` event follows (the run finishes).
- What came back: `TestRunLogOpenFailureShowsOneErrorEvent`,
  `TestRunLogOpenFailureRunContinues` → PASS.
- Evidence: `raw-runlog-tests.log`; source `tui/runlog_test.go:407-510`.

### 12. error (write fails mid-run) — disk fills during the run
- How I tried it: the named tests inject a full-disk write failure through the
  writer's test seam (real writer, real run; `tui/runlog_test.go:481-560`).
- What came back: exactly one `ERROR: log file` event on the first failure,
  further failures silent → `TestRunLogWriteFailureShowsOneErrorEvent`,
  `TestRunLogWriteFailureReportsOnlyOnce` PASS.
- Evidence: `raw-runlog-tests.log`.

### 13. error (red log line) — a stage fails during the run
- How I tried it: a real run of the real video with a garbage cookie file, so
  the real `yt-dlp` fails both the subtitle and audio stages; the video's
  processing stage fails and logs through the program's logger.
- What came back: the file ends with a red-line record:
  `time=… level=ERROR msg="video processing failed" video_id=dQw4w9WgXcQ title="Rick Astley - Never Gonna Give You Up (Official Video) (4K Remaster)" error="both subtitle and transcription failed: subtitle=all subtitle download steps failed: … transcription=downloading audio: yt-dlp failed: exit status 1"`,
  then `streaming batch complete success=0 skipped=0 partial=0 failed=1`
  (`level-ERROR-lines=1`). The line is plain text and copyable like the rest.
- Evidence: `raw-probe-redline.log`; `run-c/ytss-output/logs/ytss.log`.
- Observation, not a flow failure: a batch-level failure (e.g. the playlist
  list fetch itself fails) returns an error that the TUI puts in the events
  panel only — it is not a logger record, so it does not enter the file. See
  `raw-probe-redline.log` first attempt (invalid video URL: file received 4
  INFO lines, no ERROR line) and `tui/model.go` `runOneBatch` /
  `runPipelineWithConfig`'s `fail()` which call `state.AddRecentEvent`, not
  `slog`.

### 14. unchanged — watches the panels and presses keys during a run
- How I tried it: the real pty session above plus the committed model/event
  tests (see acceptance 6).
- What came back: panels render, key hints unchanged, `r`/`s`/`q`/Tab behave
  as documented; no writer error event on a successful run.
- Evidence: `raw-tui-drive.log`, `raw-model-tests.log`.

## Test commands run

```
go test ./tui -run 'TestRunLogFile|TestRunLogOpenFailure|TestRunLogWriteFailure|TestRunLogKeepsEventBridgeContent|TestRunLogNoWriterErrorOnSuccess|TestRunLogRestoreDoesNotClobber|TestRunLogWatch' -v -count=1
  -> 19 tests PASS   (raw-runlog-tests.log)

go test ./tui -run 'TestModel|TestEndToEndSmoke|TestEvents|TestFilePicker|TestConfigPanel|TestClipLines' -v -count=1
  -> 70 tests PASS, 0 FAIL   (raw-model-tests.log)
```

The full package suite (`go test ./...`) was left to closing review, as the
acceptance-test procedure requires.

## Note on the clean-copy setup

`embedded/bin` is gitignored and absent from a fresh worktree; the worktree's
`go build ./...` only succeeds after copying the gitignored binaries from the
main checkout into `embedded/bin` (the repo's `make build` would download or
build them). Nothing from `embedded/bin` was committed.
