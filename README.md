# YTSS — YouTube Summarize Scraper

A Go CLI tool that batch-processes YouTube channels to download subtitles, transcribe audio via whisper.cpp, and generate LLM-powered summaries.

## Features

- Batch process multiple YouTube channels from a YAML config
- 4-step cascading subtitle download (manual → auto → fallback)
- Automatic audio transcription via whisper.cpp when no subtitles available
- Language-specialized whisper models (kotoba-ja, belle-zh)
- Pluggable LLM backends: Ollama, llama.cpp, Claude API, Claude Code, Gemini CLI, Antigravity CLI, Qwen Code, OpenAI-compatible
- Three-stage LLM pipeline: summary → keywords → Mermaid flowchart
- Built-in prompt templates in English, Traditional Chinese, Japanese
- Skip already-processed videos (glob-based detection)
- Cookie support for restricted videos (auto-detect, Chrome multi-profile)
- Obsidian integration (tags, wikilinks, Dataview MOC)
- Self-contained binary with embedded yt-dlp, ffmpeg, whisper-cli

## Installation

### Homebrew (macOS / Linux)

```bash
brew install kouko/tap/ytss
```

Upgrade to the latest version:

```bash
brew upgrade ytss
```

### Build from Source

#### Prerequisites

Build dependencies (only needed for building, not for running):

```bash
# macOS
xcode-select --install
brew install cmake nasm

# Linux (Ubuntu/Debian)
sudo apt install build-essential cmake nasm pkg-config
```

#### Build

```bash
git clone https://github.com/kouko/youtube-summarize-scraper.git
cd youtube-summarize-scraper

# Download/build all dependencies and compile
make all
```

This will:
1. Download yt-dlp binary from official releases
2. Build ffmpeg from source (minimal audio/subtitle config)
3. Build whisper-cli from ggml-org/whisper.cpp (with Metal on macOS)
4. Compile the `ytss` Go binary with all tools embedded

## Usage

### Quick start

```bash
# Summarize a single video
ytss video https://www.youtube.com/watch?v=dQw4w9WgXcQ

# Summarize a single video by ID
ytss video dQw4w9WgXcQ

# Summarize latest 5 videos from a channel
ytss channel @YouTube -n 5

# Batch process all channels from config
ytss run
```

### Configuration

Copy and edit the example config:

```bash
cp config.example.yaml config.yaml
```

See [config.example.yaml](config.example.yaml) for all available options.

### Commands

| Command | Description |
|---------|-------------|
| `ytss run` | Batch process all channels from config.yaml |
| `ytss video <URL or ID>` | Summarize a single video |
| `ytss channel <URL or @handle> -n N` | Summarize latest N videos from a channel |

### Global Flags

| Flag | Description |
|------|-------------|
| `-c, --config` | Config file path (default: `./config.yaml`) |
| `-o, --output` | Output directory (overrides config) |
| `--llm` | Override LLM backend (`ollama`/`llamacpp`/`claude-api`/`claude-code`/`gemini-cli`/`antigravity-cli`/`qwen-code`/`openai-compat`) |
| `--cookie-file` | Path to cookie.txt (Netscape format) |
| `--cookie-browser` | Extract cookie from browser (`chrome`/`firefox`/`safari`/`edge`/`brave`) |
| `--force` | Force re-process even if output exists |
| `--dry-run` | List videos without processing |
| `-v, --verbose` | Verbose logging |

## Output Structure

```
output/
├── @channel-a/
│   ├── _index.md                          # Obsidian MOC (if enabled)
│   ├── 2026-03-20__dQw4w9WgXcQ__Video_Title/
│   │   ├── 2026-03-20__dQw4w9WgXcQ__subtitle.srt
│   │   ├── 2026-03-20__dQw4w9WgXcQ__transcription.md
│   │   └── 2026-03-20__dQw4w9WgXcQ__summary.md
```

## Architecture

```
ytss video <url>
     │
     ▼
┌─ Fetch Metadata (yt-dlp --dump-json) ─┐
│                                         │
▼                                         ▼
┌─ Subtitle Download ─┐    ┌─ Whisper Transcription ─┐
│ 1. Manual (target)   │    │ Audio → WAV 16kHz       │
│ 2. Auto (target)     │    │ Model selection by lang  │
│ 3. Manual (any)      │ OR │ whisper-cli → SRT        │
│ 4. Auto (any)        │    │                          │
└──────────┬───────────┘    └──────────┬───────────────┘
           │                           │
           ▼                           ▼
    ┌─ subtitle.srt ──────── transcription.md ─┐
    │                                           │
    ▼                                           │
┌─ Three-Stage LLM ────────────────────────────┐
│ Stage 1: Summary (prompt template)            │
│ Stage 2: Keywords extraction                  │
│ Stage 3: Mermaid flowchart                    │
└───────────────────┬───────────────────────────┘
                    ▼
              summary.md
```

## Design Spec

Full design specification: [docs/loom/specs/2026-03-22-ytss-design.md](docs/loom/specs/2026-03-22-ytss-design.md)

## License

MIT
