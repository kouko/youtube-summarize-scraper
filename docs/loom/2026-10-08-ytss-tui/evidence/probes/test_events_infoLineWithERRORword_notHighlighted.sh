#!/usr/bin/env bash
# concern: an INFO-level log line containing the word "ERROR" must not be
#         highlighted as an error in the events panel
#
# REQ-4: Show recent log events — Acceptance #4
#   WHEN the pipeline emits log lines, the TUI shall display the last N log
#   lines (default 10) in a scrolling area, with ERROR-level lines highlighted
#   in red.
#
# The change highlights any line containing the substring "ERROR" regardless
# of log level; this probe verifies that only level=ERROR lines are highlighted.
set -euo pipefail
cd "$(dirname "$0")/../../../../.."
go test -race ./tui -run TestEventsInfoLineWithERRORwordNotHighlighted -count=1
