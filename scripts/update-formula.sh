#!/usr/bin/env bash
# chmod +x this file before use
# Update Homebrew formula with new version and SHA256 checksums.
# Usage: update-formula.sh <version> <checksums-file> <formula-file>
#
# Example:
#   update-formula.sh 1.2.3 checksums.txt homebrew/Formula/ytss.rb

set -euo pipefail

VERSION="${1:?Usage: update-formula.sh <version> <checksums-file> <formula-file>}"
CHECKSUMS_FILE="${2:?Missing checksums file}"
FORMULA_FILE="${3:?Missing formula file}"

if [[ ! -f "$CHECKSUMS_FILE" ]]; then
  echo "Error: checksums file not found: $CHECKSUMS_FILE" >&2
  exit 1
fi

if [[ ! -f "$FORMULA_FILE" ]]; then
  echo "Error: formula file not found: $FORMULA_FILE" >&2
  exit 1
fi

get_sha() {
  local pattern="$1"
  grep "$pattern" "$CHECKSUMS_FILE" | awk '{print $1}'
}

SHA_DARWIN_ARM64=$(get_sha "darwin-arm64")
SHA_LINUX_AMD64=$(get_sha "linux-amd64")
SHA_LINUX_ARM64=$(get_sha "linux-arm64")

for var in SHA_DARWIN_ARM64 SHA_LINUX_AMD64 SHA_LINUX_ARM64; do
  if [[ -z "${!var}" ]]; then
    echo "Error: could not find SHA256 for ${var} in $CHECKSUMS_FILE" >&2
    exit 1
  fi
done

cat > "$FORMULA_FILE" <<RUBY
class Ytss < Formula
  desc "YouTube batch subtitle scraper with whisper transcription and LLM summaries"
  homepage "https://github.com/kouko/youtube-summarize-scraper"
  version "${VERSION}"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/kouko/youtube-summarize-scraper/releases/download/v#{version}/ytss-darwin-arm64.tar.gz"
      sha256 "${SHA_DARWIN_ARM64}"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/kouko/youtube-summarize-scraper/releases/download/v#{version}/ytss-linux-arm64.tar.gz"
      sha256 "${SHA_LINUX_ARM64}"
    else
      url "https://github.com/kouko/youtube-summarize-scraper/releases/download/v#{version}/ytss-linux-amd64.tar.gz"
      sha256 "${SHA_LINUX_AMD64}"
    end
  end

  def install
    bin.install "ytss"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/ytss --version")
  end
end
RUBY

echo "Updated $FORMULA_FILE to v${VERSION}"
echo "  darwin-arm64: ${SHA_DARWIN_ARM64}"
echo "  linux-amd64:  ${SHA_LINUX_AMD64}"
echo "  linux-arm64:  ${SHA_LINUX_ARM64}"
