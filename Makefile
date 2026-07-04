BINARY_NAME := ytss
GOOS ?= $(shell uname -s | tr '[:upper:]' '[:lower:]')
GOARCH ?= $(shell uname -m | sed 's/x86_64/amd64/' | sed 's/aarch64/arm64/')
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -X main.version=$(VERSION)

# FORCE=1 makes the dep scripts re-download/rebuild even when the binary
# already exists. Empty (default) = provision only what's missing.
FORCE ?=

.PHONY: all build clean download-deps build-deps deps build-all audit-embedded

# `all`: force-refresh EVERY dependency to the newest, then build.
# Use for releases / to pull the latest yt-dlp + whisper. Always recompiles
# ffmpeg + whisper from source (minutes) — that's the cost of the guarantee.
all: FORCE=1
all: deps audit-embedded build

# Download/build all external dependencies for current platform.
# Each script skips when its binary exists, unless FORCE is set.
deps: download-deps build-deps

download-deps:
	@echo "==> Downloading yt-dlp for $(GOOS)-$(GOARCH)..."
	@FORCE=$(FORCE) bash scripts/download-yt-dlp.sh $(GOOS) $(GOARCH)

build-deps:
	@echo "==> Building ffmpeg for $(GOOS)-$(GOARCH)..."
	@FORCE=$(FORCE) bash scripts/build-ffmpeg.sh $(GOOS) $(GOARCH)
	@echo "==> Building whisper-cli for $(GOOS)-$(GOARCH)..."
	@FORCE=$(FORCE) bash scripts/build-whisper.sh $(GOOS) $(GOARCH)

# Audit every binary under embedded/bin/<platform>/ for dyld load
# problems. Currently macOS-only (uses otool); the script skips
# gracefully on other platforms. Catches build-host-specific rpath
# entries, unresolved @rpath deps, and absolute-path deps outside
# /usr/lib + /System/Library.
audit-embedded:
	@echo "==> Auditing embedded binaries for dyld load problems..."
	@bash scripts/audit-embedded-binaries.sh embedded/bin/$(GOOS)-$(GOARCH)

# Build the Go binary. Depends on `deps` so a missing dependency is provisioned
# automatically (first run compiles ffmpeg + whisper from source — needs
# cmake/nasm). Present deps are skipped, so repeat builds stay fast and offline.
build: deps
	@echo "==> Building $(BINARY_NAME) for $(GOOS)-$(GOARCH)..."
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags "$(LDFLAGS)" -o $(BINARY_NAME) .

# Build for all supported platforms (sequential: deps → build per platform)
build-all:
	@echo "==> Building for darwin-arm64..."
	$(MAKE) deps GOOS=darwin GOARCH=arm64
	$(MAKE) build GOOS=darwin GOARCH=arm64
	@mv $(BINARY_NAME) dist/$(BINARY_NAME)-darwin-arm64 2>/dev/null || true
	@echo "==> Building for darwin-amd64..."
	$(MAKE) deps GOOS=darwin GOARCH=amd64
	$(MAKE) build GOOS=darwin GOARCH=amd64
	@mv $(BINARY_NAME) dist/$(BINARY_NAME)-darwin-amd64 2>/dev/null || true
	@echo "==> Building for linux-amd64..."
	$(MAKE) deps GOOS=linux GOARCH=amd64
	$(MAKE) build GOOS=linux GOARCH=amd64
	@mv $(BINARY_NAME) dist/$(BINARY_NAME)-linux-amd64 2>/dev/null || true
	@echo "==> All platforms built!"

# Clean build artifacts
clean:
	rm -f $(BINARY_NAME)
	rm -rf dist/
	rm -rf embedded/bin/

# Clean only Go build (keep deps)
clean-build:
	rm -f $(BINARY_NAME)
	rm -rf dist/

# Show current platform
info:
	@echo "GOOS=$(GOOS) GOARCH=$(GOARCH)"
	@echo "Deps dir: embedded/bin/$(GOOS)-$(GOARCH)/"
	@ls -la embedded/bin/$(GOOS)-$(GOARCH)/ 2>/dev/null || echo "(no deps downloaded yet)"
