#!/bin/bash
# Temporary script: move videos from @unknown/ to correct channel directories.
# Reads channel handle from summary.md frontmatter (no yt-dlp needed).

set -euo pipefail

OUTPUT_DIR="/Users/kouko/VisualStudioCodeProject/youtube-summarize-scraper/ytss-output"
UNKNOWN_DIR="$OUTPUT_DIR/@unknown"

if [ ! -d "$UNKNOWN_DIR" ]; then
    echo "No @unknown directory found."
    exit 0
fi

moved=0
failed=0

for dir in "$UNKNOWN_DIR"/*/; do
    [ -d "$dir" ] || continue
    dirname=$(basename "$dir")

    # Find summary.md in this directory
    summary=$(find "$dir" -maxdepth 1 -name "*__summary.md" -print -quit 2>/dev/null)
    if [ -z "$summary" ]; then
        echo "SKIP (no summary): $dirname"
        ((failed++)) || true
        continue
    fi

    # Extract channel handle from frontmatter: channel: "@handle"
    channel=$(grep -m1 '^channel:' "$summary" | sed 's/^channel: *"*@*\([^"]*\)"*/\1/')
    if [ -z "$channel" ] || [ "$channel" = "unknown" ]; then
        echo "SKIP (no channel in frontmatter): $dirname"
        ((failed++)) || true
        continue
    fi

    # Target directory
    channel_dir="$OUTPUT_DIR/@${channel}"
    new_path="$channel_dir/$dirname"

    if [ -d "$new_path" ]; then
        echo "SKIP (target exists): $dirname -> @${channel}/$dirname"
        ((failed++)) || true
        continue
    fi

    # Move
    mkdir -p "$channel_dir"
    mv "$dir" "$new_path"
    echo "MOVED: @unknown/$dirname -> @${channel}/$dirname"
    ((moved++)) || true
done

echo ""
echo "Done: $moved moved, $failed skipped"

# Clean up empty @unknown directory
if [ -d "$UNKNOWN_DIR" ] && [ -z "$(ls -A "$UNKNOWN_DIR")" ]; then
    rmdir "$UNKNOWN_DIR"
    echo "Removed empty @unknown directory"
fi
