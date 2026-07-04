#!/bin/bash
# build-whisper.sh — Build whisper-cli from ggml-org/whisper.cpp
#
# Builds with Metal acceleration on macOS, CPU-only on Linux.
#
# Prerequisites:
#   macOS: xcode-select --install && brew install cmake
#   Linux: sudo apt install build-essential cmake
#
# Usage:
#   ./scripts/build-whisper.sh [GOOS] [GOARCH]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

TARGET_OS="${1:-$(uname -s | tr '[:upper:]' '[:lower:]')}"
TARGET_ARCH="${2:-$(uname -m)}"

case "$TARGET_ARCH" in
    x86_64)        TARGET_ARCH="amd64" ;;
    arm64|aarch64) TARGET_ARCH="arm64" ;;
esac

OUTPUT_DIR="$PROJECT_DIR/embedded/bin/${TARGET_OS}-${TARGET_ARCH}"
OUTPUT_PATH="${OUTPUT_DIR}/whisper-cli"
mkdir -p "$OUTPUT_DIR"

if [ -f "$OUTPUT_PATH" ] && [ -z "${FORCE:-}" ]; then
    echo "[INFO] whisper-cli already exists at $OUTPUT_PATH, skipping build"
    echo "[INFO] Set FORCE=1 (or run 'make all') to rebuild"
    exit 0
fi

# Temp build directory
TMPBASE="${TMPDIR:-/tmp}"
BUILD_DIR="${TMPBASE}/ytss-whisper-build-$$"
mkdir -p "$BUILD_DIR"

cleanup() {
    echo "[INFO] Cleaning up build directory..."
    rm -rf "$BUILD_DIR"
}
trap cleanup EXIT

echo "[INFO] Building whisper-cli for ${TARGET_OS}-${TARGET_ARCH}..."

# Clone whisper.cpp
echo "[INFO] Cloning whisper.cpp..."
git clone --depth 1 https://github.com/ggml-org/whisper.cpp.git "$BUILD_DIR/whisper.cpp"
cd "$BUILD_DIR/whisper.cpp"

# Platform-specific cmake flags
CMAKE_FLAGS=(
    -DCMAKE_BUILD_TYPE=Release
    -DBUILD_SHARED_LIBS=OFF
)

case "$TARGET_OS" in
    darwin)
        echo "[INFO] macOS detected — enabling Metal acceleration"
        CMAKE_FLAGS+=(
            -DGGML_METAL=ON
        )
        ;;
    linux)
        echo "[INFO] Linux detected — CPU-only build"
        CMAKE_FLAGS+=(
            -DGGML_METAL=OFF
        )
        ;;
    *)
        echo "ERROR: Unsupported OS: $TARGET_OS" >&2
        exit 1
        ;;
esac

echo "[INFO] Configuring with cmake..."
cmake -B build "${CMAKE_FLAGS[@]}"

echo "[INFO] Building..."
cmake --build build -j"$(nproc 2>/dev/null || sysctl -n hw.ncpu)" --config Release

# Find the built binary
BUILT_BINARY=""
for candidate in build/bin/whisper-cli build/bin/whisper; do
    if [ -f "$candidate" ]; then
        BUILT_BINARY="$candidate"
        break
    fi
done

if [ -z "$BUILT_BINARY" ]; then
    echo "ERROR: Could not find built whisper-cli binary" >&2
    echo "Available files in build/bin/:" >&2
    ls -la build/bin/ 2>/dev/null || echo "(directory not found)"
    exit 1
fi

echo "[INFO] Installing whisper-cli binary..."
cp "$BUILT_BINARY" "$OUTPUT_PATH"
chmod +x "$OUTPUT_PATH"

echo "[SUCCESS] whisper-cli built at $OUTPUT_PATH"
"$OUTPUT_PATH" --version 2>/dev/null || echo "[INFO] Version check skipped"
