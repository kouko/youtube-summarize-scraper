# 文章式摘要：觀點章節優先，正文依原始敘事順序保留細節 — acceptance test evidence

Tried on 2026-11-08，在乾淨專案複製中測試於 `ff3e44f384bcd8c13e87fab59ce775cbf39a1a40`
(re-test after combined fix: analyst-voice prompt a5f616f + pipeline length guard ff3e44f; this file replaces the b4a0373 run).

## 0. Environment and setup
- Fresh `git worktree add /private/var/folders/pv/m7y9m2v56lg68nl8wtxkbm0c0000gn/T/opencode/at-ff3e44f ff3e44f`
  → detached HEAD `ff3e44f384bcd8c13e87fab59ce775cbf39a1a40`, clean tree.
- `embedded/bin/` is gitignored, so a bare build in the fresh copy cannot embed the tools
  (`make deps` provisions them). Per the project's own build instructions I copied the prebuilt
  darwin-arm64 binaries (yt-dlp / ffmpeg / whisper-cli) from the main checkout into the worktree's
  `embedded/bin/darwin-arm64/`. Build tooling, not part of the change.
  `go build ./...` → BUILD_OK.
- `go test ./...` (run per the station's instruction; the full-suite gate still executes at
  finalize-review) → every package `ok`, exit 0.
- Fix scope checked (`git show ff3e44f --stat`): exactly one file,
  `pipeline/pipeline.go`, 39 insertions — the length guard with 40% floor and one retry.
  No prompt changes in this commit; the analyst-voice prompt changes are in a5f616f (parent).
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
    -o /private/var/folders/pv/m7y9m2v56lg68nl8wtxkbm0c0000gn/T/opencode/at-ff3e44f-run/output --force
  ```
  Exit code 0, final log line: `video w8GM1mghKJc: completed successfully`
  (`…/at-ff3e44f-run/run.log`; failed attempts: `run.log`, `run2.log`, `run3.log`).
  - Video length 19:26 (user-accepted deviation from "20+ minutes").
  - No manual subtitles available → whisper fallback (`subtitle_type: "whisper"`, model `belle-zh`),
    same as the previous round.
  - Stage 1: `provider=claude-code model=haiku response_length=6850` (bytes; ≈ 2,499 runes).
  - No "stage 1 summary below length floor, retrying" log line → length guard did not trigger.
- Produced file:
  `…/output/@老李玩钱/2026-11-05__w8GM1mghKJc__中期选举尘埃落定美股分化严重何去何从/2026-11-05__w8GM1mghKJc__summary.md`
  (193 lines; SHA-256 `1e72365e0d42376d30a7ee4fcc52e7f2d870252028566bb50b31e87974724711`).

## 1. Body begins immediately after frontmatter with `## 主要觀點` (H2) distilling core positions, not a segment recap
- How I tried it: `grep -n '^#' <summary.md>`; read lines 30–36.
- What came back: frontmatter ends at line 29 (`---`), line 30 is the tool-inserted video-embed
  line (`![](https://www.youtube.com/watch?v=w8GM1mghKJc)`), line 32 is `## 主要觀點` — the first
  section of the body. Lines 34–36 state core positions (post-election divergence is the normal
  pre-election-risk / post-election-relief pattern; rotation, not systemic weakness; diversify and
  keep balance) and answer the title's question ("何去何從"), not a segment recap.
- Evidence: heading list in section 9; file lines 30–36.

## 2. Continuous article in `###` chapters in original narrative order; concrete facts retained
- How I tried it: `grep -nE '^### ' <summary.md>`; read the whole file; compared chapter order and
  every concrete number/name against `transcription.md`.
- What came back:
  ```
  57:### 美股分化與帳戶表現
  75:### 中期選舉的歷史規律與不確定性消除
  103:### 油價回落與通膨預期
  131:### 電力政策對AI產業鏈的衝擊
  156:### 半導體與存儲的上半年主導與板塊輪動
  167:### 半導體與存儲的長期邏輯與資產配置
  173:### 個人操作計畫與估值觀點
  191:### 說明欄資訊
  ```
  8 chapters follow the transcript's narrative order (election result + oil/inflation → tech split → portfolio + semis thesis → TSLA/SpaceX entries → closing). Retained facts: 3,900 美元當日漲、32.6 萬美元總資產、蘋果大漲、八個百分點（WTI）、九十美元（WTI）、近三個百分點（道指）、27%（德州數據中心電力）、173/180 股與 199 美元（輝達）、20/50 股與 315 美元（博通）、368/352 美元（特斯拉）、135/120 美元（SpaceX）、十二個月（選後規律）、三月／四月（上半年走勢）；names: 微軟、亞馬遜、谷歌、輝達、AMD、博通、吉娜、川普、馬斯克、特斯拉、SpaceX、好市多、迪士尼、伯克希爾、Visa.
  **No dropped concrete facts** — the previously missing account amounts and Apple are now present. The new per-chapter full-coverage rule appears obeyed.
- Evidence: heading list above; inline python token comparison in section 5's method; read of
  `transcription.md` lines 23–30 and 59–63 against summary lines 54–115.

## 3. Article completes without truncation; single-video run exits 0
- How I tried it: read the last lines; checked the run exit code and log; grepped for truncation
  markers (`…`, `...`, `回應|未完|to be continued` → 0 hits).
- What came back: file ends at line 193 with a complete sentence
  （「說明欄提供盈透證券（IBKR）的開戶連結，並附有開戶入金的相關活動資訊。」）;
  no truncation marker, no mid-sentence cut. `EXIT=0`; log ends `video w8GM1mghKJc: completed successfully`.
  Recorded deviations: video is 19:26 (user-accepted); whisper fallback this round.
- Evidence: run.log tail; last 3 lines of the summary file.

## 4. No pre-existing summary file modified (SHA-256 snapshot before/after)
- How I tried it: before and after the run, hashed every `.md` file under
  `/Users/kouko/youtube-summarize-scraper/ytss-output`:
  ```
  find ytss-output -name '*.md' -type f -print0 | sort -z | xargs -0 shasum -a 256 > .../ytss-output-before.sha256
  find ytss-output -name '*.md' -type f -print0 | sort -z | xargs -0 shasum -a 256 > .../ytss-output-after.sha256
  ```
  Normalized paths (strip `/Users/kouko/youtube-summarize-scraper/` prefix) and compared:
  `diff <(sort before_normalized) <(sort after_normalized)` → empty
  (`IDENTICAL: all summary files unchanged`). The run wrote only into the isolated temp output dir
  under `/private/var/folders/pv/m7y9m2v56lg68nl8wtxkbm0c0000gn/T/opencode/`.
- Evidence: both hash files and the empty normalized diff.

## 5. C-mode: article prose 40–60% of transcription length; only spoken filler compressed
- How I tried it: python measurement over `summary.md` and `transcription.md` (same method as the
  previous report: summary prose = body after frontmatter minus ```mermaid blocks, counting non-blank
  line runes; transcript body = transcription.md minus frontmatter minus the video-embed line;
  the pipeline injects `len([]rune(transcriptText))` at `pipeline/pipeline.go:834`).
- What came back:
  ```
  transcript body runes (no newlines): 5427
  summary prose, mermaid stripped, incl. embed line: 2547 runes  = 46.9%
    excl. embed line: 2499 runes  = 46.0%
    excl. embed + headings: 2364 runes  = 43.6%
  0.4x = 2170 ; 0.45x = 2442 ; 0.5x = 2713 ; 0.6x = 3256
  ```
  The article (2,547 runes even counting the tool-inserted embed line) is **above the acceptance
  floor (≥ 2,170)** and **above the template's own new 45% floor (≥ 2,442)**. The 50% target (2,713) is not yet reached but the jump from 33.9% (1,856) to 46.9% (2,547) is a +37% relative improvement.
  Pipeline guard: Stage 1 response_length=6850 bytes ≈ 2,499 runes; targetFloor = 5427 * 40 / 100 = 2170 runes. Since 2,499 > 2,170, no retry triggered (no "stage 1 summary below length floor, retrying" log).
  Previous round: 1,856/5,480 = 33.9%. This round: 2,547/5,427 = 46.9%.
- Evidence: script output above; run.log shows no retry log line.

## 6. No meta-narration (no attribution to source medium/narrator; no reporting verbs)
- How I tried it (grep broadly, then read every hit in context):
  ```
  grep -n '影片\|本集\|頻道\|講者\|作者\|指出\|表示\|強調\|認為\|提到\|引用\|他\|她' <summary.md>
  grep -niE 'the video|this episode|the channel|the author|the speaker|\bhe\b|\bshe\b|notes|states|emphasizes|believes|mentions|cites' <summary.md>
  ```
- What came back: the English scan is clean; the Chinese scan returns the two disclaimer lines (allowed)
  plus the word "其他" (meaning "other") in mermaid node labels and prose — not "他" as standalone pronoun:
  ```
   59:（以下為影片中的個人操作計畫與帳戶數據，非投資建議）       ← disclaimer exception (allowed)
  158:華爾街通常不會讓同一個板塊長期扛起大盤，因此板塊輪動勢必出現，讓其他板塊接力上漲  ← "其他" = other, not "他"
  165:若只押注前鋒，行情輪到其他位置時，損失會相當大。現金部位就是守門員  ← "其他" = other
  175:（以下為影片中的個人操作計畫，非投資建議）                 ← disclaimer exception (allowed)
  ```
  Additional targeted search for reporting-verb patterns:
  `grep -nE '影片.{0,12}(提醒|表示|強調|認為|提到|引用|指出|建議|說明|稱)'` → only hits the two disclaimer lines.
  `grep -nE '指出|表示|強調|認為|提到|引用|提醒|聲稱|宣稱|解釋說|補充說'` → 0 hits in prose.
  `grep -nE '[^其]他[^們]|她'` → 0 hits (the "他" in "其他" is excluded by pattern).
  Mermaid block scan: `awk '/^```mermaid/,/^```/' summary.md | grep -E '他|她|影片|講者|作者'` → no hits.
  Violation count: **0** (the two disclaimer hits are allowed exceptions; "其他" is not a pronoun reference).
  Previous round was 3 — this round fixed to 0 by the analyst-voice prompt (removing banned-word examples, positive subject whitelist).
- Evidence: grep outputs above; full file read (193 lines) for context.

## 7. `## 主要觀點` both distills core positions AND directly answers the title's question (additive)
- How I tried it: read lines 32–36; title is 「中期选举尘埃落定！美股分化严重，何去何從」.
- What came back: line 34 answers directly:
  「這正是標題『何去何從』所問的核心。當日三大指數同步走高，道瓊工業指數漲近3%，納斯達克與標普500指數僅小幅上揚。板塊內部的差異更大：微軟、蘋果、谷歌等大型科技股與傳統權值股走強，輝達、特斯拉等與半導體硬體相關的個股則普遍下跌。這種分化並非普漲或普跌，而是資金在板塊之間輪動的結果。」
  Line 36 gives the conclusion:
  「對美股的去向，答案是中期方向仍偏多，但主導力量正在更換。歷史上，中期選舉結束後的十二個月，美股多數時候表現強勢，因為選舉結果消除了前期的不確定性。同時，油價回落與電力政策分別讓傳統防禦與價值板塊受惠，並對半導體與雲端資本支出鏈形成壓力。在半導體與存儲板塊長期邏輯未變的前提下，短期更適合以分散配置應對板塊輪動，而不宜重倉單一板塊。」
  The same section distils the core positions (divergence is the typical election pattern; rotation is normal; diversify), so both halves of the criterion hold.
- Evidence: file lines 32–36.

## 8. Dynamic mermaid caps: transcription 5,427 → tier <10000 → 3–4 diagrams; overview position
- How I tried it: `grep -n '```mermaid' <summary.md>`; `grep -c '^#### '`; transcription length from
  section 5; tier code `summarizer/summarizer.go:255-268` (`<10000 → total 4`, additional range 2-3).
- What came back:
  ```
  38:```mermaid      ← overview, directly under the 主要觀點 prose
  86:```mermaid      ← chapter 1 (美股分化與帳戶表現)
  117:```mermaid     ← chapter 2 (中期選舉的歷史規律與不確定性消除)
  141:```mermaid     ← chapter 3 (電力政策對AI產業鏈的衝擊)
  ```
  4 diagrams total = at the cap (tier 3–4), within it. The first diagram (line 38) sits directly
  under the `## 主要觀點` prose (line 34) and before the first `###` chapter (line 57), with no
  `####` heading (`^#### ` count = 0). The remaining 3 hang on 3 of the 8 chapters — not one per
  chapter.
- Evidence: grep outputs above; file lines 34–57.

## 9. No `### 重點整理`/`### 概述`, no duplicated heading, body entirely Traditional Chinese
- How I tried it:
  ```
  grep -c '### 重點整理\|### 概述' <summary.md>   → 0
  grep -c '^## 主要觀點' <summary.md>             → 1 (no duplicate)
  grep -c '^#### ' <summary.md>                   → 0
  ```
  Simplified-only character scan over the body (after frontmatter, embed line excluded) using the
  same shared/simplified-only set as before: hits `{}` — no simplified-only character occurs. The only simplified text
  in the file is the frontmatter `title` (the creator's original title), which is source metadata,
  explicitly allowed.
- Evidence: greps above; scan output.

## REQ-9 (static). en/ja/zh-Hant templates carry the length anchor, title answering, no-attribution rule, tiered diagram table; `{{mermaid_limit}}` present; MermaidLimitText language-aware
- How I tried it: read the three `prompts/builtin/summary-article-{zh-Hant,en,ja}.md` at ff3e44f;
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
- Evidence: grep outputs above; `git show ff3e44f -- prompts/` diff (length lines only); template diffs in a5f616f.

## UI flow (spec): run single video → summary file appears; viewpoint-first format, bounded diagrams, keywords unchanged
- How I tried it: the vehicle run itself (`go run . video w8GM1mghKJc --llm claude-code -o <temp> --force`).
- What came back: file created under `<temp>/@老李玩钱/2026-11-05__w8GM1mghKJc__…/`;
  frontmatter first (tags list present, `subtitle_type: "whisper"`, duration 19:26), then
  `## 主要觀點`, then `###` chapters, 4 mermaid diagrams; run completed exit 0. The flow itself works;
  all content quality rows pass this round.
- Evidence: run.log; produced file.

## Re-run on 2026-11-08, at ff3e44f (combined: analyst-voice prompt a5f616f + pipeline length guard ff3e44f)
- All rows re-tested in full: the combined fix changes both the article templates (analyst voice, length anchor) and the pipeline (length guard with retry). A re-run regenerates the
  entire article, so no row could be carried over safely. The fix diff touches:
  - `a5f616f`: three article templates (removed banned-word examples, added analyst-voice framing, length anchor 50%/45%/per-chapter, title-answering, no-attribution whitelist).
  - `ff3e44f`: `pipeline/pipeline.go` length guard with 40% floor and one retry.
- #1 (viewpoint-first): works — unchanged structure.
- #2 (details retained): **improved to works** — previously dropped account amounts and Apple are now retained.
- #3 (complete article): works — unchanged.
- #4 (existing files untouched): works — identical to previous round snapshot.
- #5 (length): **improved to works** — 46.9% vs previous 33.9%; crosses 40% floor and 45% hard floor. Length guard measured 2,499 runes vs 2,170 floor, no retry needed.
- #6 (meta-narration): **improved to works** — 0 violations vs previous 3; analyst-voice prompt removed banned-word examples, positive whitelist converged.
- #7 (title answering): works — directly answers "何去何從" in opening.
- #8 (mermaid caps): works — 4 diagrams within 3–4 cap, correct placement.
- #9 (formatting): works — no extra blocks, no H4, Traditional Chinese.
- #10 (three-language sync): works — static check passes.
- UI flow: works — all quality gates pass.