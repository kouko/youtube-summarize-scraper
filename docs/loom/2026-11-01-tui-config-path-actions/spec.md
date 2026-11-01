# TUI config card: copy path & reveal folder in Finder — spec
intent: 2026-11-01-tui-config-path-actions@9882e8beee6232454fd02c4035695c5b3aaf16cc
confirmed-behavior: 2026-11-01 @b2d0220
pre-build-review: not-required — no security or privacy surface (a config path only, no secrets), no public contract, no irreversible data beyond the clipboard overwrite the user already requested in Acceptance #1, and the requirements are unambiguous

## Requirements
REQ-1 — Copy config path to clipboard
  WHEN the config card has focus and a config file is selected, the system shall copy that file's full path to the system clipboard and report the success in Recent Events → Acceptance #1
REQ-2 — Reveal config folder in Finder
  WHEN the config card has focus and a config file is selected, the system shall invoke the macOS Finder reveal command targeting that file's containing folder and report the success in Recent Events → Acceptance #2
REQ-3 — No config selected
  IF no config file is selected, THEN pressing either new key shall not touch the clipboard or Finder and shall report a "no config selected" message in Recent Events → Acceptance #3
REQ-4 — Result reporting
  WHEN the user presses either new key, the system shall report the outcome in Recent Events: one line naming the action, and on failure the reason → Acceptance #4
REQ-5 — Hint row
  WHEN the config card has focus, the system shall show both new keys in the card's hint row → Acceptance #5
REQ-6 — Existing keys preserved
  The system shall keep the existing key behaviours (Enter, c, r, s, q, Tab, arrow keys) unchanged → Acceptance #6

## Design decision
- Keys `y` (copy path) and `o` (reveal folder in Finder), both acted on only while the config card has focus. agent-decided — the path is displayed on the card, and both keys are unclaimed in the current key map (tui/model.go:653-721 uses enter/esc/q/ctrl+c/r/s/c/tab/shift+tab/up/down/k/j/pgup/pgdown).
- Clipboard via macOS `pbcopy` fed the path on stdin. agent-decided — the repo is macOS-only (PRINCIPLES fixed choices: Go CLI via Homebrew) and the reveal feature is macOS-only anyway; `pbcopy` has no terminal-support dependency. OSC 52 rejected, see Alternatives.
- Reveal via `open -R <config-path>`: opens the containing folder in Finder with the file selected. agent-decided — satisfies "open the folder the config lives in" while also locating the file; plain `open <dir>` rejected, see Alternatives.
- External commands run through one small seam (`runCommand`) so tests stub the OS calls; no build tags — a missing command surfaces through the normal failure line. agent-decided — keeps tests deterministic on any host and avoids platform forks.
- Success and failure both append a line through the existing `AppState.AddRecentEvent` path (tui/state.go:178); failure lines name the way out (choose a config first, or retry). agent-decided — PRINCIPLES "never fail silently".
- Event text is English, consistent with the rest of the events stream (pipeline/slog output). agent-decided.
- Hint row of the config card gains the two new segments next to the existing `[Enter] Select Config`; hint layout already word-wraps. agent-decided.

## Alternatives considered
- OSC 52 escape sequence for the clipboard — rejected: terminal/SSH support varies; repo is macOS-only and `pbcopy` is simpler and stub-testable.
- `open <dir>` (open the folder without selecting the file) — rejected: `open -R` also locates the file in the folder, strictly more informative for the same command shape.
- Two open keys (`o` reveal, `e` edit with default app) — rejected: the user explicitly chose only the folder-open variant.
- Keys active on other panels — rejected: the path is only shown on the config card; limiting to card focus avoids surprising clipboard writes.

## Current state evidence
- Forward: the config card renders the selected path via renderConfigCard (tui/model.go:1454-1470); the selected path lives in the state snapshot (tui/state.go:19, ConfigPath set at :83).
- Reverse: keyboard dispatch flows through handleKey (tui/model.go:653-721); the card's hint row is built by hintSegmentsFor (tui/model.go:1120-1142); event lines append via AppState.AddRecentEvent (tui/state.go:178-185).
- Error: no copy/reveal path exists today; nothing handles a missing selection or a failed external command for these actions.
- Data: ConfigPath is a string; RecentEvents is a capped slice of strings; AddRecentEvent is the single append point.
- Boundary: the change stops at two new key cases in handleKey, the card hint row, and one command-run seam; the pipeline, CLI, and all other panels are untouched.

## UI flows
Config File card, focused:

| Case | What the user does | What they see |
|---|---|---|
| Copy, config selected | presses `y` | the full config path is on the system clipboard; Recent Events gains "Config path copied to clipboard: <path>" |
| Reveal, config selected | presses `o` | Finder opens the folder containing the config and selects the file; Recent Events gains "Revealed in Finder: <path>" |
| No config selected | presses `y` or `o` | nothing is copied and no Finder window opens; Recent Events gains "No config selected — press Enter to choose one" |
| Copy/reveal fails | presses `y` or `o` | Recent Events gains a line naming the action and the reason, e.g. "Copy failed: <reason> — try again" / "Reveal failed: <reason> — check the path exists" |

- Empty — the card already shows "(no config selected)"; pressing either new key then reports the no-config line above instead of acting.
- In progress — both actions are instant (one short-lived OS command); no spinner or waiting state.
- Error — one line per failure way, each naming the way out: no config → press Enter to choose one; command failed → retry the key, or check the config file still exists.
- Success — the ordinary populated case above; focus stays on the card, and every existing key (Enter, c, r, s, q, Tab, arrows) keeps its current behaviour.

Paths: pressing either new key keeps focus on the card — the user can immediately continue with Enter/r/Tab as before; no surface is entered or left by these keys.
