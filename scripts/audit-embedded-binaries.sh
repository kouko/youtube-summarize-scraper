#!/usr/bin/env bash
# Audit every binary under embedded/bin/<platform>/ for dyld load problems
# that would blow up on end-user machines.
#
# Specifically looks for:
#   1. LC_RPATH entries that point into build-host-specific paths
#      (e.g. /Users/runner/... on GitHub Actions, /Users/kouko/... locally).
#      Those paths do not exist on the user's machine and cause
#      "dyld: Library not loaded" at spawn time.
#   2. @rpath dependencies where the referenced dylib is not bundled
#      alongside the binary in the same embedded/bin directory.
#   3. Absolute LC_LOAD_DYLIB entries pointing outside /usr/lib and
#      /System/Library — most commonly to /opt/homebrew/... when a
#      build script's autoconf/cmake step auto-detects a Homebrew-
#      installed library on the build host and bakes its path in.
#      This is exactly how ffmpeg's ./configure silently picks up
#      /opt/homebrew/opt/libx11/lib/libX11.6.dylib on GHA runners.
#
# System dylibs (/usr/lib/... and /System/Library/...) are allowed.
# Weak-linked deps are allowed because dyld tolerates them being
# absent.
#
# Current implementation is **macOS-only** (uses otool). Linux ELF
# auditing would require readelf or patchelf and a different check
# matrix; when a Linux build path exists, the script exits 0 with a
# note rather than failing. Today ytss's Linux builds use fully-static
# linking (--extra-ldflags=-static) so the bug class is mostly
# impossible there anyway.
#
# Run locally after `make deps`, or in CI (darwin matrix jobs only)
# to fail the release pipeline before shipping a broken binary.
#
# Usage: bash scripts/audit-embedded-binaries.sh [embedded/bin/<platform>]
#        (defaults to embedded/bin/darwin-<arch> for the host)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

if [[ $# -ge 1 ]]; then
  TARGET_DIR="$1"
else
  ARCH="$(uname -m)"
  case "$ARCH" in
    arm64|aarch64) ARCH="arm64" ;;
    x86_64|amd64)  ARCH="amd64" ;;
  esac
  TARGET_DIR="$PROJECT_ROOT/embedded/bin/darwin-$ARCH"
fi

# Derive the platform slug from the target dir name (last path component).
# The caller's Makefile is expected to pass e.g. embedded/bin/darwin-arm64
# or embedded/bin/linux-amd64; we use the prefix to decide whether to
# audit or skip.
PLATFORM_SLUG="$(basename "$TARGET_DIR")"
case "$PLATFORM_SLUG" in
  darwin-*) ;;
  linux-*|windows-*)
    echo "==> Skipping audit for $PLATFORM_SLUG (Mach-O-only tooling)"
    echo "    Static-build flags in build-ffmpeg.sh / build-whisper.sh are"
    echo "    the primary defence on this platform; ELF RPATH auditing is"
    echo "    a future enhancement."
    exit 0
    ;;
  *)
    echo "audit: unknown platform slug: $PLATFORM_SLUG" >&2
    exit 1
    ;;
esac

if [[ ! -d "$TARGET_DIR" ]]; then
  echo "audit: target directory not found: $TARGET_DIR" >&2
  echo "audit: run 'make deps' first" >&2
  exit 1
fi

echo "==> Auditing embedded binaries in $TARGET_DIR"

FAIL=0

# Collect shipped dylib filenames so we can verify @rpath deps against it.
shopt -s nullglob
SHIPPED_DYLIBS=()
for f in "$TARGET_DIR"/*.dylib; do
  SHIPPED_DYLIBS+=("$(basename "$f")")
done
shopt -u nullglob

is_shipped() {
  local name="$1"
  local shipped
  for shipped in "${SHIPPED_DYLIBS[@]:-}"; do
    [[ "$shipped" == "$name" ]] && return 0
  done
  return 1
}

# Mach-O magic bytes: 0xcafebabe (fat), 0xfeedface (32-bit), 0xfeedfacf
# (64-bit), and their byte-swapped counterparts. We skip non-Mach-O files
# like model blobs or Python zipapps.
is_macho() {
  local file="$1"
  local magic
  magic="$(xxd -p -l 4 "$file" 2>/dev/null || true)"
  case "$magic" in
    cafebabe|bebafeca|feedface|cefaedfe|feedfacf|cffaedfe) return 0 ;;
    *) return 1 ;;
  esac
}

check_binary() {
  local bin="$1"
  local name
  name="$(basename "$bin")"
  echo "    $name"

  local bad_rpaths
  bad_rpaths="$(otool -l "$bin" 2>/dev/null \
    | awk '/cmd LC_RPATH/{flag=1; next} flag && /path /{print $2; flag=0}' \
    | grep -E '^/Users/|^/home/|/pkg/mod/|whisper\.cpp/build|sherpa-onnx-go-macos|ytss-ffmpeg-build' \
    || true)"
  if [[ -n "$bad_rpaths" ]]; then
    echo "      ERROR: build-host-specific LC_RPATH entries:"
    while IFS= read -r rpath; do
      echo "        $rpath"
    done <<< "$bad_rpaths"
    FAIL=1
  fi

  # otool -L format:
  #   <binary>:
  #     /usr/lib/libfoo.dylib (compatibility version ..., current version ...)
  #     @rpath/libbar.dylib (compatibility version ..., current version ..., weak)
  #
  # We check three categories of failure:
  #
  # 1. @rpath deps where the referenced dylib is not bundled in
  #    $TARGET_DIR. dyld will try to resolve via LC_RPATH at runtime;
  #    if no companion dylib exists in any rpath dir, the load aborts.
  #
  # 2. Absolute-path deps outside /usr/lib and /System/Library.
  #    Anything in /opt/homebrew, /Users, /private, /tmp, etc. will only
  #    exist on the build host. Catches the libX11-from-Homebrew class
  #    of bug.
  #
  # Weak deps are skipped in both cases because dyld tolerates them
  # being missing.
  # otool -L prints a section header for each architecture in a fat
  # binary, e.g. "embedded/bin/darwin-arm64/yt-dlp (architecture arm64):"
  # Those headers do NOT start with whitespace; real dep lines DO start
  # with a tab. Filter to whitespace-prefixed lines so we skip every
  # architecture header as well as the top-level binary header.
  local nonweak_deps
  nonweak_deps="$(otool -L "$bin" 2>/dev/null \
    | grep -E '^[[:space:]]' \
    | grep -v ', weak)$' \
    | awk '{print $1}' \
    || true)"

  local dep
  while IFS= read -r dep; do
    [[ -z "$dep" ]] && continue
    case "$dep" in
      @rpath/*)
        local dep_name="${dep#@rpath/}"
        if ! is_shipped "$dep_name"; then
          echo "      ERROR: unresolved @rpath dep: $dep (no $dep_name in $TARGET_DIR)"
          FAIL=1
        fi
        ;;
      @loader_path/*|@executable_path/*)
        # Runtime-relative, skipped — resolution depends on the binary's
        # location at runtime, not at audit time.
        ;;
      /usr/lib/*|/System/Library/*)
        # macOS system libraries — always present.
        ;;
      /*)
        # Any other absolute path is build-host-specific. The most
        # common offender is /opt/homebrew/... when a build script's
        # autoconf/cmake step picks up a Homebrew-installed library.
        echo "      ERROR: non-system absolute dep: $dep"
        echo "             (this path will not exist on end-user machines)"
        FAIL=1
        ;;
      *)
        # Relative paths or unknown format — flag for inspection.
        echo "      WARNING: unrecognised dep format: $dep"
        ;;
    esac
  done <<< "$nonweak_deps"
}

for f in "$TARGET_DIR"/*; do
  [[ -f "$f" ]] || continue
  [[ "$(basename "$f")" == .* ]] && continue
  if is_macho "$f"; then
    check_binary "$f"
  fi
done

if [[ "$FAIL" -ne 0 ]]; then
  echo
  echo "audit: FAILED — one or more embedded binaries have bad dyld metadata."
  echo "audit: these binaries will abort with 'Library not loaded' on end-user machines."
  exit 1
fi

echo "==> Audit passed"
