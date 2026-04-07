#!/bin/bash
# build-ffmpeg.sh — Build minimal static ffmpeg from source
#
# Builds ffmpeg with only the features needed for ytss:
# - Audio format conversion (WAV, opus, m4a, mp3)
# - Subtitle format conversion (vtt → srt)
# - Audio resampling (16kHz for whisper)
#
# Prerequisites:
#   macOS: xcode-select --install && brew install cmake nasm pkg-config
#   Linux: sudo apt install build-essential cmake nasm pkg-config
#
# Usage:
#   ./scripts/build-ffmpeg.sh [GOOS] [GOARCH]

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
OUTPUT_PATH="${OUTPUT_DIR}/ffmpeg"
mkdir -p "$OUTPUT_DIR"

if [ -f "$OUTPUT_PATH" ]; then
    echo "[INFO] ffmpeg already exists at $OUTPUT_PATH, skipping build"
    echo "[INFO] Delete the file to rebuild"
    exit 0
fi

# Temp build directory
TMPBASE="${TMPDIR:-/tmp}"
BUILD_DIR="${TMPBASE}/ytss-ffmpeg-build-$$"
mkdir -p "$BUILD_DIR"

cleanup() {
    echo "[INFO] Cleaning up build directory..."
    rm -rf "$BUILD_DIR"
}
trap cleanup EXIT

echo "[INFO] Building minimal static ffmpeg for ${TARGET_OS}-${TARGET_ARCH}..."

# Clone ffmpeg
FFMPEG_VERSION="n7.1"
echo "[INFO] Cloning ffmpeg ${FFMPEG_VERSION}..."
git clone --depth 1 --branch "$FFMPEG_VERSION" https://github.com/FFmpeg/FFmpeg.git "$BUILD_DIR/ffmpeg"
cd "$BUILD_DIR/ffmpeg"

# Configure — minimal build for audio + subtitle conversion
CONFIGURE_FLAGS=(
    --prefix="$BUILD_DIR/install"
    --enable-static
    --disable-shared
    --disable-doc
    --disable-htmlpages
    --disable-manpages
    --disable-podpages
    --disable-txtpages
    # Disable unnecessary components
    --disable-programs
    --enable-ffmpeg
    --disable-ffplay
    --disable-ffprobe
    # Disable video (we only need audio + subtitle)
    --disable-avdevice
    --disable-swscale
    --disable-postproc
    # Disable unnecessary features
    --disable-network
    --disable-debug
    --disable-runtime-cpudetect
    # Disable autodetection of external libraries that might exist on the
    # build host but not on end-user machines. Without these flags
    # ffmpeg's ./configure happily picks up Homebrew-installed libs
    # (e.g. libX11) and bakes their absolute paths into the resulting
    # binary, which then fails with
    # "Library not loaded: /opt/homebrew/opt/libx11/lib/..." on user
    # machines that don't have libx11 installed via Homebrew.
    # ytss only uses ffmpeg for audio/subtitle conversion, none of these
    # video-display features are needed.
    --disable-xlib
    --disable-libxcb
    --disable-libxcb-shm
    --disable-libxcb-shape
    --disable-libxcb-xfixes
    --disable-sdl2
    # Enable what we need
    --enable-small
)

# Platform-specific flags
case "$TARGET_OS" in
    darwin)
        # macOS: use native frameworks for audio decoding
        CONFIGURE_FLAGS+=(
            --enable-audiotoolbox
            --disable-videotoolbox
        )
        if [ "$TARGET_ARCH" = "arm64" ]; then
            CONFIGURE_FLAGS+=(--arch=aarch64 --enable-cross-compile --target-os=darwin)
        fi
        ;;
    linux)
        CONFIGURE_FLAGS+=(
            --extra-ldflags="-static"
            --extra-cflags="-static"
            --pkg-config-flags="--static"
        )
        ;;
esac

echo "[INFO] Configuring ffmpeg..."
./configure "${CONFIGURE_FLAGS[@]}"

echo "[INFO] Building ffmpeg..."
make -j"$(nproc 2>/dev/null || sysctl -n hw.ncpu)"

echo "[INFO] Installing ffmpeg binary..."
cp ffmpeg "$OUTPUT_PATH"
chmod +x "$OUTPUT_PATH"

echo "[SUCCESS] ffmpeg built at $OUTPUT_PATH"
"$OUTPUT_PATH" -version 2>/dev/null | head -1 || echo "[WARN] Could not verify version"
