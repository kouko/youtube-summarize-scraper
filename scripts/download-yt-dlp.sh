#!/bin/bash
# download-yt-dlp.sh — Download yt-dlp official binary for the target platform
#
# Usage:
#   ./scripts/download-yt-dlp.sh [GOOS] [GOARCH]
#   ./scripts/download-yt-dlp.sh              # auto-detect current platform
#   ./scripts/download-yt-dlp.sh darwin arm64  # specific platform

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

TARGET_OS="${1:-$(uname -s | tr '[:upper:]' '[:lower:]')}"
TARGET_ARCH="${2:-$(uname -m)}"

# Normalize arch
case "$TARGET_ARCH" in
    x86_64)        TARGET_ARCH="amd64" ;;
    arm64|aarch64) TARGET_ARCH="arm64" ;;
esac

OUTPUT_DIR="$PROJECT_DIR/embedded/bin/${TARGET_OS}-${TARGET_ARCH}"
mkdir -p "$OUTPUT_DIR"

# Determine download URL
BASE_URL="https://github.com/yt-dlp/yt-dlp/releases/latest/download"

case "${TARGET_OS}-${TARGET_ARCH}" in
    darwin-arm64|darwin-amd64)
        ASSET_NAME="yt-dlp_macos"
        ;;
    linux-amd64)
        ASSET_NAME="yt-dlp_linux"
        ;;
    linux-arm64)
        ASSET_NAME="yt-dlp_linux_aarch64"
        ;;
    *)
        echo "ERROR: Unsupported platform: ${TARGET_OS}-${TARGET_ARCH}" >&2
        exit 1
        ;;
esac

DOWNLOAD_URL="${BASE_URL}/${ASSET_NAME}"
OUTPUT_PATH="${OUTPUT_DIR}/yt-dlp"

if [ -f "$OUTPUT_PATH" ] && [ -z "${FORCE:-}" ]; then
    echo "[INFO] yt-dlp already exists at $OUTPUT_PATH, skipping download"
    echo "[INFO] Set FORCE=1 (or run 'make all') to re-download the latest"
    exit 0
fi

echo "[INFO] Downloading yt-dlp for ${TARGET_OS}-${TARGET_ARCH}..."
echo "[INFO] URL: $DOWNLOAD_URL"

curl -L --progress-bar -o "$OUTPUT_PATH" "$DOWNLOAD_URL"
chmod +x "$OUTPUT_PATH"

echo "[SUCCESS] yt-dlp downloaded to $OUTPUT_PATH"
"$OUTPUT_PATH" --version 2>/dev/null || echo "[WARN] Could not verify version (cross-platform binary)"
