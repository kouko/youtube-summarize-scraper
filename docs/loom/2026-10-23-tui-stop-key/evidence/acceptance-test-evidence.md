# TUI 加入停止鍵 — acceptance test evidence

Tried on 2026-10-23, in a clean git worktree at `020bf3f2919da02282243d09cdff3cdd993ce8b7`
(worktree of branch `feat/2026-10-23-tui-stop-key`, diff base `32142e3`).
Environment: macOS 26.7 (darwin/arm64), go 1.27.3, tmux 3.8, zsh.

Clean-checkout setup (from README / Makefile): `go build ./...` needs
`embedded/bin/darwin-arm64/{yt-dlp,ffmpeg,whisper-cli}` (gitignored build
deps). In the fresh worktree these are absent, so `make deps` would download
yt-dlp and compile ffmpeg + whisper from source. To keep the run offline and
fast, the same prebuilt platform deps were copied from the existing checkout
into the worktree (identical binaries; the change's code stayed untouched and
unbuilt). After provisioning, `go build ./...` and `go vet ./tui/` both
succeeded, and `go build -o ytss .` produced the runnable binary.

Live TUI runs used a sandbox config (`watch: true`, `watch_interval: 30`,
zero channels/playlists, local output dir, no network and no LLM work): the
first watch iteration finishes instantly and the run then sleeps, keeping the
TUI in Running state long enough to drive the keys. The binary was launched
inside tmux (`tmux new-session -d -s acc -x 170 -y 42 -e YTSS_CONFIG_DIR=<dir>
-c <dir> <dir>/ytss tui`), keys were sent with `tmux send-keys -t acc <key>`,
and frames captured with `tmux capture-pane -t acc -p` (ANSI stripped). The
TUI was exited cleanly at the end of each session (process exit code 0).

## 1. pipeline 正在執行時，按 `s` 鍵會停止 pipeline（狀態面板顯示從 Running 變為 Stopped）
- How I tried it: launched `ytss tui` in tmux, opened the file picker
  (Enter), selected the sandbox config (Enter), pressed `r`, waited for the
  pipeline to start, then pressed `s`.
- What came back: while running the status panel read `Status: Running` and
  the pipeline logged `watch: iteration 1 starting`. After pressing `s` the
  status panel read `Status: Stopped`, the pipeline logged
  `watch: iteration 1 complete, stopping`, and the TUI stayed open.
- Evidence:
  - `frame-02-running.txt` (running, before `s`):
    `│[s] Stop  [r] Run …` / `│Watch Iteration: 1` / `│Status: Running`
  - `frame-03-after-s.txt` (after `s`):
    `│[r] Run` / `│Watch Iteration: 1` / `│Status: Stopped`;
    events panel: `msg="watch: iteration 1 complete, stopping"`
  - Named test (tui package, targeted run): `TestModelSKeyWhileRunningEmitsStopRun` PASS,
    `TestModelStopRunMsgShowsStopped` PASS.
- Frames from the live runs: copied below are only the key lines; full frames
  were captured to a scratch directory outside the repo.

## 2. pipeline 未在執行時，按 `s` 鍵不會發生任何事（不會啟動、不會報錯）
- How I tried it: same live session, after the pipeline was stopped (frame-03),
  pressed `s` again and waited 3 s.
- What came back: status stayed `Status: Stopped`, watch iteration stayed 1,
  no new `watch: iteration … starting` event, no ERROR line in Recent Events.
- Evidence:
  - `frame-04-idle-s.txt`: `│[r] Run` / `│Watch Iteration: 1` /
    `│Status: Stopped`; events panel unchanged (last run event still
    `iteration 1 complete, stopping`).
  - Named test: `TestModelSKeyNotRunningDoesNothing` PASS.

## 3. 停止後，按 `r` 鍵可以重新啟動 pipeline
- How I tried it: same live session, right after the idle-`s` check, pressed `r`.
- What came back: status changed to `Status: Running`, hint row showed
  `[s] Stop  [r] Run`, and a fresh run logged `watch: iteration 1 starting`
  (the new run starts its own iteration counter at 1 — expected).
- Evidence:
  - `frame-05-restart-r.txt`: `│[s] Stop  [r] Run` / `│Status: Running`;
    events panel gained `msg="watch: iteration 1 starting"` at 08:45:14 (a
    second, later start after the 08:44:44 one).
  - Named tests: `TestModelRestartAfterStop` PASS,
    `TestModelRestartAfterStopNeedsConfigPath` PASS (boundary: no config
    selected → `r` does nothing).

## 4. 提示列在執行中顯示 `[s] Stop`
- How I tried it: read the status panel's hint row from the captured frames of
  the same live session, running vs stopped; also drove the hinted segment.
- What came back: while running the status hint row is
  `[s] Stop  [r] Run`; when stopped it is `[r] Run` only.
- Evidence:
  - `frame-02-running.txt` line 2: `│[s] Stop  [r] Run …`
  - `frame-03-after-s.txt` line 2 / `frame-04-idle-s.txt` line 2:
    `│[r] Run` (no `[s] Stop` segment when stopped).
  - Named tests: `TestModelStatusHintsShowStopWhileRunning` PASS,
    `TestModelStatusHintsHideStopWhenStopped` PASS.

## UI flow (spec): TUI 執行中按 `r` → shows `Status: Running`, hint shows `[r] Run`
- How I tried it: separate live session; selected the config, pressed `r`
  (run starts), then pressed `r` again while running.
- What came back: a second `r` while running starts nothing (exactly one
  `watch: iteration 1 starting` event in the panel), status stays Running,
  hint row shows `[s] Stop  [r] Run` (contains `[r] Run`).
- Evidence: `frame-10-r-while-running.txt`: `│[s] Stop  [r] Run` /
  `│Status: Running`, exactly one occurrence of `watch: iteration 1 starting`.
- Named test: `TestModelSecondStartIgnored` (pre-existing W3-01 test) covers
  the same boundary at the message level.

## UI flow (spec): TUI 執行中按 `s` → Running→Stopped, hint `[r] Run`→`[s] Stop`, can restart with `r`
- Covered exactly by acceptance lines 1–3 above (same live session, frames
  02→03→05).

## UI flow (spec): TUI 未執行時按 `s` → 無任何反應
- Covered exactly by acceptance line 2 above (frame-04).

## Constraints

### q / ctrl+c quit behavior unchanged (incl. two-press confirmation while running)
- How I tried it: same live sessions.
  - While running, pressed `q` once → bottom hint line became
    `Pipeline is running — Press q again to quit, any other key to cancel`
    (`frame-06-q-confirm.txt`), still `Status: Running`. Pressed `x` →
    confirmation disarmed, hint row restored to the normal key hints
    (`frame-07-disarm.txt`), still running. Then pressed `s` (Stopped), then a
    single `q` → the TUI exited immediately; process exit code 0.
  - Second live session: while running, pressed ctrl+c once → same
    confirmation line (`frame-09-ctrlc-confirm.txt`); pressed ctrl+c again →
    TUI exited, run cancelled, exit code 0.
- Evidence: frames `frame-06`, `frame-07`, `frame-09`; exit markers
  `tui-exit.txt` / `tui-exit2.txt` (`EXITED_RC=0`).
- Named tests: `TestModelQWhileRunningPromptsConfirm`,
  `TestModelOtherKeyCancelsQuitConfirm`, `TestModelQWhileIdleQuitsImmediately`,
  `TestModelConfirmQuitCancelsAndQuits` — all PASS.

### batch.random_order untouched
- `git diff 32142e3..HEAD --stat` touches only `tui/model.go`,
  `tui/model_test.go` and loom docs; no pipeline or config code changed. The
  `random_order` shuffle in `pipeline/pipeline.go` is outside the diff.

## Adversarial probe
- Command: `go test -json -v docs/loom/2026-10-23-tui-stop-key/evidence/probes/stop_key_stale_after_end_test.go`
  (run inside the clean worktree).
- Result: `Test_sstopkey_idle_after_selfending_run` PASS (0.01s) — after a run
  ends by itself, `s` does not dispatch a stop and `r` restarts cleanly (the
  stale `isRunning` bug the fix commit addresses).

## Targeted test run (criteria's own tests; full package suite left to finalize-review)
- Command: `go test -json -v ./tui/ -run 'TestModelSKeyWhileRunningEmitsStopRun|TestModelSKeyNotRunningDoesNothing|TestModelStopRunMsgShowsStopped|TestModelRestartAfterStop|TestModelRestartAfterStopNeedsConfigPath|TestModelStatusHintsShowStopWhileRunning|TestModelStatusHintsHideStopWhenStopped|TestModelRunFinishedMsgTokenMatch|TestModelQWhileRunningPromptsConfirm|TestModelOtherKeyCancelsQuitConfirm|TestModelQWhileIdleQuitsImmediately|TestModelConfirmQuitCancelsAndQuits'`
- Result: 12/12 PASS, package `ok` 0.825s. Also `TestModelSecondStartIgnored`
  covered above.

## Verification mode summary
- Acceptance lines 1–4: verified LIVE against the real TUI in a pseudo-terminal
  (tmux), driven with only what a user has (keyboard + config file).
- Tests cited are additional, not a substitute for the live runs.
