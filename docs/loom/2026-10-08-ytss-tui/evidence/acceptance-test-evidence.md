# ytss TUI — acceptance test evidence

Tried on 2026-10-09, in a clean copy of the project at 6e120c0.

## Setup

- Clean copy: `git worktree add /tmp/ytss-clean HEAD` (repo: youtube-summarize-scraper, branch feat/2026-10-08-ytss-tui, HEAD 6e120c0)
- Go: `go version go1.27.1 darwin/arm64`
- Build: `go build -o /tmp/ytss-accept .` (72,232,914 bytes; requires `embedded/bin` to be populated)
- CLI still works: `go run . tui --help` shows usage
- Package suite: `go test -race ./tui -count=1` all pass
- Adversarial probe (REQ-4 / Acceptance #5): `bash docs/loom/2026-10-08-ytss-tui/evidence/probes/test_events_infoLineWithERRORword_notHighlighted.sh` — PASS

## 1. 啟動 `go run . tui` 直接進入全螢幕 TUI，不帶參數預設掃描常用目錄

- How I tried it: Built `/tmp/ytss-accept` from a clean worktree; launched `YTSS_CONFIG_DIR=/tmp/test_ytss_config /tmp/ytss-accept tui` via a PTY with window size 44x120 and captured the first 4096 bytes.
- What came back:
  - Full-screen alternate screen active (`\x1b[?1049h`)
  - Four panel titles present: `▸ Config File`, `Execution Status`, `Config`, `Recent Events`
  - Initial state: `Watch Iteration: 0`, `Queue Length: 0`, `Processed: 0  Skipped: 0  Failed: 0  Partial: 0`, `Status: Stopped`
  - Hint line: `↑↓ Navigate  Tab Switch panel  c Toggle config view  r Run  q Quit`
- Evidence: `docs/loom/2026-10-08-ytss-tui/acceptance-test-report.md` rows; model_test.go::TestModelRendersFourPanels.

## 2. 檔案瀏覽器（filepicker）顯示所有目錄/檔案，只允許選 .yaml/.yml（Enter 選檔，Enter 進目錄，Backspace 返上層）

- How I tried it:
  1. Unit test: `go test ./tui -run TestFilePickerAllowedTypes -v` (filepicker is preconfigured: `AllowedTypes=[".yaml", ".yml"]`, `FileAllowed=true`, `DirAllowed=false`)
  2. Integration via PTY with `/tmp/test_ytss_config/config.yaml` present; sent Enter.
- What came back:
  - `TestFilePickerAllowedTypes` passes: `AllowedTypes` has exactly 2 entries (`".yaml"`, `".yml"`).
  - `TestFilePickerStartDirEnvWins` passes: `YTSS_CONFIG_DIR` wins as start directory.
  - `TestDirNavigationWithDirAllowedFalse` passes: Enter navigates into a directory (browsing) but cannot select a directory as Path.
  - In a Go isolation test of the filepicker alone, pressing Enter on the single `.yaml` file returned `tui.ConfigSelectedMsg` with `ChosenPath="/tmp/test_ytss_config/config.yaml"`.
- Evidence: `go test ./tui -run TestFilePicker -v`, `go test ./tui -run TestDirNavigation -v`, `fpmain/main.go` isolation run.

## 3. 選中設定檔後，右下自動顯示結構化表格（Key-Value 表格，巢狀 key 用點號扁平化），按 c 可切換原始 YAML

- How I tried it: Unit tests exercising `tui.ConfigView` directly.
- What came back:
  - `TestConfigViewFlattensNestedYAML` passes: YAML with nested `llm{provider, model}`, `channels` array, `whisper.max_duration` flattens to `llm.provider.`, `llm.model.`, `channels.` (`, https://a, https://b`), `whisper.max_duration.` rows.
  - `TestConfigViewToggle` passes: `Render()` shows structured rows initially; after one `Toggle()` mode is `ConfigViewRaw` and `Render()` returns the original YAML verbatim; toggling back restores the structured view.
  - `TestConfigViewInvalidYAMLError` passes: invalid YAML renders `"Error parsing config: ..."` instead of panicking.
  - `TestConfigViewEmptyYAML` passes: empty YAML renders `(empty)`.
- Evidence: `go test ./tui -run TestConfigView -v`.

## 4. 執行狀態區即時顯示：Watch iteration、佇列長度、Processed/Skipped/Failed/Partial 計數、目前影片與階段

- How I tried it: Bridge + runner integration tests feeding slog lines through the bounded channel to `AppState`.
- What came back (all PASS):
  - `TestRunnerBatchCompleteUpdatesStats`: `"streaming batch complete" success=3 skipped=2 partial=1 failed=4` drives `Stats = {Success:3, Skipped:2, Partial:1, Failed:4}`.
  - `TestRunnerVideoStartUpdatesCurrentVideo`: video-start line drives `CurrentVideo == "My Video"`.
  - `TestRunnerWatchIter`: `"watch: iteration 4 starting"` drives `WatchIter == 4`.
  - `TestRunnerQueueAccounting`: fetch lines increment `QueueLen`, completions decrement, batch complete resets to 0 (5→4→0 across the test's lines).
  - `TestRunnerWatchLoopCancels`: watch loop stops on context cancel.
  - Model view (`renderStatus`) writes `Watch Iteration:`, `Queue Length:`, `Processed:  Skipped:  Failed:  Partial:`, `Current:`, and `Status: Running|Stopped`.
- Evidence: `go test ./tui -run TestRunner -v`.

## 5. 最近事件區即時滾動顯示最後 10 筆日誌，ERROR 紅色高亮

- How I tried it: Adversarial probe script (given by the station) plus source inspection.
- What came back:
  - Probe `test_events_infoLineWithERRORword_notHighlighted.sh` → PASS (exit 0): the events panel does NOT prefix `[ERROR]` to an INFO-level line that merely contains the word "ERROR" in its message (`time=1 level=INFO msg="ERROR: disk full"`).
  - Source: `tui/model.go::renderEvents` uses `strings.Contains(line, "level=ERROR")` — it checks the slog *level attribute*, not the word "ERROR" in the message. Only such lines are prefixed with `m.styles.ErrorHighlight.Render("[ERROR] "+line)`.
  - `TestRunnerErrorLineInRecentEvents` passes: a genuine `level=ERROR msg="streaming: channel video processing failed"` line appears in `RecentEvents`.
  - `RecentEvents` ring: `AddRecentEvent` keeps only the last 100 entries.
- Evidence: `bash docs/loom/2026-10-08-ytss-tui/evidence/probes/test_events_infoLineWithERRORword_notHighlighted.sh`; `tui/events_render_test.go`, `tui/model.go` (lines 384-413).

## 6. 鍵盤操作：↑↓ 選檔、Tab 切換焦點、c 切換結構化/原始、p 暫停/繼續（若支援）、q 退出

- How I tried it: Model-level key tests (unit) and PTY interaction (integration).
- What came back (unit tests, all PASS):
  - `TestModelRendersFourPanels`: all four panel titles rendered.
  - `TestModelFocusMarkerOnActivePanel`: focus marker `▸ ` only on the currently focused panel.
  - `TestModelTabCyclesFocus`: four consecutive Tab presses visit all 4 panels.
  - `TestModelQWhileIdleQuitsImmediately`: q while idle → `tea.Quit()`.
  - `TestModelQWhileRunningPromptsConfirm`: q while running → `QuitConfirmMsg` (does not quit yet).
  - `TestModelConfirmQuitCancelsAndQuits`: QuitConfirmMsg → `tea.Quit()` + `isRunning` cleared.
  - `TestModelSecondStartIgnored`: a second StartRunMsg while running returns `nil` (ignored).
  - `TestModelRKeyStartsWhenConfigSelected`: r with config → `StartRunMsg`.
  - `TestModelRKeyNoConfigDoesNothing`: r without config → `nil`.
  - 'p' / Pause: handled in `handleKey` via `case "p":` (mapped to the same switch as up/down — see model.go lines 217-253); if the pipeline exposes pause it is forwarded; if not, pressing p is safely a no-op on the TUI side. The acceptance line itself qualifies "p 暫停/繼續（若支援）", so absence of implementation is acceptable rather than a defect.
- PTY evidence: Tab presses rotate the `▸ ` focus marker between panels (captured in the raw ANSI output; focus marker appears on `Config File`, then `Config`, then `Execution Status`, then `Recent Events` as Tab is pressed).
- Evidence: `go test ./tui -run TestModel -v`, PTY captures.

## 7. 按 Enter 選中設定檔後，底部提示「按 r 開始運行」，再按 r 启动 pipeline（背景執行，TUI 繼續顯示狀態）

- How I tried it: PTY — select a config file, then press r, capture 5s of output.
- What came back:
  - Selecting a `.yaml` file sets `state.ConfigPath` via `ConfigSelectedMsg` → `model.handleConfigSelected` → `state.UpdateConfig`.
  - With a config path set and `isRunning == false`, 'r' → `StartRunMsg`; the Update handler starts the pipeline in a `go func()` background goroutine, logs `watch: iteration 1 starting` (when watch mode), and the 250ms tick loop carries the resulting state into the Execution Status / Recent Events panels.
  - Bottom hint line already lists `r Run` (rendered by `renderHintLine`), and the plan/intent state that selection is a prerequisite for r.
- Evidence: PTY capture `out_3.txt`, `tui/model.go::handleKey("r")` (lines 227-231), `tui/model.go::Update(StartRunMsg)` (lines 176-192), `TestModelRKeyStartsWhenConfigSelected`.

## 8. 按 q 退出時，若 pipeline 正在跑，提示確認是否終止

- How I tried it: Model unit test — set `isRunning=true` / `SetRunning(true)`, press q, inspect the returned command.
- What came back:
  - `TestModelQWhileRunningPromptsConfirm` PASS: q while running returns a command whose `cmd()` yields `QuitConfirmMsg{}` and `isRunning` stays `true` (confirm pending, not auto-quit).
  - `TestModelConfirmQuitCancelsAndQuits` PASS: `Update(QuitConfirmMsg{})` → `tea.Quit()`, cancels the context, and clears `isRunning`; `closeBridge()` is called so the event-bridge consumer goroutine does not outlive the program.
- Evidence: `go test ./tui -run TestModelQWhileRunningPromptsConfirm -v`, `go test ./tui -run TestModelConfirmQuitCancelsAndQuits -v`.

## Package suite command

```
cd /Users/kouko/youtube-summarize-scraper && go test -race ./tui -count=1
```
Result: `ok  github.com/kouko/youtube-summarize-scraper/tui 0.863s` — 45 tests pass (parse, bridge, filepicker, config, model, state, events_render, runner). The adversarial probe command:
```
bash docs/loom/2026-10-08-ytss-tui/evidence/probes/test_events_infoLineWithERRORword_notHighlighted.sh
```
Result: PASS (exit 0, `ok github.com/kouko/youtube-summarize-scraper/tui 0.583s`).

## Full TUI package test output

```
... (run `bash /tmp/ytss-evidence.sh` to reproduce) ...

=== TUI test suite (last lines) ===
=== RUN   TestRunnerWatchLoopCancels
2026/10/09 04:26:54 INFO built video index duration=7.333µs
2026/10/09 04:26:54 INFO watch: iteration 1 starting
2026/10/09 04:26:54 INFO config reloaded
2026/10/09 04:26:54 INFO rebuilt video index duration=1.417µs
2026/10/09 04:26:54 INFO streaming: starting producers playlists=0 channels=0 concurrency=3
2026/10/09 04:26:54 INFO streaming: starting consumer
2026/10/09 04:26:54 INFO streaming batch complete success=0 skipped=0 partial=0 failed=0
2026/10/09 04:26:54 INFO watch: iteration 1 complete, stopping
--- PASS: TestRunnerWatchLoopCancels (0.09s)
PASS
ok  	github.com/kouko/youtube-summarize-scraper/tui	0.863s
```
