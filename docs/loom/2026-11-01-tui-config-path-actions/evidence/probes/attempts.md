# Adversarial attempts — 2026-11-01-tui-config-path-actions

Change under attack: `tui/actions.go`, `tui/model.go`, `tui/model_test.go` at
`b67d0b5`. Artifact types: `code` (recipe `adversarial-code.md`). The repo
declares no mutation or fuzz tooling, so the attacks below are executable
abuse and boundary cases run through the real `Update`/`handleKey` path with
fake `pbcopy`/`open` stubs on `PATH` (shell builtins only, so a narrowed
`PATH` does not break the stubs), plus one direct check of real macOS `open`.

Stub shape used by the trial cases: a temporary `pbcopy` that appends its
stdin to a marker and an `open` that appends each argv element to the same
marker; `PATH` narrowed to the stub directory; the model is driven with
`tea.KeyPressMsg`/`tea.MouseClickMsg` and `AppState.Snapshot().RecentEvents`.

| # | Attack point | What was done | Result |
|---|---|---|---|
| 1 | No config selected (`tui/model.go:704-716`) | Fake `pbcopy`/`open` on `PATH`, empty `ConfigPath`, press `y` then `o` | HELD — no stub invoked; two "No config selected — press Enter to choose one" events |
| 2 | Dependency fails (`tui/actions.go:27-29`) | Stub `pbcopy` exits 3 with `boom` on stderr, press `y` | HELD — event `Copy failed: pbcopy failed: exit status 3 — boom — try again`; no panic |
| 3 | Keys on other panels (`tui/model.go:705,718`) | Tab to `PanelStatus`, press `y`/`o`; then Tab back and press again | HELD — inert while status focused (no stub call, no event); acts again after refocus |
| 3b | Picker popup open (`tui/model.go:626-644`) | Enter opens popup, then press `y`/`o` | HELD — popup owns keys; no stub call |
| 4 | Rapid repeated presses | 25 `y` presses in a row, then focus flips and `o` | HELD — 25 synchronous `pbcopy` runs, exactly 1 `open` run, no queueing or interleaving |
| 5 | Non-macOS / command missing (`tui/actions.go:14-22`) | `PATH` to an empty dir (no `pbcopy`, no `open`), press `o` and `y` | HELD — `Reveal failed: …` / `Copy failed: …` events, no panic |
| 6 | Injection / hostile path | Config path `-p; touch pwned; $(id) \`id\` \n あ ; rm -rf ~`, press `y` then `o` | HELD — exact bytes reach `pbcopy` stdin; no shell is involved (`exec.Command` + argv, no `sh -c`); `pwned` not created |
| 7 | Reveal argv (REQ-2) | Stub `open` records argv; config selected; press `o` | **BROKE** — `open` argv was `["-R"]`: the config path is fed to the stub's *stdin*, never as an argument |
| 7b | REQ-2 against real macOS | `echo /tmp/x | open -R` in a shell | **BROKE** — real `open` prints usage and exits 1: `-R` without a file argument reveals nothing |
| 8 | REQ-5 hint row | Strip ANSI from `View()`; click renders `[y] Copy` and `[o] Reveal` | HELD — both render when the card is focused; `[o] Reveal` wraps across two hint lines; clicking `[y]` copies and clicking `Reveal` reveals |
| 9 | REQ-6 existing keys | Press `y`, `o`, then Enter and Esc | HELD — Enter still opens the picker, Esc still closes it, focus stays on the card |

Attempts that failed to break the change (1, 2, 3, 3b, 4, 5, 6, 8, 9) are not
committed as programs; only attack 7 earned one:

- `reveal_missing_path_arg_test.go` — `go test -json docs/loom/2026-11-01-tui-config-path-actions/evidence/probes/reveal_missing_path_arg_test.go`
  is RED at `b67d0b5` (`open never received the config path as an argument;
  argv=["-R"]`) and turns GREEN once the reveal command receives the path.

Notes (not attacks, routed elsewhere): the plan's W1-01 Test line names
`copySetsClipboard`, `revealRunsOpenR`, `noConfigShowsHint`,
`cardHintsShowCopyReveal`, `newKeysInertOutsideCard` and its boundary pairs,
and its Risk 3 says the seam "is overridden in the suite" — none of those
tests exist in `tui/` and `runCommand` has no swap seam. The change's own
suite is green (`go test ./tui/...` → ok).
