# 文章式摘要：觀點章節優先，正文依原始敘事順序保留細節 — acceptance test evidence

Tried on 2026-11-07, in a clean copy of the project at `a5f616f1e114c38457c1fa55f39de34de04d0c2d`
(re-test after the prompt-reframe commit `607448c..a5f616f`; the superseded 607448c run is
replaced by this file).

## 0. Environment and setup
- Fresh `git worktree add /private/var/folders/pv/m7y9m2v56lg68nl8wtxkbm0c0000gn/T/opencode/at-a5f616f a5f616f` → detached HEAD `a5f616f1e114c38457c1fa55f39de34de04d0c2d`, clean tree.
- `embedded/bin/` is gitignored, so a bare `go build ./...` in the fresh copy fails:
  `embedded/embedded.go:14:12: pattern bin: cannot embed directory bin: contains no embeddable files`.
  Per the project's own build instructions (`make deps` provisions `embedded/bin/<platform>/`),
  I provisioned the worktree by copying the prebuilt binaries already present in the main
  checkout (yt-dlp / ffmpeg / whisper-cli, darwin-arm64). This is build tooling, not part of
  the change. After that: `go build ./...` → BUILD_OK.
- `go test ./...` (run per the station's instruction; the full-suite gate still executes at
  finalize-review) → all packages `ok`, exit 0. Log tail:
  ```
  ok  	github.com/kouko/youtube-summarize-scraper/cmd	2.568s
  ok  	github.com/kouko/youtube-summarize-scraper/config	0.955s
  ...
  ok  	github.com/kouko/youtube-summarize-scraper/tui	7.376s
  ```
  (Full output: `/private/var/folders/pv/m7y9m2v56lg68nl8wtxkbm0c0000gn/T/opencode/go-test-a5f616f.log`)
- `config.yaml` is not versioned; I copied the user's real config into the worktree
  (`style: "article"`, `language: "zh-Hant"`, output redirected via `-o`).
- Vehicle run:
  ```
  go run . video w8GM1mghKJc --llm claude-code \
    -o /private/var/folders/pv/m7y9m2v56lg68nl8wtxkbm0c0000gn/T/opencode/at-a5f616f-run/output --force
  ```
  Exit code 0, log line: `video w8GM1mghKJc: completed successfully`.
  Environment deviations, recorded as-is:
  - Video length 19:26 (user-accepted deviation from "20+ minutes").
  - No manual subtitles available this run → pipeline fell back to whisper (`subtitle_type: "whisper"`,
    model `belle-zh`). Summary generated from the whisper transcript.
- Fix scope checked (`git show a5f616f --stat`): the commit reframes the three article templates
  (`prompts/builtin/summary-article-{zh-Hant,en,ja}.md`) to the analyst-own-voice rule and
  removes every banned-pattern example; it also aligns
  `pipeline/mermaid_insert_test.go` with the 607448c fallback insertion point. No other code change.
- Produced file: `…/output/@老李玩錢/2026-11-05__w8GM1mghKJc__中期选举尘埃落定美股分化严重何去何从/2026-11-05__w8GM1mghKJc__summary.md`
  (162 lines; SHA-256 `4ff8a715feb5e98af8b5e06072da66309eb86bc6f1bb289e53b1a6e4ffa9e2c5`).

## 1. Body begins immediately after frontmatter with `## 主要觀點` (H2) distilling core positions, not a segment recap
- How I tried it: read the file head; `grep -n "^#" <summary.md>`.
- What came back: frontmatter ends at line 29 (`---`), line 30 is the tool-inserted
  video-embed image line (`![](https://www.youtube.com/watch?v=w8GM1mghKJc)`), line 32 is
  `## 主要觀點` — the first section of the body. Prose lines 34/36 state the core positions
  directly (divergence = post-election rotation, not systemic weakness; long-term thesis of
  semis/storage unchanged; diversify + keep cash), not a segment recap.
- Evidence: headings listing in section 2; file lines 29–36 (frontmatter `---` at 29, embed at 30,
  `## 主要觀點` at 32).

## 2. Continuous article in `###` chapters in original narrative order; concrete facts retained
- How I tried it: `grep -nE '^### ' <summary.md>`; read the whole file; compared chapter order
  against the transcription's narrative order.
- What came back:
  ```
  57:### 期中選舉落定與不確定性消除
  81:### 油價回落與通膨緩和
  106:### 科技板塊內部的分化：用電與排放政策
  133:### 上半年半導體與存儲撐起大盤，板塊輪動屬正常現象
  141:### 足球隊式的組合配置
  149:### 個人操作計畫（非投資建議）
  160:### 說明欄
  ```
  7 chapters follow the video's narrative order (election result → oil/inflation → tech split →
  semis/storage thesis → portfolio allocation → personal plan → description box). Concrete facts
  retained: 接近3%（道瓊漲幅）、單日跌逾8%、90 美元（WTI）、27%（德州資料中心電力）、
  3,900 多美元（帳戶當日）、32.6 萬美元（總資產）、173/180 股（英偉達）、199 美元（加倉線）、
  20/50 股（博通）、315 美元（跳空缺口下緣）、368/352 美元（特斯拉）、135/120 美元（SpaceX）；
  names: 微軟、亞馬遜、谷歌、英偉達、AMD、博通、吉娜（德州民主黨州長候選人）、川普、特斯拉、SpaceX。
- Evidence: headings above; excerpt lines 149–162 (personal plan keeps all entry/exit prices).

## 3. Article completes without truncation; single-video run exits 0
- How I tried it: read the last lines of the file; checked run exit code and log.
- What came back: file ends at line 162 with a complete sentence
  （「影片說明欄提供盈透證券（Interactive Brokers）的開戶連結，並附有相關開戶活動資訊。」）；
  no truncation marker, no mid-sentence cut. Run log ends `video w8GM1mghKJc: completed successfully`,
  `EXIT=0`. Stage 1 log: `provider=claude-code model=haiku response_length=4890`.
- Evidence: run.log tail (above); last 3 lines of the summary file.

## 4. No pre-existing summary file modified (SHA-256 snapshot before/after)
- How I tried it: before the run and after the run, hashed every `.md` file under
  `/Users/kouko/youtube-summarize-scraper/ytss-output`:
  ```
  find ytss-output -name '*.md' -type f -print0 | sort -z | xargs -0 shasum -a 256 > .../ytss-output-before.sha256
  find /Users/kouko/youtube-summarize-scraper/ytss-output -name '*.md' -type f -print0 | sort -z | xargs -0 shasum -a 256 > .../ytss-output-after.sha256
  ```
  (before run from the main checkout; after-file paths normalized to the same relative form).
- What came back: 25,486 files hashed each time; `diff before after` → identical, no changes.
  The run wrote only into the isolated temp output dir under
  `/private/var/folders/pv/m7y9m2v56lg68nl8wtxkbm0c0000gn/T/opencode/`.
- Evidence: `diff ytss-output-before.sha256 ytss-output-after.norm.sha256` → empty
  (`IDENTICAL: all 25,486 summary files unchanged`).

## 5. C-mode: article prose ≈40–60% of transcription length; only spoken filler compressed
- How I tried it: python measurement over `summary.md` and `transcription.md`
  (same method as the superseded run: prose = body after frontmatter minus ```mermaid blocks;
  transcript body = pipeline's `transcriptText`, i.e. transcription.md minus frontmatter minus
  the video-embed line — SRTToText strips timestamps; the pipeline itself injects
  `len([]rune(transcriptText))` as `{{transcription_length}}`).
- What came back:
  ```
  transcript body runes: 5481
  summary body, mermaid stripped, incl. embed line: 1857 runes  = 33.9%
    excl. embed line: 1808 runes  = 33.0%
    excl. embed + headings: 1678 runes  = 30.6%
  0.4x transcript = 2192 ; 0.6x = 3288
  ```
  The article (1,857 runes even counting the tool-inserted embed line) is below the 40% floor
  (≥ 2,192 runes the template demands: 「確保文章散文至少 40%」). Previous round measured
  2,257/5,530 = 40.8% (in-band); this round is 33.9% — the length criterion fails.
- Evidence: script output above (script reproduced inline in this section's method).

## 6. No meta-narration (no attribution to source medium/narrator, no reporting verbs)
- How I tried it (grep broadly, then read every hit in context):
  ```
  grep -n '影片\|本集\|頻道\|講者\|作者\|指出\|表示\|強調\|認為\|提到\|引用' <summary.md>
  grep -niE 'the video|this episode|the channel|the author|the speaker|\bhe\b|\bshe\b|notes|states|emphasizes|believes|mentions|cites' <summary.md>
  ```
- What came back: 2 hits, both the sole-allowed exception forms:
  ```
  151:以下為影片中的個人操作計畫，非投資建議。          ← disclaimer-style note (allowed)
  162:影片說明欄提供盈透證券（Interactive Brokers）的開戶連結，並附有相關開戶活動資訊。  ← description-box content (allowed)
  ```
  No sentence uses 影片/本集/頻道/作者/講者/他／她 as subject or possessive outside those two
  exceptions; no reporting verb (指出/表示/強調/認為/提到/引用) anywhere; no English equivalents.
  Violation count: **0** (this round's requirement was 0; accepted residual ≤1 borderline).
- Evidence: grep outputs above; full file read (162 lines) for context.

## 7. `## 主要觀點` both distills core positions AND directly answers the title's question (additive)
- How I tried it: read lines 32–36 of the summary; title is 「中期选举尘埃落定！美股分化严重，何去何从」.
- What came back: line 34 answers directly:
  「標題所問『何去何從』的答案是：短期內板塊將持續輪動；半導體與存儲作為AI發展的基石，
  長期邏輯未變，但單一板塊重倉會放大風險，因此分散配置與保留現金部位才是組合的基礎。」
  The same paragraph distills the core positions (rotation is normal; thesis unchanged;
  diversify), and line 36 adds the historical context (post-election 12 months mostly up).
- Evidence: file lines 32–36.

## 8. Dynamic mermaid caps: transcription ≈6,000 chars → tier <10000 → 3–4 diagrams; overview diagram position
- How I tried it: `grep -n '```mermaid' <summary.md>`; `grep -c '^#### ' <summary.md>`;
  transcription length from section 5 (5,481 runes → tier `<10000` → total 3–4 per
  `mermaidLimit`, `summarizer/summarizer.go:258`).
- What came back:
  ```
  38:```mermaid      ← Key Viewpoints overview
  67:```mermaid      ← chapter 1 (期中選舉落定與不確定性消除)
  91:```mermaid      ← chapter 2 (油價回落與通膨緩和)
  117:```mermaid     ← chapter 3 (科技板塊內部的分化)
  ```
  4 diagrams total = at the cap, within it. The first diagram (line 38) sits directly under the
  `## 主要觀點` prose (lines 34/36) and before the first `###` chapter heading (line 57), with no
  `####` heading (`^#### ` count = 0). The remaining 3 diagrams hang on 3 of the 7 chapters —
  not one per chapter.
- Evidence: grep outputs above; file lines 34–57.

## 9. No `### 重點整理`/`### 概述`, no duplicated heading, body entirely Traditional Chinese
- How I tried it:
  ```
  grep -c '### 重點整理\|### 概述' <summary.md>   → 0
  grep -c '## 主要觀點' <summary.md>              → 1 (no duplicate)
  grep -c '^#### ' <summary.md>                   → 0
  ```
  Simplified-only character scan over the body (lines 30–162) using a simplified-only
  codepoint set (产东问这那个时间数据关国对说为与会后发经实现务报应电买卖万亿设让过还进来没们标选题举党总统动态构资讯网络银铁飞机等):
  hits: `{'用': 7}` — 用 is a shared character (present in both scripts), not simplified-only;
  all other simplified-only chars: 0 hits. The only simplified characters in the file are in the
  frontmatter `title` (「中期选举尘埃落定…何去何从」), which is the creator's original title —
  explicitly allowed.
- Evidence: greps above; scan output.

## REQ-9 (static). en/ja templates carry title answering + direct-statement (no-attribution) rule + tiered diagram table; {{mermaid_limit}} present; MermaidLimitText language-aware
- How I tried it: read the three `prompts/builtin/summary-article-{zh-Hant,en,ja}.md` at a5f616f;
  checked the mermaid templates and `summarizer/summarizer.go:258-320` (`mermaidLimit`,
  `mermaidLimitLang`, `MermaidLimitText`) and `summarizer/mermaid.go:22` (placeholder substitution).
- What came back:
  - All three article templates: analyst-own-voice framing, no-attribution rule, 40%/60% length
    guidance, title-question answering rule, and the tiered diagram table
    (<500 → 1–2, <3000 → 2–3, <10000 → 3–4, ≥10000 → 4–5). en template lines 39–46;
    ja lines 37–44; zh-Hant lines 36–44.
  - `{{mermaid_limit}}` present in all three mermaid templates
    (`prompts/builtin/mermaid-en.md:2`, `mermaid-ja.md:2`, `mermaid-zh-Hant.md:2`), substituted
    at `summarizer/mermaid.go:22` via `MermaidLimitText(summaryLength, language)`.
  - `MermaidLimitText` has three language branches (zh / ja / default-en, `summarizer.go:292-318`).
  - Tier logic `summarizer.go:258-269` matches the tables in the templates.
- Evidence: excerpts above; `git show a5f616f -- prompts/` diff for the reframe.

## UI flow (spec): run single video → summary file appears; viewpoint-first format, bounded diagrams, keywords unchanged
- How I tried it: the vehicle run itself (`go run . video w8GM1mghKJc --llm claude-code -o <temp> --force`).
- What came back: file created at `<temp>/@老李玩錢/2026-11-05__w8GM1mghKJc__…/2026-11-05__w8GM1mghKJc__summary.md`;
  frontmatter first (tags list present, `subtitle_type: "whisper"`, duration 19:26), then
  `## 主要觀點`, then `###` chapters, 4 mermaid diagrams, run completed exit 0.
- Evidence: run.log; produced file.

## Re-run on 2026-11-07, at a5f616f (after reframe commit 607448c..a5f616f)
- All rows re-tested in full: the templates drive the entire article and a re-run regenerates
  everything, so no row could be carried over safely.
- The previous run's single hard failure (#6, 12 meta-narration violations) is fixed: 0 this
  round (only the two allowed exception forms appear). #9's residue (本集) is also gone.
- New regression: #5 (C-mode length) fell from 40.8% to 33.9%, below the 40% floor.
- #4 snapshot re-run: 25,486 files identical before/after.
