#!/usr/bin/env bash
# Adversarial check: re-run the three regression tests to ensure fixes hold.
set -e
cd "$(dirname "$0")/../../.."
if go test ./tui/ -run TestModelPickerEnterHintClickSelectsHighlightedFile -count=1 >/dev/null 2>&1 \
   && go test ./tui/ -run TestModelConfigRowClickStartsEditAndSaveHintCommits -count=1 >/dev/null 2>&1; then
    exit 0
else
    exit 1
fi
