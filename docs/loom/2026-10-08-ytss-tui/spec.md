# ytss TUI — spec
intent: 2026-10-08-ytss-tui@37fe688
confirmed-behavior: 2026-10-08 @f230298
pre-build-review: required — the TUI adds a new interactive interface; ensure it does not block the pipeline.

## Requirements
REQ-1 — Show config file selector → Acceptance #1, #2
  WHEN the TUI starts, it shall display a file picker allowing the user to navigate the filesystem and select a .yaml or .yml configuration file, with the default start directory set to the user's common config directory (e.g., ~/kouko-obsidian-vault/_config/).

REQ-2 — Display parsed configuration → Acceptance #3
  WHEN a configuration file is selected, the TUI shall display the parsed configuration as a structured key‑value table (flattened nested keys, arrays joined by commas), and allow toggling between the structured view and the raw YAML via the 'c' key.

REQ-3 — Show real‑time execution status → Acceptance #4
  WHEN the pipeline is running (via the background 'r' command), the TUI shall update the execution status area with: Watch iteration, queue length, processed/skipped/failed/partial counts, and the currently processing video with its stage.

REQ-4 — Show recent log events → Acceptance #4
  WHEN the pipeline emits log lines, the TUI shall display the last N log lines (default 10) in a scrolling area, with ERROR‑level lines highlighted in red.

REQ-5 — Keyboard controls → Acceptance #5, #6, #7
  The TUI shall support the following keys:
    - ↑/↓ : navigate in the file picker or move focus between panels (when in config/content area)
    - Tab : switch focus between the four main panels (file picker, config content, execution status, recent events)
    - c   : toggle between structured config view and raw YAML view
    - p   : pause/resume the pipeline (if supported)
    - q   : quit the TUI (prompt for confirmation if the pipeline is running)
    - r   : start the pipeline with the selected configuration file (runs in background, TUI continues to show status)
    - /   : open a filter box for the file picker (optional)

## Design decision
- File picker uses bubbletea/filepicker with allowed file types .yaml/.yml, start directory set to the environment variable YTSS_CONFIG_DIR if set, otherwise to the user's home directory.
- Configuration display: the selected file is parsed with yaml.Unmarshal into map[string]interface{}, then flattened into key‑value rows (nested keys joined by dots, arrays joined by ', '), and rendered as a two‑column table using lipgloss tables.
- Execution status and recent events are updated via an event‑bridge: a bounded channel (capacity 100) receives log lines from a slog.Tee wrapper; a background goroutine parses each line into an event enum and updates a shared AppState (protected by a mutex). The TUI copies the AppState every 250ms to render.
- The pipeline itself is unchanged: it continues to log via slog as before; the event bridge simply tees the output.
- The TUI is launched via a new subcommand: `go run . tui` (or `ytss tui` after build). It does not take a config flag; the config file is chosen via the file picker.
- When the user presses 'r', the TUI spawns the pipeline as a subprocess (os/exec.Command) with the selected config file, or alternatively calls pipeline.NewPipeline directly and runs ProcessBatch in a goroutine — we choose the latter to keep everything in‑process and share the same AppState via the event bridge.

## Acceptance
1. The TUI starts and shows a file picker with the default directory pre‑filled to a reasonable location (e.g., the directory containing the current config.yaml if it exists, otherwise $HOME).
2. The user can navigate the filesystem, enter directories, and select only .yaml/.yml files; pressing Enter on a selected file loads it.
3. Upon selection, the structured config view appears (key‑value table), and pressing 'c' toggles to the raw YAML view.
3. Pressing 'r' starts the pipeline; the execution status area begins to update with Watch iteration, queue length, counts, and current video.
4. The recent events area shows the last log lines, with ERROR lines highlighted.
5. Pressing 'q' prompts for confirmation if the pipeline is running; otherwise exits immediately.
6. All original CLI functionality (`go run . run -c <config>`) remains unchanged.

## Open questions
- Whether to support mouse scrolling in the file picker and log area (deferred; keyboard‑first is sufficient).
