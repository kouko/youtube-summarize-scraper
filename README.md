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

**`make all` vs `make build`:**

- `make all` — **force-refreshes every dependency** (re-downloads the latest yt-dlp, recompiles ffmpeg + whisper), then builds. Use for releases or to pull the newest tools. Always recompiles ffmpeg/whisper from source (several minutes).
- `make build` — builds, **provisioning only missing dependencies**. Fast and offline once deps exist; the first run (or any missing dep) downloads/compiles it (ffmpeg + whisper need `cmake`/`nasm`). Use for day-to-day development.

To force-refresh a single dependency: `make deps FORCE=1`. For a full clean rebuild: `make clean && make all`.

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

#### OpenAI-compatible servers (single or multiple)

`openai-compat` is a **map of named instances**, so it can point at one or
several OpenAI-compatible servers (LM Studio, vLLM, oMLX, …) and fail over
between them.

Single server — bare `openai-compat` resolves to the `default` instance:

```yaml
llm:
  provider: "openai-compat"
  openai-compat:
    default:
      endpoint: "http://localhost:1234/v1"   # e.g. LM Studio (default port 1234)
      model: "qwen2.5-7b-instruct"
```

High availability across multiple boxes — list them in `provider` (first =
primary, rest = fallbacks); each instance gets its own circuit breaker:

```yaml
llm:
  provider: ["openai-compat:box1", "openai-compat:box2"]
  openai-compat:
    box1: { endpoint: "http://192.168.1.10:1234/v1", model: "qwen2.5-7b-instruct" }
    box2: { endpoint: "http://192.168.1.11:1234/v1", model: "qwen2.5-7b-instruct" }
```

Naming rules:

- Instance names are user-defined and unlimited; reference them as `openai-compat:<name>`.
- `default` is reserved — bare `openai-compat` resolves to it (omit it if you never use the bare form).
- Names must not contain a colon `:` (the prefix/instance separator).

> **Breaking change:** `openai-compat` is now a map. A pre-existing single-server
> config (`openai-compat: { endpoint: ... }`) must be nested under a `default:` key.

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
| `--llm` | Override LLM backend (`ollama`/`llamacpp`/`claude-api`/`claude-code`/`gemini-cli`/`antigravity-cli`/`qwen-code`/`openai-compat`; use `openai-compat:<name>` to target a named instance) |
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
