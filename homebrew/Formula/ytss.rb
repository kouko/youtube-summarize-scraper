class Ytss < Formula
  desc "YouTube batch subtitle scraper with whisper transcription and LLM summaries"
  homepage "https://github.com/kouko/youtube-summarize-scraper"
  version "0.0.0"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/kouko/youtube-summarize-scraper/releases/download/v#{version}/ytss-darwin-arm64.tar.gz"
      sha256 "PLACEHOLDER_DARWIN_ARM64"
    else
      url "https://github.com/kouko/youtube-summarize-scraper/releases/download/v#{version}/ytss-darwin-amd64.tar.gz"
      sha256 "PLACEHOLDER_DARWIN_AMD64"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/kouko/youtube-summarize-scraper/releases/download/v#{version}/ytss-linux-arm64.tar.gz"
      sha256 "PLACEHOLDER_LINUX_ARM64"
    else
      url "https://github.com/kouko/youtube-summarize-scraper/releases/download/v#{version}/ytss-linux-amd64.tar.gz"
      sha256 "PLACEHOLDER_LINUX_AMD64"
    end
  end

  def install
    bin.install "ytss"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/ytss --version")
  end
end
