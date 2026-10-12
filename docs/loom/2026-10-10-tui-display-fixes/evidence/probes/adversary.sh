#!/usr/bin/env bash
# concern: adversarial check for TUI display fixes - ensures fixed correctness issues don't regress
# This program re-runs the two regression tests covering the fixed correctness issues:
#   TestModelPickerEnterHintClickSelectsHighlightedFile
#   TestModelConfigRowClickStartsEditAndSaveHintCommits
set -e
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$SCRIPT_DIR/../../../../.."
cd "$REPO_ROOT"
if go test ./tui/ -run TestModelPickerEnterHintClickSelectsHighlightedFile -count=1 >/dev/null 2>&1 \
   && go test ./tui/ -run TestModelConfigRowClickStartsEditAndSaveHintCommits -count=1 >/dev/null 2>&1; then
    exit 0
else
    exit 1
fi
