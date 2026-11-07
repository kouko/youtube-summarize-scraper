# Article-style summary with viewpoint-first section — acceptance test evidence

Tried on 2026-11-07, in a clean copy of the project at `4686b7c7fc7300858b8eca525d70ec15f90585ab` (branch `feat/2026-11-06-article-style-viewpoint-summary`).

Test environment: clean `git worktree add` at HEAD in `/private/var/folders/pv/m7y9m2v56lg68nl8wtxkbm0c0000gn/T/opencode/acceptance-2026-11-06-article-style`. Output isolated to `/private/var/folders/pv/m7y9m2v56lg68nl8wtxkbm0c0000gn/T/opencode/ytss-acceptance-output`. LLM: claude-code CLI (haiku); network + Chrome cookie extraction available.

## Setup (all runs)

- `git worktree add <temp>/acceptance-2026-11-06-article-style HEAD` → clean checkout at `4686b7c7fc7300858b8eca525d70ec15f90585ab`.
- `go build ./...` in the clean checkout first fails with `embedded/embedded.go:14:12: pattern bin: cannot embed directory bin: contains no embeddable files` until the platform tool binaries are provisioned. Per README, `make build` provisions only missing deps ("fast and offline once deps exist"); the tool binaries (yt-dlp, ffmpeg, whisper-cli — third-party tools, unchanged by this change) exist in this machine's environment, so they were provisioned into `embedded/bin/darwin-arm64/`; `go build ./...` then passed, and `go build -o ytss-test .` passed too.
- `config.yaml` is gitignored (`.gitignore:10:config.yaml`) and therefore **absent in the clean checkout**. A first run without it fell back to built-in defaults (`config/config.go:70-71`: `Language: "en"`, `Style: "outline"`) and produced an English outline summary (`### Overview`, `### Section Summary`, `### Key Takeaways` — not the change under test). That run is **void** and discarded; the change's behavior depends on the user's `summary.style: article` + `summary.language: zh-Hant`, so the user's real `config.yaml` was copied into the worktree and the run repeated. The void run is recorded here only as a setup note.
- Acceptance-line walk below is the re-run with the user's real config (the environment the change ships for).

Final run command (from the clean worktree with the user's config):

```
go run . video w8GM1mghKJc --llm claude-code -o /private/var/folders/pv/m7y9m2v56lg68nl8wtxkbm0c0000gn/T/opencode/ytss-acceptance-output --force
```

Key log lines:

```
language detected from title/description language=zh
subtitle download succeeded video_id=w8GM1mghKJc language=su type=manual
stage 1: generating summary video_id=w8GM1mghKJc
stage 1 complete video_id=w8GM1mghKJc provider=claude-code model=haiku response_length=5410
stage 2: extracting keywords video_id=w8GM1mghKJc
stage 3: generating mermaid diagrams video_id=w8GM1mghKJc
summary written video_id=w8GM1mghKJc path=…/2026-11-05__w8GM1mghKJc__summary.md
video processing complete … 
video w8GM1mghKJc: completed successfully
```

Exit code 0. Manual subtitles were used (no whisper transcription on this run); frontmatter: `subtitle_type: "manual"`, `llm_provider: "claude-code"`, `llm_model: "haiku"`, `duration: "19:26"`.

## 1. Summary body begins, immediately after the frontmatter, with `## 主要觀點` (H2) distilling the speaker's viewpoints, not a segment recap

- How I tried it: `grep -n "^#" <summary.md>`; read the first 35 lines.
- What came back: frontmatter (lines 1–24), the standard pipeline video-embed image line (line 26), then line 28: `## 主要觀點`. It is the first section heading; no `###`/`####` precedes it. The section (lines 30–32) states the core judgments: market divergence after the midterm results, rotation into traditional/defensive sectors, tech divergence between cloud giants and chips, the judgment that heavy-concentration losses are normal rotation, long-term semis thesis intact, stay diversified with a cash reserve. It is viewpoint distillation, not a segment-by-segment recap.
- Evidence: `grep -n "^#"` → `28:## 主要觀點`, `34:### 選舉前後的歷史規律與市場反應`, `73:### 油價暴跌與通膨預期`, `97:### 科技股內部的分化：AI 資本支出與晶片股`, `124:### 上半年的板塊輪動與長期邏輯`, `130:### 個人操作計畫：英偉達與博通`, `146:### 個人操作計畫：特斯拉與 SpaceX`, `160:### 帳戶表現與說明欄`.

## 2. Body continues as a continuous article in `###` chapters following the video's original narrative order; concrete facts (numbers, amounts, dates, names, cases, quotes) retained

- How I tried it: compared the chapter order against the transcription's narrative order; grepped the summary for the concrete facts.
- What came back: chapter order (election history → oil-price drop → tech divergence → H1 rotation & long-term thesis → position plans NVDA/AVGO → TSLA/SPCE → account + description box) follows the video's narrative order (transcript: account/market open → indices divergence → midterm history → oil → tech divergence → H1 rotation → long-term → NVDA/AVGO plan → TSLA/SPCE → closing/description box). Retained numbers: 3% Dow gain, WTI −8%+ to below $90, Texas 27% of US data-center power, account +$3,900, total assets $326k, NVDA 173 shares/target 180/add below $199, AVGO 20 shares/target 50/add near $315, TSLA open $368/add $352, SPCE open below $135/add below $120. Names retained: 民主黨, 特朗普, 馬斯克, 德克薩斯州, 英偉達, 博通, 特斯拉, SpaceX.
- One detail dropped: the transcript names the Texas candidate 吉娜 (transcription.md line 46-47); the article says 該州州長候選人 without the name (the case itself is retained).
- Evidence: chapter list above; grep hits for the numbers (`173`, `3900`, `27%` etc.); `grep -n "吉娜" <summary.md>` → no hit; `grep -n "吉娜" <transcription.md>` → hit at line 46-47.

## 3. Article completes without truncation (no mid-sentence cut); batch/single run completes normally

- How I tried it: read the last lines of the summary file; observed the run's exit status.
- What came back: the article ends with 「…並提及相關開戶活動的免費股票優惠。」 — a complete sentence; no truncation marker, no cut mid-word. The single-video run printed `video w8GM1mghKJc: completed successfully` and exited 0. Batch shares the same per-video pipeline path; the batch/TUI surfaces are outside this change's boundary (spec §Boundary). Note the test vehicle is 19:26, not 20+ minutes — the user accepted this video as the test vehicle during build (recorded deviation, intent #3 originally said 20+ min).
- Evidence: `tail -c 400 <summary.md>`; run output (exit 0).

## 4. No pre-existing summary file is modified by a run (snapshot-and-compare)

- How I tried it: hashed every existing `*__summary.md` under the configured output dir `ytss-output/` (12,578 files, sha256) before the run; re-hashed the same set after the run; sorted and diffed.
- Command: `find ytss-output -type f -name "*__summary.md" -print0 | xargs -0 shasum -a 256 > <snapshot>`; same after the run; `diff <(sort before) <(sort after)`.
- What came back: both lists have 12,578 lines; `diff` exit 0 (identical). The test run wrote only under the isolated temp output dir. The pre-existing summary for this same video (`ytss-output/@老李玩钱/2026-11-05__w8GM1mghKJc__中期选举尘埃落定美股分化严重何去何从/2026-11-05__w8GM1mghKJc__summary.md`, sha256 `386e3cc6e0327ef8cf2b45ce6f2280cc23ddd5cc6d10179f274be91c0cc67623`) is unchanged.
- Evidence: `/private/var/folders/pv/m7y9m2v56lg68nl8wtxkbm0c0000gn/T/opencode/ytss-acceptance-preexisting-summary-sha256.txt` and `…-after-summary-sha256.txt` (12,578 entries each); diff exit 0.

## 5. C-mode: article ≈40–60% of transcription length; only spoken filler compressed

- How I tried it: measured rune counts (Python) — transcription file 5,994 runes, transcript body (frontmatter stripped) 5,530 runes; summary body after frontmatter 5,658 runes incl. mermaid blocks (3,619 runes), prose-only (mermaid and headings stripped) 1,909 runes.
- What came back: prose-only article = 1,909/5,530 = **34.5%** of the transcript body (below the 40–60% band). Summary body including mermaid = **102.3%** of the transcript body (above the band). Stage-1 model output was `response_length=5410` bytes (≈1,800 han characters, ≈33%), consistent with the prose measure. The article did compress the transcript's heavy spoken filler and retained the substantive facts, but the length does not land in the 40–60% band on either measure.
- Evidence: `python3 measure_ratio2.py` output: `article prose / transcript body: 34.5%`, `article body incl mermaid / transcript body: 102.3%`.
## 6. No meta-narration: no sentence uses 影片/影片作者/作者/講者/他/她/本頻道 (or the-video/the-author/the-speaker) as sentence subject or possessive modifier

- How I tried it: `grep -n "影片\|作者\|講者\|本頻道" <summary.md>` and a `他/她`-as-subject scan.
- What came back: two violations of the banned pattern (影片 + reporting verb as subject):
  - line 128: 「**影片強調**，投資組合應依照足球隊的概念打造…」
  - line 150: 「特斯拉與 SpaceX 相關股票今日因中期選舉結果下跌，**影片指出**原因與公司基本面無關…」
  Allowed exceptions present and correctly shaped: three disclaimer lines (32/132/148 「以下為影片中的個人操作計畫，非投資建議」) and one neutral description-box line (162 「影片說明欄提供盈透證券（IBKR）的開戶連結」). No 他/她 as subject anywhere.
- Verdict note: the accepted stochastic residual is ≤1 borderline instance per round; observed count is 2, and both are the exact canonical banned pattern (not borderline shapes), so the row fails.
- Evidence: grep output above; `grep -nE "他(的|指出|表示|強調|認為|提到|說)|她(的|指出|表示|強調|認為|提到|說)"` → no hits.

## 7. `## 主要觀點` does two things together: distills core positions AND directly answers or explains the title's question/promise/theme (additive)

- How I tried it: read the `## 主要觀點` section (lines 28–32) against the title 「中期選舉塵埃落定！美股分化嚴重，何去何從」.
- What came back: the section states what happened (divergence, rotation drivers) and then the where-to-go conclusion: rotation into traditional/defensive sectors is a normal market phenomenon; heavy concentration in one sector caused the losses; semis/storage long-term thesis unchanged; portfolios should stay diversified and balanced with a cash reserve and no single-sector over-concentration. That is both a core-position distillation and the answer to the title's 何去何從 question (what to hold, what to avoid, where the rotation leads). The answer is woven into the paragraph rather than front-loaded with an explicit 「答案是…」 phrase.
- Evidence: section text quoted in the report; verdict works with that note.

## 8. Dynamic mermaid limits: total capped by transcription length tier; first diagram = the 主要觀點 overview WITHOUT a `####` heading; remaining diagrams only from `###` chapters, never one per chapter

- How I tried it: `grep -n '```mermaid' <summary.md>`; `grep -c "####" <summary.md>`; read each diagram's content; compared to the transcript-length tier.
- What came back: transcription ≈6,000 chars → tier `<10000` → cap 3–4. Observed **4 diagrams** (blocks at lines 43, 57, 82, 109) — within the cap. **No `####` heading anywhere** (count 0), so no duplicated `## 主要觀點` heading. The overview diagram is present and headingless — its content mirrors the `## 主要觀點` section (divergence → funds rotation → traditional sectors benefit → tech split → heavy-concentration loss is normal → portfolio strategy: diversify, cash, avoid concentration). However, in file order it is the **second** mermaid block: the first block (line 43) depicts chapter-1 content (選前承壓→選舉落幕→市場反應→歷史數據), and both blocks are stacked at the end of chapter 1, before the chapter-2 heading — the `## 主要觀點` section itself has no diagram directly under it. The remaining diagrams (82 = 油價暴跌 chapter, 109 = 科技股分化 chapter) come from `###` chapters; 3 chapter diagrams for 7 chapters — not one per chapter.
- Evidence: grep line numbers; block content read; `####` count 0.
- Verdict note: count ✓, no-duplicate-heading ✓, not-one-per-chapter ✓; but "first diagram = the 主要觀點 overview" is not met in file order (overview is 2nd; chapter-1 diagram is 1st), so the row is `partly`.

## 9. No meta-narration residue beyond #6, no `### 重點整理`/`### 概述` extra blocks, no duplicated heading, output entirely in Traditional Chinese

- How I tried it: `grep -n "重點整理\|概述"`; `grep -c "^## 主要觀點"`; `grep -c "####"`; automated scan for simplified-only characters in the prose (≈50 typical simplified↔traditional pairs, e.g. 这/说/发/们/时/经/现/电/车/长/门/开/让/还/后/产/场/机/体/华/过/进/种/别/条/里/两/学/会/个/认/问/题/对/转/单/从).
- What came back: no `### 重點整理` / `### 概述` blocks; `## 主要觀點` appears exactly once; no `####` headings; simplified-character scan: 0 hits in the prose. (The frontmatter `title:` keeps the creator's original Simplified-Chinese video title — source metadata copied verbatim, existing behavior, not part of the article body.)
- Evidence: grep counts; `python3 scan_simplified.py` → "no simplified-only characters found in prose".

## REQ-9 (spec) — Three-language sync: zh-Hant / en / ja templates carry the same rules (static check)

- How I tried it: read `prompts/builtin/summary-article-{en,ja}.md` and `prompts/builtin/mermaid-{en,ja}.md`; checked `summarizer/summarizer.go` tier logic.
- What came back: en and ja article templates exist and carry (a) title-question answering (`## Key Viewpoints` line 25 in each; ja line 25), (b) the positive sentence-subject whitelist with the rewrite method and the two sole exceptions (lines 3–7 in each), (c) the tiered mermaid count table (lines 37–44 in each). The en/ja mermaid templates contain the `{{mermaid_limit}}` placeholder (mermaid-en.md:2, mermaid-ja.md:2), and `MermaidLimitText` (`summarizer/summarizer.go:288`) is language-aware (zh/ja/en branches) with tiers `mermaidLimit` (`summarizer.go:258`: <500 → total 2, <3000 → 3, <10000 → 4, ≥10000 → 5).
- Evidence: file reads above; `grep -n "mermaid_limit" prompts/builtin/mermaid-en.md prompts/builtin/mermaid-ja.md` → both line 2.

## UI flow (spec) — Run the summarizer (single video) → summary file appears in the usual place with the new shape

- How I tried it: the single-video run above (the change's entire surface is the per-video pipeline + templates + config style; batch and TUI surfaces are unchanged per spec boundary).
- What came back: the file appeared under the output dir at `<output>/@老李玩钱/2026-11-05__w8GM1mghKJc__中期选举尘埃落定美股分化严重何去何从/2026-11-05__w8GM1mghKJc__summary.md`; frontmatter first, then `## 主要觀點`, then `###` chapters in narrative order, 4 mermaid diagrams, keywords in frontmatter tags as before. Run completed with exit 0.
- Evidence: run log + file inspection above.

## Re-run notes

- First run (void, defaults `en`/`outline`): recorded above under Setup; discarded because it did not exercise the change. All rows above are from the re-run with the user's real config at the same HEAD.
