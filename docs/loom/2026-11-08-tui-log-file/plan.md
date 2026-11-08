# TUI run log file — plan
intent: 2026-11-08-tui-log-file@0b89a58

## Task DAG

### Wave 1 — Foundation (independent)

**W1-01 — Add log writer helper** acceptance: 1,2,3
- Files: `tui/logwriter.go` (new)
- Test: A1 positive: helper creates `<output_dir>/logs/ytss.log` holding every record passed to it; negative: read-only output dir returns an open error instead of panicking. A2 positive: written bytes are plain UTF-8 slog text lines with no ANSI escapes; boundary: file mode 0644 and directory mode 0755 are set on creation. A3 positive: a second helper opened on the same path appends and keeps earlier lines; boundary: flags are O_CREATE|O_APPEND|O_WRONLY with no O_TRUNC.
- Risk: new I/O surface; must not write credentials — the helper writes only the records it is handed, so the call site decides content; follows spec Design decision 27-29, 33.

**W1-02 — Wire log writer into TUI run start/end** acceptance: 1,2,3,4,5,6
- Files: `tui/model.go`, `cmd/tui.go`
- Test: A1 positive: after a batch run the file holds the same messages the Recent Events panel showed; negative: a run that emits no records leaves an existing file byte-identical. A2 positive: the run's file opens as text and copies whole in an editor; boundary: lines end with LF and keep the slog `key=value` shape. A3 positive: a second batch run appends and earlier run lines remain; boundary: restarting the TUI process still appends to the same path. A4 positive: the file holds only program slog lines and no yt-dlp or whisper stderr; negative: raw subprocess progress bytes never reach the writer. A5 positive: scanning a run's file finds no cookie, keychain, or API-key patterns; negative: configured cookie values never pass through the tee. A6 positive: panels, recent events, and key bindings behave as before; boundary: the only new event content is a log-writer error line.
- Risk: touches the core run loop; must not alter the CLI `run` command's stderr behaviour; writer created once per run where config loads; follows spec Design decision 28, 30-31.

### Wave 2 — Failure handling (depends on W1-02)

**W2-01 — Open failure: one error in Recent Events, run continues** acceptance: 6
- Files: `tui/logwriter.go`, `tui/model.go`
- Test: A6 positive: with a read-only output dir the panel shows one error line naming path and cause; negative: the run still finishes and later stages keep emitting events.
- Risk: must satisfy PRINCIPLES NN1 (never fail silently) and NN3 (unattended runs keep working); no retry loop; follows spec Design decision 32.

**W2-02 — Write failure: one ERROR event on first failure, suppress rest** acceptance: 6
- Files: `tui/logwriter.go`, `tui/model.go`
- Test: A6 positive: a mid-run write failure adds exactly one ERROR event naming path and cause; negative: further write failures in that run add no more events.
- Risk: Go's slog discards Handler.Handle errors, so the writer must surface failure itself and must not recurse through the tee; follows spec Design decision 31.

### Wave 3 — Watch loop (depends on W1-02, W2)

**W3-01 — Watch loop: writer created once per run before the loop** acceptance: 3
- Files: `tui/model.go`
- Test: A3 positive: two watch-loop iterations append to the same file when `output_dir` is unchanged; boundary: a reloaded `output_dir` applies to the next run, not mid-batch.
- Risk: `p.ReloadConfig` runs per iteration but the path must stay fixed for the batch; follows spec Design decision 28 and Paths.

## Questions asked

- ① — what — 你目前是用哪個終端機跑 TUI?這決定「按住修飾鍵拖曳」能不能直接解決。
- ① — what — 要不要把「log 可複製」做成程式功能?要的話我會先走 capture-intent 確認需求再改。
- ① — what — 既然 log 檔 I/O 很輕,你要怎麼做?
- ① — what — 執行 ytss 時把程式自己的執行訊息寫成純文字日誌檔,放在輸出資料夾的 logs 子目錄;你可以用任何編輯器開啟、選取、複製,程式關掉後也能查。TUI 畫面不變。對嗎?
- ② — behaviour — 你下 r 會看到輸出資料夾的 logs 子目錄出現 ytss.log,內容是這次執行的處理訊息;用編輯器打開可全選複製;再執行第二次檔案保留兩次紀錄;輸出資料夾無法寫入時事件面板出現錯誤說明但執行繼續。對嗎?

## Risks

- New I/O surface must not leak secrets: enforced by call site passing only processed slog records — PRINCIPLES NN2.
- Crash loses the tail of the current run because there is no fsync: disclosed in the spec, accepted in the intent Constraints.
- Mid-batch file deletion loses records silently to a dead inode: disclosed in the spec Paths, accepted as a NN1/NN3 trade-off; an fstat(nlink) recheck is a possible later upgrade.
- Watch-loop config reload changes `output_dir` only for the next run, never mid-batch: spec Paths constraint.
- Slog Handler.Handle error swallowing: the writer surfaces write failures directly and never recurses through the tee.
- Second vendor notice: none (second-vendor: suggest mode, non-blocking).
