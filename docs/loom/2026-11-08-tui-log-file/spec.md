# TUI run log file — spec
intent: 2026-11-08-tui-log-file@ff2991515dc93c9c7990fb2fd44afb509a07da44
confirmed-behavior: 2026-11-08 @b757c0f # decision point ②: log file created after run, selectable + copyable
pre-build-review: required — the change creates a persistent on-disk artifact that could carry credentials; PRINCIPLES non-negotiable 2 (never leak secrets) applies, and the file's write path is new I/O surface.

## Requirements

REQ-1 — Log file after a run
  WHEN a batch or single-video run in the TUI ends, the system shall have a log file at `<output_dir>/logs/ytss.log` containing the processing messages produced during that run → Acceptance #1

REQ-2 — Plain-text, copyable content
  The log file shall be plain UTF-8 text whose whole content can be opened in a general editor and selected and copied → Acceptance #2

REQ-3 — Append across runs
  WHEN a second run starts, the system shall append to the existing log file, preserving every line from earlier runs and never clearing or overwriting it → Acceptance #3

REQ-4 — Own log records only
  The log file shall contain only records produced by the program's own logger; raw progress output from external fetch and transcription tools shall not appear in it → Acceptance #4

REQ-5 — No credentials in the file
  The log file shall never contain cookie values, keychain contents, or API key values → Acceptance #5

REQ-6 — TUI unchanged
  The TUI panels, recent-events content, and key bindings shall behave as they do before this change, except for the log-writer error events defined in the Design decision → Acceptance #6

## Design decision

- The log path is `<output_dir>/logs/ytss.log`, resolved from the config actually used for the run, with the `logs` directory created on first use. `agent-decided` — the intent's Constraints pin "follows the output folder"; no new config field, per the complexity check at intent confirmation.
- The logger writes each record twice: once to the existing event bridge (unchanged TUI behaviour) and once to the file, opened append-only (`O_CREATE|O_APPEND|O_WRONLY`) at run start. `agent-decided` — smallest change that keeps REQ-6; reopening per run is what makes REQ-3 hold without rotation.
- No per-line `fsync` and no buffered writer flush cycle; the OS page cache handles durability. `agent-decided` — the intent's Constraints ask for low I/O; a crash loses at most the tail of the current run.
- **File writer target is set in `runPipelineWithConfig` (`tui/model.go:1765-1771`) where the run's config is loaded, not in `cmd/tui.go`; the CLI stays unchanged because the writer is only created once per run with that path.** `agent-decided` — keeping the tee outside the pipeline layer is sound; the exact seam lives where config is available.
- The file writer detects write-level errors (Go's `slog` discards `Handler.Handle` errors) and records **one** ERROR event in the Recent Events panel naming the path and cause; processing continues unchanged. `agent-decided` — per PRINCIPLES non-negotiable 1, a log file must not silently swallow write failures; the error line makes the failure visible to the user.
- If the log file cannot be opened (permissions, read-only volume), the run continues and the Recent Events panel shows one error line naming the path and the cause. `agent-decided` — PRINCIPLES non-negotiable 1 (never fail silently) and 3 (unattended runs keep working); a log file must not stop processing.
- Log content is unchanged; the tee adds no request/response bodies and no credentials. `agent-decided` — PRINCIPLES non-negotiable 2 already pins the content rule, so this is compliance, not a fork.

## Alternatives considered

- Configurable log path via `config.yaml` (`log_file`): rejected — the complexity check at intent confirmation showed retrofit is a few lines, so a new field now buys nothing (YAGNI).
- Per-run timestamped files: rejected — Acceptance #3 asks for retained history in one file; several files make "where did the last error go" harder for the user.
- Writing only warning/error lines: rejected — filtering risks dropping the exact line the user wants to copy, and the full own-log stream is already low-volume.
- Teeing inside the pipeline layer: rejected — the CLI `run` command already logs copyable stderr; a pipeline tee would duplicate there and change CLI behaviour outside this intent.

## Current state evidence

- Forward: `cmd/tui.go:28` sets the default logger to a text handler over the event bridge, so every pipeline log record starts there today.
- Reverse: `tui/bridge.go:57` (`NewEventBridge`) and `tui/bridge.go:105` (`Write`) consume those records into `AppState` recent events; the bridge is created in `cmd/tui.go:27` and owned by the model (`tui/model.go:266`).
- Error: nothing is persisted today — `cmd/tui.go:33-37` discards subprocess stderr and the standard logger, so a run's messages vanish when the process exits.
- Data: records are slog text lines shaped `time=<ts> level=<lvl> msg="..." key=value`.
- Boundary: this change stops at producing the file; it adds no rotation, no size cap, no config path, no TUI copy key, and does not change which lines are logged.

## UI flows

| case | what the user does | what they see |
|---|---|---|
| empty (before any run) | looks in the output folder | no `logs/` directory yet; nothing is written until a run starts |
| in progress | starts a batch run | the event panel fills as before, and `logs/ytss.log` grows with the same lines |
| success | opens `logs/ytss.log` in any editor | plain text for the whole run; select-all and copy work |
| repeat | runs a second batch | the file keeps both runs' lines; earlier content is intact |
| error (file unwritable) | runs with the output folder read-only | one error line in the event panel names the path and cause; processing still finishes |
| error (write fails mid-run) | disk fills during the run | one ERROR event in the event panel names the path and cause; processing continues |
| error (red log line) | a stage fails during the run | the failing stage's message appears in the file with its error text, copyable |
| unchanged | watches the panels and presses keys during a run | panels, recent events (except log-writer error events), and bindings behave as before |

Paths: after the first run the file exists for every later run; deleting it between runs is safe (the next run recreates it). **During a run**, especially in an hours‑long unattended watch loop (`tui/model.go:1804‑1830`), the append fd keeps writing to the same inode; if the file is deleted mid‑batch, records written after the deletion are lost to a dead inode and will not appear in any later run — the next run recreates the file and starts a fresh tail. This risk is explicitly disclosed because it strikes PRINCIPLES non-negotiable 1 (records can be lost silently) and non-negotiable 3 (unattended runs must keep working without surprise data loss). **Within a watch loop**, config reload can change `output_dir`; the writer was created once per run at run start in `runPipelineWithConfig` before the loop, so the path does not change mid‑batch. The spec's former phrase about switching configs is therefore not applicable inside a watch loop — a config change takes effect at the next TUI run start.

The file writer's permission mode is `0644` (umask‑dependent) with a stated reason: the file contains only processed‑run messages, no secrets, and restricting to owner‑read would impede debugging by other tools the user may run on the same output folder.
