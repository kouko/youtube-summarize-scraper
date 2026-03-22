# YTSS (YouTube Summarize Scraper) — Design Spec

## Overview

A single Go binary CLI tool that reads a list of YouTube channels from a YAML config, fetches the latest N videos from each channel, downloads subtitles (or transcribes audio via whisper.cpp), generates summaries via configurable LLM backends, and saves all output locally. Already-processed videos are automatically skipped.

External tools (`yt-dlp`, `whisper.cpp`) are embedded in the binary via `go:embed` and released to a cache directory at runtime. Whisper models are downloaded on demand.

**Important:** Since embedded binaries are platform-specific, each target platform requires its own build with the correct binaries staged. Cross-compilation is a per-platform sequential process. All paths using `~` (e.g., `~/.ytss/`) are resolved at runtime via `os.UserHomeDir()`.

## CLI Interface

### Commands

```
ytss run                            # Read config.yaml, batch process all channels
ytss video <URL or VIDEO_ID>        # Summarize a single video
ytss channel <URL or @handle> -n 5  # Summarize latest N videos from a channel
```

### Global Flags

```
--config, -c       Config file path (default: ./config.yaml)
--output, -o       Output directory (default: ./ytss-output, overridable in config)
--llm              Override LLM backend (ollama / llamacpp / claude-api / gemini-cli)
--cookie-file      Path to cookie.txt (Netscape format)
--cookie-browser   Auto-extract cookie from browser (chrome / firefox / safari / edge / brave)
--force            Force re-process even if output already exists (skip cache)
--dry-run          List videos that would be processed without executing
--verbose, -v      Verbose logging
```

`ytss video` and `ytss channel` work standalone without config.yaml (using defaults). When no config exists, the default LLM provider is `ollama` at `localhost:11434`. If the LLM is unreachable, subtitle and transcription are still produced; only the summary step is skipped with a warning.

**Output directory for `ytss video` and `ytss channel`:** Output follows the same structure as `ytss run`: `ytss-output/@channel-handle/YYYY-MM-DD__id__title/`. Channel handle is derived from yt-dlp's `uploader_id` field (e.g., `@HighYield`).

## Config File

```yaml
# Output
output_dir: "./ytss-output"

# Subtitle language preferences (optional, supports regex via yt-dlp)
# If unset: detect video original language → fallback to English
# Note: YouTube requires zh-Hant/zh-Hans, not bare "zh"
preferred_languages:
  - ja
  - "zh-Hant,zh-Hans"               # Comma = yt-dlp priority list for Chinese
  - en

# Default video count per channel
default_count: 5

# Whisper settings
whisper:
  model_dir: "~/.ytss/models"
  default_model: "medium"             # Fallback model for unmatched languages

  language_models:                   # Language-specific model overrides (ISO 639-1 keys)
    ja: "kotoba-ja"                  # Japanese-specialized (kotoba-tech, 1.4GB)
    zh: "belle-zh"                   # Chinese-specialized (BELLE-2, 1.5GB)
    en: "medium"                     # zh key matches zh-Hant, zh-Hans, zh-TW, etc.

  model_sources:                     # Download URLs (optional, defaults to HuggingFace)
    tiny: "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-tiny.bin"
    base: "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.bin"
    small: "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small.bin"
    medium: "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-medium.bin"
    large-v3: "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-large-v3.bin"
    large-v3-turbo: "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-large-v3-turbo.bin"
    belle-zh: "https://huggingface.co/BELLE-2/Belle-whisper-large-v3-turbo-zh-ggml/resolve/main/ggml-model.bin"
    kotoba-ja: "https://huggingface.co/kotoba-tech/kotoba-whisper-v2.0-ggml/resolve/main/ggml-model.bin"
    kotoba-ja-q5: "https://huggingface.co/kotoba-tech/kotoba-whisper-v2.0-ggml/resolve/main/ggml-model-q5.bin"

# Cookie settings (optional)
cookie:
  file: ""                           # Path to cookie.txt
  browser: ""                        # chrome / firefox / safari / edge / brave
  chrome_profile: ""                 # Chrome profile name (e.g., "Default", "Profile 1")

# LLM settings
llm:
  provider: "ollama"
  ollama:
    model: "llama3"
    endpoint: "http://localhost:11434"
    think: false                     # Enable thinking mode (better quality, slower, more tokens)
    timeout: 900                     # Seconds per LLM request (default: 900 = 15min)
  llamacpp:
    endpoint: "http://localhost:8080"
  claude_api:
    api_key: "${CLAUDE_API_KEY}"
    model: "claude-sonnet-4-20250514"
  gemini_cli:
    model: "gemini-2.5-pro"          # Model name
    path: ""                         # Path to gemini binary (default: search in PATH)

# Summary settings
summary:
  language: "zh-Hant"                # Selects built-in prompt language (en / zh-Hant / ja)
  # Inline prompt (for simple prompts, overrides built-in prompt)
  prompt: ""
  # External prompt file (takes precedence over inline and built-in prompt)
  summary_prompt_file: ""            # e.g., "./prompts/summary-prompt.md"
  max_tokens: 2000
  keywords:
    enabled: true                    # Enable LLM keyword extraction (default: true)
    language: "zh-Hant"              # Keyword language (default: en)
    count: 10                        # Max number of keywords
  mermaid:
    enabled: true                    # Enable Mermaid flowchart generation (default: true)

# Video filter settings
filter:
  types: ["video", "live", "short"]  # video / live / short (default: all types)
  min_duration: 0                    # Min seconds (0 = no filter)
  max_duration: 0                    # Max seconds (0 = no limit)

# Obsidian integration (optional)
obsidian:
  enabled: false
  auto_tags: ["youtube"]
  generate_moc: true
  wikilinks: true

# Channel list
channels:
  - url: "https://www.youtube.com/@channel-a"
    count: 10                        # Override default_count
    summary_prompt_file: "./prompts/tech-summary.md"  # Per-channel override
    cookie:                          # Per-channel cookie (optional, overrides global)
      browser: "chrome"
      chrome_profile: "Profile 2"
    filter:                          # Per-channel filter override
      types: ["video"]               # This channel: videos only
      min_duration: 60
  - url: "https://www.youtube.com/@channel-b"
  - url: "https://www.youtube.com/@channel-c"

# Playlist list
playlists:
  - url: "https://www.youtube.com/playlist?list=WL"
    name: "稍後觀看"              # Display name (optional, auto-detected from yt-dlp)
    count: 10                     # Max videos to process (optional, default: default_count)
    summary_prompt_file: ""       # Per-playlist prompt override (optional)
    cookie:                       # Per-playlist cookie (optional, overrides global)
      browser: "chrome"
      chrome_profile: "Profile 1"
  - url: "https://www.youtube.com/playlist?list=PLxxxxx"
```

## Output Structure

```
ytss-output/
├── @channel-a/
│   ├── 2026-03-20__dQw4w9WgXcQ__Rick_Astley_Never_Gonna_Give_You_Up/
│   │   ├── 2026-03-20__dQw4w9WgXcQ__subtitle.srt
│   │   ├── 2026-03-20__dQw4w9WgXcQ__transcription.md
│   │   └── 2026-03-20__dQw4w9WgXcQ__summary.md
│   └── 2026-03-18__abc123xyz__Another_Video_Title/
│       ├── 2026-03-18__abc123xyz__subtitle.srt
│       ├── 2026-03-18__abc123xyz__transcription.md
│       └── 2026-03-18__abc123xyz__summary.md
```

### Naming Rules

- Folder: `YYYY-MM-DD__{video_id}__{sanitized_title}`
- Files: `YYYY-MM-DD__{video_id}__{type}.{ext}`
- Date: video upload date
- Sanitized title: special characters and spaces removed, length limited
- `transcription.md`: subtitle content with SRT formatting stripped, plain text only. YouTube auto-generated subtitles use a rolling format where each sentence appears in 2-3 consecutive SRT blocks. `SRTToText` deduplicates consecutive identical lines after stripping timestamps.
- `summary.md` and `transcription.md` include a YAML frontmatter header with video metadata

### Frontmatter

Both `transcription.md` and `summary.md` start with a YAML frontmatter block. All fields are always present; empty values use `""` for strings and `[]` for lists.

**transcription.md:**
```yaml
---
title: "2026-03-20 Video Title (transcription)"
video_id: "dQw4w9WgXcQ"
url: "https://www.youtube.com/watch?v=dQw4w9WgXcQ"
channel: "@channel-a"
channel_name: "Channel A"
upload_date: "2026-03-20"
duration: "12:34"
language: "ja"
tags: ["tag1", "tag2"]
categories: ["Science & Technology"]
subtitle_type: "manual"
processed_at: "2026-03-22T15:30:00+08:00"
---
```

The `title` field is formatted as `YYYY-MM-DD Video Title (type)`:
- transcription.md → `"2026-03-20 Video Title (transcription)"`
- summary.md → `"2026-03-20 Video Title (summary)"`

**summary.md** includes additional fields:
```yaml
keywords: ["AI", "機器學習"]         # LLM-generated keywords ([] if extraction failed)
llm_provider: "ollama"
llm_model: "llama3"
```

**Tags and keywords in Obsidian mode (`obsidian.enabled: true`):**
- `tags` is enriched: YouTube original tags + `auto_tags` + channel name + `keywords` (all merged)
- `keywords` field is always kept separately for programmatic access (e.g., Claude Code)

**Tags and keywords in non-Obsidian mode:**
- `tags`: YouTube original tags only
- `keywords`: LLM-generated keywords only

### Obsidian Integration

Optional Obsidian-specific features, disabled by default. When enabled, output files are optimized for use inside an Obsidian vault.

**Config:**
```yaml
obsidian:
  enabled: false
  auto_tags: ["youtube"]           # Tags automatically appended to frontmatter tags
  generate_moc: true               # Generate channel MOC (Map of Content) index
  wikilinks: true                  # Use wikilinks in summary to link transcription
```

**When `obsidian.enabled: true`:**

1. **Tags enrichment** — `auto_tags` values (e.g., `youtube`) and sanitized channel name are appended to the frontmatter `tags` list
2. **Wikilinks** — `summary.md` includes a wikilink to the corresponding transcription file:
   ```markdown
   > Full transcription: [[2026-03-20__dQw4w9WgXcQ__transcription]]
   ```
3. **Channel MOC** — each channel directory gets a `_index.md` with a Dataview query:
   ```markdown
   # @channel-a

   ```dataview
   TABLE upload_date, duration, subtitle_type
   FROM "YouTube/@channel-a"
   WHERE video_id != null
   SORT upload_date DESC
   ```
   ```
   The MOC is regenerated on each `ytss run` to include new videos.

### Summary Prompt Template

Stage 1 prompt resolution order (see "Built-in Prompt Templates" section for details):
1. Per-channel `summary_prompt_file` (if set)
2. Global `summary.summary_prompt_file` (if set)
3. Global `summary.prompt` (inline, if set)
4. Built-in prompt for `summary.language` (default)

External prompt files and built-in prompts support variable substitution using `{{variable}}` syntax:

| Variable | Description |
|----------|-------------|
| `{{title}}` | Video title |
| `{{channel_name}}` | Channel display name |
| `{{language}}` | Detected language code |
| `{{upload_date}}` | Upload date (YYYY-MM-DD) |
| `{{duration}}` | Video duration |
| `{{tags}}` | Comma-separated tags |
| `{{transcript}}` | Full transcription text |
| `{{transcription_length}}` | Transcription character count |
| `{{transcription_tier}}` | Size tier label, language-aware: zh-Hant uses 字 (e.g., "500-3,000 字"), en uses chars (e.g., "1,000-5,000 chars"), ja uses 文字 (e.g., "500-3,000 文字"). Thresholds differ by language (CJK: lower thresholds due to higher info density per char) |

For inline `summary.prompt`, the transcript is automatically appended after the prompt text. Variable substitution is not available in inline mode.

### Channel Video Fetching

Uses a two-phase approach: fast listing via `--flat-playlist`, then on-demand full metadata fetch only for new videos.

**Phase 1 — Lightweight listing:**

1. Select YouTube channel tab URL(s) based on `filter.types` config:
   - `["video"]` → `<channel_url>/videos`
   - `["live"]` → `<channel_url>/streams`
   - `["short"]` → `<channel_url>/shorts`
   - Multiple types → one request per tab (e.g., `["video", "live"]` → `/videos` + `/streams`)
   - Unset or all types → `/videos` + `/streams` + `/shorts` (three requests)
2. `yt-dlp --flat-playlist --dump-json --playlist-end N <tab_url>` — fetches lightweight metadata (id, title, duration, description). N = `count + 5`. Type filtering is handled at the tab URL level, not program level.
3. Apply `min_duration` / `max_duration` filter on the listing results.
4. Take the first N videos that pass the filter.

**Phase 2 — Skip check + on-demand fetch:**

5. For each video, run global skip detection (see below) **before** fetching full metadata.
6. Only for non-skipped videos: fetch full metadata via `yt-dlp --dump-json <video_url>`, then process.

This approach avoids fetching full metadata for already-processed videos, reducing per-channel time from ~1 min to ~8s when all videos are already processed.

**Note:** `--flat-playlist` returns `duration` as float (e.g., `1434.0`) and does not include `live_status`, `upload_date`, `tags`, `channel`, or `media_type`. These fields are populated in Phase 2 via full metadata fetch.

### Playlist Processing

Playlists are processed before channels in `ytss run`. Each playlist uses `yt-dlp --flat-playlist` for listing, then on-demand full metadata fetch for new videos (same two-phase approach as channels).

**Output directory:** `_playlist__{playlist_id}__{sanitized_name}/`
- Example: `_playlist__WL__稍後觀看/`, `_playlist__PLxxxxx__My_Favorites/`
- Uses double-underscore separator matching video directory naming convention

**Frontmatter additions** (both transcription.md and summary.md):
- `playlist: "稍後觀看"` — playlist name (empty for channel videos)
- `playlist_id: "WL"` — playlist ID (empty for channel videos)
- `channel` and `channel_name` still reflect the video's original channel

**Processing flow:**
1. `yt-dlp --flat-playlist --dump-json <playlist_url>` (with cookie if configured)
2. Apply duration filter (min/max)
3. Take first N videos (playlist order preserved)
4. Per video: global skip check → fetch full metadata → ProcessVideo
5. Random delay between playlists (batch settings)

**Skip detection:** See "Skip Detection" section below. A video processed via channel will be copied (not re-processed) to the playlist directory, and vice versa.

**Playlist name resolution:** If `name` is not set in config, it is auto-detected from yt-dlp metadata (`playlist_title` field).

### Skip Detection

**Process for each video:**

1. `FindVideoDir(outputDir, videoID)` — search for existing `*__videoID__*` directory across all channel and playlist directories.

2. **If found (existing directory exists):**
   - **Same directory** (source == target):
     - Has `summary.md` → **skip** (fully complete)
     - No `summary.md` → **resume** (read existing transcription, only generate summary)
   - **Cross-directory** (source ≠ target, e.g., channel → playlist or playlist → playlist):
     - Source has `summary.md` → **copy** all files to target directory, update frontmatter (`playlist`, `playlist_id`, `channel` fields), then skip
     - Source has no `summary.md` → treat as **new video** (don't copy incomplete data)

3. **If not found** → **new video**, full pipeline processing.

**Cross-directory copy details:**
- All files (subtitle.srt, transcription.md, summary.md) are copied to the target directory
- Frontmatter in transcription.md and summary.md is updated to reflect the target context:
  - Copying to playlist: set `playlist` and `playlist_id`
  - Copying to channel: clear `playlist` and `playlist_id`
- File names are preserved (same prefix format)
- `processed_at` is updated to the copy timestamp

**Helper functions:**
- `FindVideoDir(outputDir, videoID)` — glob `outputDir/*/**/*__videoID__*`, returns first match or ""
- `HasFile(videoDir, suffix)` — check if `*__suffix` exists in dir
- `IsProcessed(outputDir, channelHandle, videoID)` — per-channel check with summary.md (used by `ytss video`)
- `IsProcessedGlobal(outputDir, videoID)` — global check with summary.md

All methods match on video ID only, resilient to title changes or sanitization logic updates.

When `--force` flag is set, skip detection is bypassed and existing output is overwritten (no copy).

## Core Pipeline

```
┌─────────────────────────────────────────────────────────────────┐
│                         ytss run                                │
│                                                                 │
│  ┌──────────┐   ┌──────────────┐   ┌────────────────────────┐  │
│  │ Read     │──▶│ Fetch latest │──▶│ Filter by type/  │──▶│ Skip if video  │  │
│  │ config   │   │ videos per   │   │ duration, take   │   │ ID folder      │  │
│  │          │   │ channel      │   │ first N matches  │   │ already exists  │  │
│  └──────────┘   └──────────────┘   └───────────┬────────────┘  │
│                                                 ▼               │
│                                    ┌────────────────────────┐  │
│                                    │ Subtitle Strategy      │  │
│                                    │                        │  │
│                                    │ 1. Preferred languages │  │
│                                    │    set in config?      │  │
│                                    │    ├─ Yes → find match │  │
│                                    │    └─ No → detect      │  │
│                                    │         original lang  │  │
│                                    │ 2. Manual subs first   │  │
│                                    │ 3. Auto subs second    │  │
│                                    │ 4. Fallback → English  │  │
│                                    │ 5. None → whisper      │  │
│                                    └───────────┬────────────┘  │
│                                                 ▼               │
│                              ┌──────────────────────────────┐  │
│                              │ Generate Output Files        │  │
│                              │                              │  │
│                              │ • subtitle.srt  (raw subs)   │  │
│                              │ • transcription.md (text)    │  │
│                              │ • summary.md (LLM summary)   │  │
│                              └──────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

**Important:** In `ProcessVideo`, full metadata is fetched first (Step 1), then channel handle is derived (Step 2). This ensures `ytss video` commands have complete metadata before any processing begins.

### Three-Stage LLM Call

Summary generation uses three sequential LLM calls. The input for Stage 1 is the **transcription** (plain text from transcription.md, not the raw SRT subtitle).

```
Stage 1: Generate Summary
    Input:  transcription text (via prompt template)
    Output: summary text (free-form markdown)
    Fail:   skip summary.md entirely, log error
         ↓
Stage 2: Extract Keywords (if summary.keywords.enabled)
    Input:  summary text from Stage 1
    Prompt: auto-generated based on keywords.language and keywords.count
    Output: one keyword per line → parse into list
    Fail:   keywords: [] in frontmatter, flow continues normally
         ↓
Stage 3: Generate Mermaid Flowchart (if summary.mermaid.enabled)
    Input:  summary text from Stage 1
    Prompt: auto-generated, requesting graph TD with simple syntax
    Output: Mermaid code block
    Fail:   skip Mermaid section, flow continues normally
         ↓
Program assembles final summary.md:
    frontmatter (metadata + keywords)
    + summary text (概述 section)
    + Mermaid flowchart (if valid, inserted after 概述 and before 章節摘要)
    + summary text (remaining sections)
```

- All three stages use the **same LLM provider**
- Stages execute **sequentially** (not parallel, to avoid overloading local LLM)
- Stage 2 and 3 input is the short summary (not the full transcription), so they are fast
- Stage 2 failure is non-blocking — `keywords` defaults to `[]`
- Stage 3 failure is non-blocking — Mermaid section is simply omitted
- Stage 3 output is validated for Mermaid syntax before inclusion
- Stage 3 prompt language follows `summary.language` (same as Stage 1)
- Keyword parsing: split response by newlines, trim whitespace and bullet markers, discard empty lines
- **Thinking model compatibility**: Some models (e.g., Qwen3.5) wrap responses in `<think>...</think>` tags. All LLM backend responses are passed through `StripThinkingTags()` which removes `<think>`, `<thinking>`, and `<reflection>` XML blocks. The `ollama.think` config controls whether thinking mode is enabled (default: false). When enabled, `num_predict` is automatically multiplied by 4x to accommodate both thinking and response tokens.

### Built-in Prompt Templates

The tool ships with built-in prompt templates in three languages (en, zh-Hant, ja), selected via `summary.language`. Users can override with `summary.prompt` (inline) or `summary.summary_prompt_file` (external file).

Prompt resolution order:
1. Per-channel `summary_prompt_file` (if set)
2. Global `summary.summary_prompt_file` (if set)
3. Global `summary.prompt` (inline, if set)
4. Built-in prompt for `summary.language` (default)

#### Stage 1 Prompt — Summary (zh-Hant)

File: `prompts/builtin/summary-zh-Hant.md`

```markdown
你是一位專業的影片內容分析師。請根據以下影片資訊與字幕轉錄內容，產生結構化的摘要。

## 影片資訊
- 標題：{{title}}
- 頻道：{{channel_name}}
- 日期：{{upload_date}}
- 時長：{{duration}}
- 標籤：{{tags}}
- 轉錄字數：{{transcription_length}} 字

## 輸出規模指引

本影片轉錄共 {{transcription_length}} 字。請根據內容豐富程度自行決定各段落的數量和細節，但不得低於以下最低要求：

| 轉錄字數 | 概述最少 | 章節最少 | 每章要點最少 | 重點最少 |
|---------|---------|---------|---------------|---------|
| < 500 字 | 2 句 | 2 章 | 2 項 | 3 個 |
| 500-3,000 字 | 4 句 | 3 章 | 2 項 | 5 個 |
| 3,000-10,000 字 | 4 句 | 4 章 | 3 項 | 7 個 |
| > 10,000 字 | 6 句 | 5 章 | 3 項 | 10 個 |

本影片屬於「{{transcription_tier}}」級別。以上為最低要求，如內容豐富請自行增加。

## 輸出格式

請嚴格按照以下格式輸出，不要省略任何段落。

### 概述
用上述最低句數以上簡要說明這部影片的主題、目標觀眾、以及核心結論或主張。

### 章節摘要
將影片內容按主題轉折分章節。每個章節格式如下：

#### [章節標題]
按照敘述順序或邏輯先後，以條列方式整理該章節的要點與事實細節。
每個要點應包含足夠的上下文，讓讀者不需要觀看影片也能理解前後關係。
可以使用階層列表來表示從屬或因果關係。

如果影片內容是線性敘述（如教學），按時間順序分章節。
如果影片內容是多主題（如新聞彙整），按主題分章節。

### 重點整理
用階層列表整理影片中最重要的要點：
- 以主題或類別分組，每組用粗體標題
  - 底下列出該主題的關鍵事實或結論
- 優先列出具有實用價值或新穎性的資訊
- 如果影片包含行動建議（action items），獨立列為一組

## 注意事項
- 遇到專有名詞、技術名詞、人名、產品名稱時，保留原文並在翻譯旁以括號標注
  - 例如：大型語言模型（LLM）、注意力機制（Attention Mechanism）
  - 人名保留原文：伊隆·馬斯克（Elon Musk）
- 忠實反映影片內容，不加入推測、評論或額外資訊
- 如果字幕中有明顯的辨識錯誤（如同音異字），請根據上下文修正
- 使用繁體中文撰寫，語氣保持客觀中立

## 影片字幕轉錄內容
{{transcript}}
```

#### Stage 1 Prompt — Summary (en)

File: `prompts/builtin/summary-en.md`

```markdown
You are a professional video content analyst. Based on the video information and transcription below, produce a structured summary.

## Video Information
- Title: {{title}}
- Channel: {{channel_name}}
- Date: {{upload_date}}
- Duration: {{duration}}
- Tags: {{tags}}
- Transcription length: {{transcription_length}} characters

## Output Scale Guide

This video transcription contains {{transcription_length}} characters. Adjust the detail level based on content richness, but meet these minimum requirements:

| Transcription length | Min overview | Min sections | Min points per section | Min key points |
|---------------------|-------------|-------------|----------------------|---------------|
| < 1,000 chars | 2 sentences | 2 sections | 2 items | 3 points |
| 1,000-5,000 chars | 4 sentences | 3 sections | 2 items | 5 points |
| 5,000-15,000 chars | 4 sentences | 4 sections | 3 items | 7 points |
| > 15,000 chars | 6 sentences | 5 sections | 3 items | 10 points |

This video falls in the "{{transcription_tier}}" tier. These are minimums — increase if content warrants it.

## Output Format

Follow this format strictly. Do not skip any section.

### Overview
Briefly describe the video's topic, target audience, and core conclusion or thesis.

### Section Summary
Divide the content into sections by topic shift. Each section:

#### [Section Title]
List the key points and factual details in narrative or logical order.
Each point should include enough context for a reader to understand the progression without watching the video.
Use nested lists to express subordination or causal relationships.

For linear content (e.g., tutorials), use chronological sections.
For multi-topic content (e.g., news roundups), use thematic sections.

### Key Takeaways
Organize the most important points using hierarchical lists:
- Group by theme or category, each group with a **bold heading**
  - List key facts or conclusions under each theme
- Prioritize actionable or novel information
- If the video contains action items, list them as a separate group

## Guidelines
- Preserve technical terms, proper nouns, product names, and person names in their original language
- Faithfully reflect video content — do not add speculation, commentary, or extra information
- Correct obvious transcription errors (e.g., homophones) based on context
- Write in English with an objective, neutral tone

## Video Transcription
{{transcript}}
```

#### Stage 1 Prompt — Summary (ja)

File: `prompts/builtin/summary-ja.md`

```markdown
あなたはプロの動画コンテンツアナリストです。以下の動画情報と字幕書き起こし内容に基づき、構造化された要約を作成してください。

## 動画情報
- タイトル：{{title}}
- チャンネル：{{channel_name}}
- 日付：{{upload_date}}
- 時間：{{duration}}
- タグ：{{tags}}
- 書き起こし文字数：{{transcription_length}} 文字

## 出力規模ガイド

本動画の書き起こしは {{transcription_length}} 文字です。内容の豊富さに応じて各セクションの量と詳細度を自由に調整してください。ただし、以下の最低要件を満たすこと：

| 書き起こし文字数 | 概要最低 | セクション最低 | 各セクション要点最低 | 要点最低 |
|---------------|---------|-------------|------------------|---------|
| < 500 文字 | 2 文 | 2 セクション | 2 項目 | 3 個 |
| 500-3,000 文字 | 4 文 | 3 セクション | 2 項目 | 5 個 |
| 3,000-10,000 文字 | 4 文 | 4 セクション | 3 項目 | 7 個 |
| > 10,000 文字 | 6 文 | 5 セクション | 3 項目 | 10 個 |

本動画は「{{transcription_tier}}」レベルです。上記は最低要件であり、内容が豊富な場合は増やしてください。

## 出力フォーマット

以下のフォーマットに厳密に従ってください。セクションを省略しないでください。

### 概要
動画のテーマ、対象視聴者、核心的な結論または主張を簡潔に説明してください。

### セクション要約
内容をテーマの転換点でセクションに分けてください。各セクションの形式：

#### [セクションタイトル]
叙述順序または論理的順序に従い、箇条書きで要点と事実の詳細を整理してください。
各要点には十分な文脈を含め、動画を視聴しなくても前後関係が理解できるようにしてください。
階層リストを使用して、従属関係や因果関係を表現できます。

線形的な内容（チュートリアルなど）は時系列で分割。
複数トピック（ニュースまとめなど）はテーマ別に分割。

### 重要ポイント
階層リストを使用して、動画の最も重要なポイントを整理：
- テーマまたはカテゴリでグループ化し、各グループに**太字の見出し**を付ける
  - 各テーマの重要な事実や結論を下に記載
- 実用的価値や新規性のある情報を優先
- アクションアイテムがあれば独立したグループとしてリストアップ

## 注意事項
- 専門用語、固有名詞、人名、製品名は原語を保持し、翻訳の横に括弧で表記
  - 例：大規模言語モデル（LLM）、アテンション機構（Attention Mechanism）
  - 人名は原語保持：イーロン・マスク（Elon Musk）
- 動画内容を忠実に反映し、推測・解説・追加情報を加えない
- 字幕の明らかな認識エラー（同音異字など）は文脈に基づき修正
- 日本語で記述し、客観的で中立的なトーンを維持

## 動画字幕書き起こし内容
{{transcript}}
```

#### Stage 2 Prompt — Keywords (auto-generated)

Not customizable. The program generates the prompt based on `keywords.language` and `keywords.count`:

- `zh-Hant`: `"請從以下摘要中提取最多 {count} 個關鍵字，每行列出一個關鍵字，不要編號，不要其他說明文字。使用繁體中文，遇到專有名詞保留原文。\n\n{summary}"`
- `en`: `"Extract up to {count} keywords from the summary below. List one keyword per line. No numbering, no extra text.\n\n{summary}"`
- `ja`: `"以下の要約から最大 {count} 個のキーワードを抽出してください。1行に1つのキーワードを記載し、番号や説明は不要です。専門用語は原語を保持してください。\n\n{summary}"`

#### Stage 3 Prompt — Mermaid Flowchart (auto-generated)

Not customizable. The program generates the prompt:

- `zh-Hant`: `"請根據以下影片摘要，用 Mermaid 流程圖呈現影片的敘事邏輯或核心概念的關係。\n\n規則：\n- 使用 graph TD（上到下）格式\n- 節點文字用雙引號包裹，例如：A[\"節點文字\"]\n- 只用簡單箭頭 -->\n- 節點數量控制在 5-12 個\n- 只輸出 Mermaid 語法區塊，不要其他說明文字\n\n{summary}"`
- `en`: `"Based on the video summary below, create a Mermaid flowchart showing the narrative logic or relationships between core concepts.\n\nRules:\n- Use graph TD (top-down) format\n- Wrap node text in double quotes, e.g.: A[\"Node text\"]\n- Use only simple arrows -->\n- Keep nodes between 5-12\n- Output only the Mermaid code block, no other text\n\n{summary}"`
- `ja`: `"以下の動画要約に基づき、Mermaid フローチャートで動画の論理構成または核心概念の関係を表現してください。\n\nルール：\n- graph TD（上から下）形式を使用\n- ノードテキストはダブルクォートで囲む。例：A[\"ノードテキスト\"]\n- 矢印は --> のみ使用\n- ノード数は 5-12 個に制限\n- Mermaid コードブロックのみ出力、説明文不要\n\n{summary}"`

### Whisper Transcription Branch

1. Download audio via `yt-dlp` in WAV 16kHz format (`-x --audio-format wav --postprocessor-args "-ar 16000"`) — whisper.cpp requires this format
2. Select whisper model: `language_models[lang]` → `default_model` fallback. Language codes are normalized to ISO 639-1 prefix for lookup (e.g., `zh-Hant` → `zh`)
3. Auto-download model if not present (using `model_sources` URLs)
4. Transcribe with `whisper.cpp`. whisper-cli is invoked with `-l <lang>` when the language is known, or `-l auto` when unknown, for better transcription accuracy.
5. Delete audio file after transcription, keep only subtitle output

### Language Detection (4-tier Fallback)

When the video language needs to be determined (for whisper model selection or when `preferred_languages` is not set):

1. **yt-dlp `language` field** — from `--dump-json` metadata. Not always set by uploaders.
2. **Subtitle language** — (reserved) from the first available subtitle.
3. **Title + description text analysis** — Unicode character range detection:
   - CJK Ideographs (U+4E00-U+9FFF) > 30% → `zh`
   - Hiragana (U+3040-U+309F) or Katakana (U+30A0-U+30FF) > 10% → `ja`
   - Hangul (U+AC00-U+D7AF) > 30% → `ko`
4. **Unknown** → whisper auto-detect (`-l auto` flag)

This fallback is applied in `pipeline.resolveVideoLanguage()` before subtitle download and whisper transcription.

### Subtitle Download Strategy (Detailed)

Uses yt-dlp's `--sub-lang` with comma-separated priority list and a cascading download approach:

```
Step 1: Try manual subtitles in target language(s)
        yt-dlp --write-subs --sub-lang "ja,zh-Hant,en" --skip-download --convert-subs srt
        ├─ Found → done
        └─ Not found → Step 2

Step 2: Try auto-generated subtitles in target language(s)
        yt-dlp --write-auto-subs --sub-lang "ja,zh-Hant,en" --skip-download --convert-subs srt
        ├─ Found → done (mark subtitle_type: auto)
        └─ Not found → Step 3

Step 3: Try manual subtitles (any language, yt-dlp fallback)
        yt-dlp --write-subs --skip-download --convert-subs srt
        ├─ Found → done
        └─ Not found → Step 4

Step 4: Try auto-generated subtitles (any language, yt-dlp fallback)
        yt-dlp --write-auto-subs --skip-download --convert-subs srt
        ├─ Found → done (mark subtitle_type: auto)
        └─ Not found → whisper transcription branch
```

Language codes are passed directly from config/detection to yt-dlp without normalization. Normalization is only done at the **whisper model lookup boundary** using the function described below.

### Language Code Normalization

A `normalizeToISO639_1(code)` function is used **only** for whisper model lookup. It is NOT used for yt-dlp operations.

**Rules (applied in order):**
1. Convert to lowercase
2. Apply special mappings (ISO 639-3 / bibliographic → ISO 639-1):

| Input | Output | Note |
|-------|--------|------|
| `cmn`, `yue`, `wuu` | `zh` | Chinese macro-language variants |
| `jpn` | `ja` | Japanese ISO 639-3 |
| `kor` | `ko` | Korean ISO 639-3 |
| `eng` | `en` | English ISO 639-3 |
| `fra`, `fre` | `fr` | French (terminologic / bibliographic) |
| `deu`, `ger` | `de` | German (terminologic / bibliographic) |
| `spa` | `es` | Spanish ISO 639-3 |
| `por` | `pt` | Portuguese ISO 639-3 |
| `rus` | `ru` | Russian ISO 639-3 |

3. If not in special mappings and length > 2: take the first two characters
   - `zh-Hant` → `zh`, `zh-Hans` → `zh`, `zh-TW` → `zh`
   - `ja-JP` → `ja`
   - `en-US` → `en`, `en-orig` → `en`, `en-uYU-mmqFLq8` → `en`
   - `ko-KR` → `ko`

4. Result is used to lookup `whisper.language_models[result]`

**Important yt-dlp note:** YouTube does not recognize bare `zh` for subtitle downloads. Always use `zh-Hant`, `zh-Hans`, or the regex pattern `zh.*` in `preferred_languages` and `--sub-lang`.

### Cookie Usage Strategy

Cookies are used **only when needed** to minimize account risk:

```
Fetch metadata (yt-dlp --dump-json, no cookie)
├─ Success → check availability field
│   ├─ availability = public / unlisted → proceed without cookie
│   └─ availability = members_only / needs_auth / premium_only / private
│       ├─ Cookie configured? → use cookie for all subsequent downloads
│       └─ No cookie? → log warning "cookie required", skip
└─ Fail with "sign in required"
   ├─ Cookie configured? → retry metadata with cookie (once)
   │   ├─ Success → use cookie for all subsequent downloads
   │   └─ Fail → log error, skip
   └─ No cookie? → log warning "cookie required", skip
```

**Cookie priority (per request):**
1. **Per-source cookie** (playlist/channel level `cookie` config) — if set, use directly without attempting no-cookie first
2. **No-cookie attempt** — if no per-source cookie is configured, try without cookie
3. **Global cookie retry** — if step 2 fails and global `cookie` is configured, retry with global cookie
4. **Error** — log warning and skip the video/playlist

Key improvements:
- **Pre-detect restricted videos** via `availability` metadata field before attempting download
- **Chrome multi-profile support**: When `browser: chrome`, try profiles in order: `chrome_profile` (if set) → `Default` → `Profile 1`, `Profile 2`, etc.
- Maps to `yt-dlp --cookies` / `--cookies-from-browser "chrome:Profile 1"` flags
- Usage is logged at WARN level

## Internal Architecture

```
ytss/
├── main.go                  # CLI entry (cobra)
├── cmd/
│   ├── run.go               # ytss run
│   ├── video.go             # ytss video
│   └── channel.go           # ytss channel
├── config/
│   └── config.go            # YAML config parsing
├── fetcher/
│   └── fetcher.go           # yt-dlp: channel video list & metadata. VideoMeta includes Description field (mapped to yt-dlp's `description` JSON field), used for language detection
├── subtitle/
│   └── subtitle.go          # Subtitle download, language strategy, SRT → plain text
├── transcriber/
│   └── transcriber.go       # Audio download + whisper.cpp transcription + model mgmt
├── summarizer/
│   ├── summarizer.go        # LLM interface definition
│   ├── ollama.go
│   ├── llamacpp.go
│   ├── claude.go
│   └── gemini.go
├── pipeline/
│   └── pipeline.go          # Orchestrates all modules
├── output/
│   └── output.go            # Naming rules, directory creation, skip detection
├── embedded/
│   └── embed.go             # go:embed yt-dlp & whisper.cpp, extract to cache
└── config.example.yaml
```

### Key Design Decisions

- **`summarizer` uses an interface** — all LLM backends implement `Summarize(text string, opts SummarizeOptions) (string, error)` where `SummarizeOptions` includes prompt template, max_tokens, and model name. The pipeline is responsible for assembling the final prompt (template + transcription) and orchestrating the three-stage LLM call sequence. CLI-based backends (gemini-cli) receive input via stdin pipe to avoid OS argument length limits
- **`embedded/` handles binary extraction** — checks `~/.ytss/bin/` at startup, extracts from embed if missing or version mismatch. When invoking `yt-dlp`, always pass `--ffmpeg-location <cache_dir>` to use the bundled ffmpeg
- **`pipeline/` is the single orchestration point** — all three commands call into pipeline, differing only in input source

## External Dependencies

### Embedded Binaries

| Tool | Source | Embed Strategy |
|------|--------|---------------|
| `yt-dlp` | GitHub Release (platform-specific binary) | `go:embed` |
| `ffmpeg` | GitHub Release (static build, e.g., [ffmpeg-static](https://github.com/eugeneware/ffmpeg-static) or [BtbN builds](https://github.com/BtbN/FFmpeg-Builds)) | `go:embed` |
| `whisper.cpp` | GitHub Release, `whisper-cli` binary (pin specific release tag) | `go:embed` |

### Build Process

```
Makefile
├── download-deps GOOS=x GOARCH=y  # Download platform-specific binaries to embedded/bin/{os}-{arch}/
├── build                          # go build for current platform
├── build-all                      # Sequential: for each target, download-deps then build
└── clean
```

Directory layout for embedded binaries:
```
embedded/bin/
├── darwin-arm64/
│   ├── yt-dlp
│   ├── ffmpeg
│   └── whisper-cli
├── darwin-amd64/
│   ├── yt-dlp
│   ├── ffmpeg
│   └── whisper-cli
└── linux-amd64/
    ├── yt-dlp
    ├── ffmpeg
    └── whisper-cli
```

- `build-all` runs sequentially: download target deps → build target → next target
- `embedded/bin/` is in `.gitignore`; CI downloads correct versions per platform
- Build tags or conditional embed paths select the correct platform binary

### Go Dependencies

| Purpose | Library |
|---------|---------|
| CLI framework | `github.com/spf13/cobra` |
| YAML parsing | `gopkg.in/yaml.v3` |
| HTTP client (LLM API) | stdlib `net/http` |
| Logging | stdlib `log/slog` |

## Error Handling & Logging

### Processing Model

Videos are processed **sequentially, one at a time**. Whisper transcription is CPU/GPU-intensive and LLM calls can be resource-heavy; concurrent processing is out of scope for the initial version.

**Batch settings** (`batch` config section):
- `random_order: true` — shuffles channel processing order each run to avoid predictable patterns and ensure fair processing
- `delay_min` / `delay_max` — random delay (in seconds) between channels to reduce request frequency. Delay is `rand(min, max)` seconds, applied after each channel except the last.

**Processing order in `ytss run`:**
1. Playlists (in order, or shuffled if `batch.random_order`)
2. Channels (in order, or shuffled if `batch.random_order`)

Playlists and channels are shuffled independently within their groups.

### Timeouts

| Operation | Default Timeout |
|-----------|----------------|
| `yt-dlp` metadata/subtitle fetch | 60s |
| `yt-dlp` audio download | 10min |
| `whisper.cpp` transcription | 30min |
| LLM summarization call | Configurable via `ollama.timeout` (default: 15min) |

### Error Strategy

- **Single video failure does not abort batch** — log error, continue to next
- **External tool failure** — capture stderr, mark video as failed
- **LLM unavailable** — produce subtitle and transcription normally, skip summary only, log warning
- **Network issues** — on model download failure, suggest manual download path

### Log Format

Structured logging via `slog`, default info level, `-v` for debug:

```
INFO  processing channel @channel-a (5 videos)
INFO  [1/5] dQw4w9WgXcQ - skipped (already exists)
INFO  [2/5] abc123xyz - downloading subtitle (ja, manual)
INFO  [2/5] abc123xyz - generating summary (ollama/llama3)
INFO  [2/5] abc123xyz - done
WARN  [3/5] def456uvw - no subtitle available, transcribing with whisper (medium)
WARN  [3/5] def456uvw - cookie required for download, retrying with cookie
ERROR [4/5] ghi789rst - failed: yt-dlp exit code 1: video unavailable
INFO  [5/5] jkl012mno - done
INFO  completed: 3 success, 1 skipped, 1 failed
```

### Completion Summary

Batch runs print statistics at the end: success / skipped / failed counts, with video IDs and reasons for failures.
