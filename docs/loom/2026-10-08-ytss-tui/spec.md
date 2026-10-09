# ytss TUI — spec
intent: 2026-10-08-ytss-tui@37fe688
confirmed-behavior: 2026-10-09 (short-card layout + sectioned config tree; user-approved)
pre-build-review: required — the TUI adds a new interactive interface; ensure it does not block the pipeline.

## Requirements
REQ-1 — Show config file selector → Acceptance #1, #2
  WHEN the TUI starts, the top-left panel shows a short config card: the currently selected config path (or "(no config selected)" before any selection). Pressing Enter with the card focused opens a centered popup overlay with the file picker (bubbles v2, .yaml/.yml only, default start directory set to the user's common config directory, e.g. ~/kouko-obsidian-vault/_config/). Selecting a file or pressing Esc closes the popup. The card takes only the height its content needs (short card), top-aligned with the status panel.

REQ-2 — Display parsed configuration → Acceptance #3
  WHEN a configuration file is selected, the bottom-left panel displays the parsed configuration as a sectioned tree: top-level sections (llm, batch, playlists, channels, ...) each get a heading line; within a section, nested keys render as indented key-value lines; playlists/channels entries render as numbered items with their name/url/count and one indented sub-key line per nested map (cookie, copy_to, filter). Values never render as raw Go map[...] dumps. Pressing 'c' toggles between this tree and the raw YAML.

REQ-3 — Show real‑time execution status → Acceptance #4
  WHEN the pipeline is running (via the background 'r' command), the TUI shall update the execution status area with: Watch iteration, queue length, processed/skipped/failed/partial counts, and the currently processing video with its stage.

REQ-4 — Show recent log events → Acceptance #4
  WHEN the pipeline emits log lines, the TUI shall display the last N log lines (default 10) in a scrolling area, with ERROR‑level lines highlighted in red.

REQ-5 — Keyboard controls → Acceptance #5, #6, #7
  The TUI shall support the following keys:
    - Tab : switch focus between the four main panels (config card, config content, execution status, recent events)
    - Enter on the focused config card : open the file-picker popup
    - ↑/↓ : inside the popup, navigate the file listing; in the config/events panels, scroll
    - Esc inside the popup : close it without selecting
    - c   : toggle between the structured config tree and the raw YAML view
    - p   : pause/resume the pipeline (if supported)
    - q   : quit the TUI — first press arms a confirmation (hint line: "Press q again to quit; any other key cancels"); second press quits; any other key disarms; idle (not running) q quits immediately
    - r   : start the pipeline with the selected configuration file (runs in background, TUI continues to show status)

## Design decision
- Layout: 2x2 grid keeps equal top-row band heights as panel bounds, but the top-left config card renders at its content height (a short card, top-aligned via lipgloss.JoinHorizontal(Top)), so the space below the card stays free. The right panels and the bottom row are unchanged.
- File picker: bubbles v2 filepicker inside a centered popup overlay (lipgloss.Place over the frozen frame), AllowedTypes .yaml/.yml, start dir = env YTSS_CONFIG_DIR, else the home-dir candidate chain. While the popup is open it owns all keys; Esc closes without selecting.
- Configuration display: the selected file is parsed with yaml.Unmarshal into map[string]interface{}, then rendered as a sectioned tree (sections in key order, nested maps indented, list entries numbered). No dot-flattening; arrays of maps render as numbered items.
- Execution status and recent events are updated via an event‑bridge: a bounded channel (capacity 100) receives log lines from a slog.Tee wrapper; a background goroutine parses each line into an event enum and updates a shared AppState (protected by a mutex). The TUI copies the AppState every 250ms to render.
- The pipeline itself is unchanged: it continues to log via slog as before; the event bridge simply tees the output.
- The TUI is launched via a new subcommand: `go run . tui` (or `ytss tui` after build). It does not take a config flag; the config file is chosen via the file-picker popup.
- When the user presses 'r', the TUI calls pipeline.NewPipeline directly and runs the batch/watch loop in a goroutine — in‑process to share the same AppState via the event bridge.

## Acceptance
1. The TUI starts and shows the config card in the top-left (a short card with the current path or "(no config selected)"), the status panel top-right, config tree bottom-left, events bottom-right, and the hint line at the bottom.
2. Pressing Enter on the config card opens the file-picker popup; the user can navigate directories and select only .yaml/.yml files; Enter on a file closes the popup and loads it; Esc closes without selecting.
3. Upon selection, the sectioned config tree appears in the bottom-left panel (sections with headings, indented nested keys, numbered playlist/channel items), and pressing 'c' toggles to the raw YAML view.
4. Pressing 'r' starts the pipeline; the execution status area begins to update with Watch iteration, queue length, counts, and current video.
5. The recent events area shows the last log lines, with ERROR lines highlighted.
6. Pressing 'q' while the pipeline is running arms a confirmation (hint line changes); the second 'q' quits, any other key cancels; q while idle quits immediately.
7. All original CLI functionality (`go run . run -c <config>`) remains unchanged.

## Open questions
- Whether to support mouse scrolling in the file picker and log area (deferred; keyboard‑first is sufficient).

## Amend2 — live redraw, viewport scrolling, config editing (2026-10-09, user-approved scope: items 1, 2, 4)
REQ-6 — Immediate redraw on events → Acceptance #8
  WHEN the event bridge applies a parsed event, the model redraws on the next tick immediately (bridge pokes the program), not on the next 250ms tick only. The periodic tick remains as a fallback for non-event changes.

REQ-7 — Viewport scrolling → Acceptance #9
  The config and events panels scroll with a bubbles v2 viewport (native mouse wheel + keyboard), replacing the manual offset windows. ↑↓ scroll the focused panel; the wheel scrolls the panel under the pointer position is out of scope for amend2 (wheel applies to the focused panel).

REQ-8 — Edit selected config values → Acceptance #10
  On the config panel, Enter opens an inline editor (bubbles textinput) for the value under the cursor; accepting writes the change back to the YAML file on disk (preserving comments and key order via yaml.Node), reloads the in-memory config view, and shows the updated tree. Esc cancels. Only scalar values are editable (maps and lists open their first scalar child); file write failures surface as an error line in Recent Events.
