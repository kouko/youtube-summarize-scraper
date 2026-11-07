# 文章式摘要：觀點章節優先，正文依原始敘事順序保留細節 — acceptance test evidence

Tried on 2026-11-07, in a clean copy of the project at `b4a03731f6d3e092c040285c0164ed976d2428b9`
(re-test after the C-mode length-anchor fix `a5f616f..b4a0373`; this file replaces the a5f616f run).

## 0. Environment and setup
- Fresh `git worktree add /private/var/folders/pv/m7y9m2v56lg68nl8wtxkbm0c0000gn/T/opencode/at-b4a0373 b4a0373`
  → detached HEAD `b4a03731f6d3e092c040285c0164ed976d2428b9`, clean tree.
- `embedded/bin/` is gitignored, so a bare build in the fresh copy cannot embed the tools
  (`make deps` provisions them). Per the project's own build instructions I copied the prebuilt
  darwin-arm64 binaries (yt-dlp / ffmpeg / whisper-cli) from the main checkout into the worktree's
  `embedded/bin/darwin-arm64/`. Build tooling, not part of the change.
  `go build ./...` → BUILD_OK.
- `go test ./...` (run per the station's instruction; the full-suite gate still executes at
  finalize-review) → every package `ok`, exit 0. Log:
  `/private/var/folders/pv/m7y9m2v56lg68nl8wtxkbm0c0000gn/T/opencode/at-b4a0373-run/go-test.log`
  (tail: `ok .../cmd`, `ok .../config`, `ok .../pipeline`, `ok .../summarizer`, `ok .../tui`, …).
- Fix scope checked (`git show b4a0373 --stat`): exactly three files,
  `prompts/builtin/summary-article-{zh-Hant,en,ja}.md`, 6 insertions / 3 deletions — the hard
  length line (target 50%, floor 45%, per-chapter full-coverage rule) and one self-check item.
  No code change.
- `config.yaml` is not versioned; I copied the user's real config into the worktree
  (`style: "article"`, `language: "zh-Hant"`, output redirected via `-o`).
- **Environment deviation (recorded as-is):** with the copied config (`cookie.browser: "chrome"`)
  the run failed twice at the fetch stage:
  `ERROR: unable to download video data: HTTP Error 403: Forbidden` (subtitle step found no
  subtitles, then whisper's audio download 403'd). A direct probe separated cause from change:
  ```
  yt-dlp -f 251 --cookies-from-browser chrome ...   → HTTP Error 403: Forbidden
  yt-dlp -f 251 (no cookies) ...                   → downloaded 15.60 MiB successfully
  ```
  The extracted Chrome cookies are being rejected by YouTube; this is environment state, not the
  change (same binary, version 2026.10.26, extracted from this build; earlier rounds today worked
  with the same setup). I therefore cleared `cookie.browser` **in the worktree's config copy only**
  (`browser: ""`) and re-ran. The user's real `/Users/kouko/youtube-summarize-scraper/config.yaml`
  was not touched. With cookies the pipeline fails → that is a live environment issue the user may
  also hit in the daily batch.
- Vehicle run (final, successful attempt):
  ```
  go run . video w8GM1mghKJc --llm claude-code \
    -o /private/var/folders/pv/m7y9m2v56lg68nl8wtxkbm0c0000gn/T/opencode/at-b4a0373-run/output --force
  ```
  Exit code 0, final log line: `video w8GM1mghKJc: completed successfully`
  (`…/at-b4a0373-run/run4.log`; failed attempts: `run.log`, `run2.log`, `run3.log`).
  - Video length 19:26 (user-accepted deviation from "20+ minutes").
  - No manual subtitles available → whisper fallback (`subtitle_type: "whisper"`, model `belle-zh`),
    same as the previous round.
  - Stage 1: `provider=claude-code model=haiku response_length=5220`.
- Produced file:
  `…/output/@老李玩钱/2026-11-05__w8GM1mghKJc__中期选举尘埃落定美股分化严重何去何从/2026-11-05__w8GM1mghKJc__summary.md`
  (142 lines; SHA-256 `8f370d81ae3530b629795954065a852c18d41c2c220be0dd8b65413468bebf4f`).

## 1. Body begins immediately after frontmatter with `## 主要觀點` (H2) distilling core positions, not a segment recap
- How I tried it: `grep -n '^#' <summary.md>`; read lines 29–36.
- What came back: frontmatter ends at line 29 (`---`), line 30 is the tool-inserted video-embed
  line (`![](https://www.youtube.com/watch?v=w8GM1mghKJc)`), line 32 is `## 主要觀點` — the first
  section of the body. Line 34 states core positions (post-election divergence is the normal
  pre-election-risk / post-election-relief pattern; rotation, not systemic weakness; diversify and
  keep balance), not a segment recap. (Thinner than the previous round — one paragraph instead of
  two — but it distills positions, which is what the line asks.)
- Evidence: heading list in section 9; file lines 29–36.

## 2. Continuous article in `###` chapters in original narrative order; concrete facts retained
- How I tried it: `grep -nE '^### ' <summary.md>`; read the whole file; compared chapter order and
  every concrete number/name against `transcription.md`.
- What came back:
  ```
  52:### 選舉結果與市場反應
  77:### 科技板塊的分化與輪動
  104:### 投資組合的分散與平衡
  131:### 特斯拉與SpaceX的操作點位
  140:### 結語
  ```
  5 chapters follow the transcript's narrative order (election result + oil/inflation → tech split →
  portfolio + semis thesis → TSLA/SpaceX entries → closing). Retained facts: 八個百分點（WTI 單日）、
  九十美元（WTI）、接近三個百分點（道瓊）、27%（德州數據中心電力）、173/180 股與 199 美元（英偉達）、
  20/50 股與 315 美元（博通）、368/352 美元（特斯拉）、135/120 美元（SpaceX）、十二個月（選後規律）、
  三月／四月（上半年走勢）；names: 微軟、亞馬遜、谷歌、英偉達、AMD、博通、吉娜、川普、馬斯克、特斯拉、SpaceX。
  **Dropped concrete facts** (present in the transcript, absent from the article):
  `三千九百多美金`（當日帳戶上漲）, `三十二万六千多`（帳戶總資產）, `苹果`（大漲的科技巨頭之一）.
  The previous round's article retained the two account amounts; the new per-chapter
  full-coverage rule was not obeyed for the opening segment.
- Evidence: heading list above; inline python token comparison in this section's method; read of
  `transcription.md` lines 23–30 and 59–63 against summary lines 54–115.

## 3. Article completes without truncation; single-video run exits 0
- How I tried it: read the last lines; checked the run exit code and log; grepped for truncation
  markers (`…`, `...`, `回應|未完|to be continued` → 0 hits).
- What came back: file ends at line 142 with a complete sentence
  （「影片最後表示，以上內容是個人對市場的觀察與思考，並提醒觀眾，若有意開立美股帳戶，影片說明欄提供相關開戶連結。」）;
  no truncation marker, no mid-sentence cut. `EXIT=0`; log ends `video w8GM1mghKJc: completed successfully`.
  Recorded deviations: video is 19:26 (user-accepted); whisper fallback this round.
- Evidence: run4.log tail; last 3 lines of the summary file.

## 4. No pre-existing summary file modified (SHA-256 snapshot before/after)
- How I tried it: before and after the run, hashed every `.md` file under
  `/Users/kouko/youtube-summarize-scraper/ytss-output`:
  ```
  find ytss-output -name '*.md' -type f -print0 | sort -z | xargs -0 shasum -a 256 > .../ytss-output-before-b4a0373.sha256
  find ytss-output -name '*.md' -type f -print0 | sort -z | xargs -0 shasum -a 256 > .../ytss-output-after-b4a0373.sha256
  ```
- What came back: 25,486 files hashed each time; `diff before after` → empty
  (`IDENTICAL: all summary files unchanged`). The run wrote only into the isolated temp output dir
  under `/private/var/folders/pv/m7y9m2v56lg68nl8wtxkbm0c0000gn/T/opencode/`.
- Evidence: both hash files and the empty diff.

## 5. C-mode: article prose 40–60% of transcription length; only spoken filler compressed
- How I tried it: python measurement over `summary.md` and `transcription.md` (same method as the
  previous report: summary prose = body after frontmatter minus ```mermaid blocks, counting non-blank
  line runes; transcript body = transcription.md minus frontmatter minus the video-embed line;
  the pipeline injects `len([]rune(transcriptText))` at `pipeline/pipeline.go:834`).
- What came back:
  ```
  transcript body runes: 5480
  summary prose, mermaid stripped, incl. embed line: 1856 runes  = 33.9%
    excl. embed line: 1808 runes  = 33.0%
    excl. embed + headings: 1735 runes  = 31.7%
  0.4x = 2192 ; 0.45x = 2466 ; 0.5x = 2740 ; 0.6x = 3288
  ```
  The article (1,856 runes even counting the tool-inserted embed line) is below the acceptance
  floor (≥ 2,192) and far below the template's own new 45% floor (≥ 2,466) and 50% target (2,740).
  Previous round: 1,857/5,481 = 33.9%. The fix changed the output length by **one rune** — the
  prompt anchor did not move the result.
- Evidence: script output above.

## 6. No meta-narration (no attribution to source medium/narrator; no reporting verbs)
- How I tried it (grep broadly, then read every hit in context):
  ```
  grep -n '影片\|本集\|頻道\|講者\|作者\|指出\|表示\|強調\|認為\|提到\|引用\|他\|她' <summary.md>
  grep -niE 'the video|this episode|the channel|the author|the speaker|\bhe\b|\bshe\b|notes|states|emphasizes|believes|mentions|cites' <summary.md>
  ```
- What came back: the English scan is clean; the Chinese scan returns the disclaimer lines (allowed)
  plus **three prose violations**:
  ```
   36: 以下為影片中的個人操作計畫，非投資建議。                       ← disclaimer exception (allowed)
  106: 影片中提醒投資人，應以足球隊的理念打造投資組合…                 ← VIOLATION (影片 + 提醒)
  108: 半導體晶片與存儲板塊長期的投資邏輯，影片認為並未改變…           ← VIOLATION (影片 + 認為)
  112: 以下為影片中的個人操作計畫，非投資建議。                       ← disclaimer exception (allowed)
  135: 以下為影片中的個人操作計畫，非投資建議。                       ← disclaimer exception (allowed)
  142: 影片最後表示，以上內容是個人對市場的觀察與思考…                 ← VIOLATION (影片 + 表示)
  ```
  Violation count: **3** (all clear, none borderline; the accepted residual is ≤1 borderline).
  Previous round was 0 — this round it came back. A mermaid node label at line 120 reads
  `輪到他板塊時損失嚴重` (「他板塊」 is a garbled 「其他板塊」); that is diagram label text, not a
  prose sentence, so it is not counted, but it is recorded as a nit.
- Evidence: grep outputs above; full file read (142 lines) for context.

## 7. `## 主要觀點` both distills core positions AND directly answers the title's question (additive)
- How I tried it: read lines 32–36; title is 「中期选举尘埃落定！美股分化严重，何去何从」.
- What came back: line 34 answers directly:
  「至於標題所問的『何去何從』，答案是板塊正在輪動，半導體與存儲股過去幾個月拉動大盤，現在輪到
  傳統周期、金融、消費與防禦型股票接棒。對於重倉單一板塊的投資人而言，最重要的是回到投資組合的
  分散與平衡，而非追逐單一板塊的漲勢。」 The same paragraph distils the core positions (divergence is
  the typical election pattern; rotation is normal; diversify), so both halves of the criterion hold.
- Evidence: file lines 32–36.

## 8. Dynamic mermaid caps: transcription 5,480 → tier <10000 → 3–4 diagrams; overview position
- How I tried it: `grep -n '```mermaid' <summary.md>`; `grep -c '^#### '`; transcription length from
  section 5; tier code `summarizer/summarizer.go:255-268` (`<10000 → total 4`, additional range 2-3).
- What came back:
  ```
  38:```mermaid      ← overview, directly under the 主要觀點 prose
  64:```mermaid      ← chapter 1 (選舉結果與市場反應)
  91:```mermaid      ← chapter 2 (科技板塊的分化與輪動)
  118:```mermaid     ← chapter 3 (投資組合的分散與平衡)
  ```
  4 diagrams total = at the cap (tier 3–4), within it. The first diagram (line 38) sits directly
  under the `## 主要觀點` prose (line 34) and before the first `###` chapter (line 52), with no
  `####` heading (`^#### ` count = 0). The remaining 3 hang on 3 of the 5 chapters — not one per
  chapter.
- Evidence: grep outputs above; file lines 34–52.

## 9. No `### 重點整理`/`### 概述`, no duplicated heading, body entirely Traditional Chinese
- How I tried it:
  ```
  grep -c '### 重點整理\|### 概述' <summary.md>   → 0
  grep -c '^## 主要觀點' <summary.md>             → 1 (no duplicate)
  grep -c '^#### ' <summary.md>                   → 0
  ```
  Simplified-only character scan over the body (after frontmatter, embed line excluded) using the
  same shared/simplified-only set as before: hits `{'用': 5, '等': 3}` — both are characters shared
  by Traditional and Simplified, so no simplified-only character occurs. The only simplified text
  in the file is the frontmatter `title` (the creator's original title), which is source metadata,
  explicitly allowed.
- Evidence: greps above; scan output.

## REQ-9 (static). en/ja/zh-Hant templates carry the length anchor, title answering, no-attribution rule, tiered diagram table; `{{mermaid_limit}}` present; MermaidLimitText language-aware
- How I tried it: read the three `prompts/builtin/summary-article-{zh-Hant,en,ja}.md` at b4a0373;
  checked `prompts/builtin/mermaid-*.md` and `summarizer/summarizer.go:255-318`.
- What came back:
  - All three article templates now carry the hard length line at line 23 (zh-Hant/ja: 50% target,
    45% floor, per-chapter full coverage; en: same in English) and the length self-check at
    line 63/65. Title-answering rule and the direct-statement (no-attribution) rule remain in all
    three; the tiered diagram table remains (<500 → 1–2, <3000 → 2–3, <10000 → 3–4, ≥10000 → 4–5).
  - `{{mermaid_limit}}` present in `mermaid-en.md`, `mermaid-ja.md`, `mermaid-zh-Hant.md`;
    substituted by `MermaidPrompt` (`summarizer/mermaid.go`).
  - `MermaidLimitText` has `case "zh"` (summarizer.go:293), `case "ja"` (:302) and a default English
    branch (:311); tier logic at :255-268 matches the tables.
- Evidence: grep outputs above; `git show b4a0373 -- prompts/` diff (length lines only).

## UI flow (spec): run single video → summary file appears; viewpoint-first format, bounded diagrams, keywords unchanged
- How I tried it: the vehicle run itself (`go run . video w8GM1mghKJc --llm claude-code -o <temp> --force`).
- What came back: file created under `<temp>/@老李玩钱/2026-11-05__w8GM1mghKJc__…/`;
  frontmatter first (tags list present, `subtitle_type: "whisper"`, duration 19:26), then
  `## 主要觀點`, then `###` chapters, 4 mermaid diagrams; run completed exit 0. The flow itself works;
  its only shortfalls are the #5 length and #6 meta-narration content rows.
- Evidence: run4.log; produced file.

## Re-run on 2026-11-07, at b4a0373 (after the length-anchor fix a5f616f..b4a0373)
- All rows re-tested in full: the fix changes the article templates, and a re-run regenerates the
  entire article, so no row could be carried over safely. The fix diff touches only the length
  guidance in the three article templates (checked line by line against `git show b4a0373`).
- #5 (length): unchanged in effect — 33.9% (1,856/5,480) vs the previous 33.9% (1,857/5,480);
  still below the 40% acceptance floor.
- #6 (meta-narration): regressed — 0 violations last round, 3 this round (lines 106, 108, 142).
- #2: now also drops two transcript amounts (3,900+ USD daily gain, 326,000 USD total assets) and
  Apple, which the previous round retained.
- #4 snapshot re-run: 25,486 files identical before/after.
