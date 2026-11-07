# Article-style summary with a viewpoint-first section — spec
intent: 2026-11-06-article-style-viewpoint-summary@548bb47 (+ 2026-11-07-key-viewpoints-answer-title, confirmed in build)
pre-build-review: not-required — prompt-content change routed through the existing 4-level prompt resolution; no security or privacy surface, no irreversible action on existing files (Acceptance #4 pins them untouched), no public API contract, requirements confirmed by the user

## Requirements (recorded as changed during build, per closing-review §4)

REQ-1 — Viewpoint section leads the summary body
  WHEN a video is summarized, the system shall produce a summary whose body begins, immediately after the frontmatter, with a `## 主要觀點` (H2) section that distills the speaker's core positions and argumentation rather than a segment-by-segment recap → Acceptance #1 (renamed from `### 影片觀點` to `## 主要觀點` during build per user direction; heading level changed to match the mermaid stage's `###` chapter matching)

REQ-2 — Article body preserves original narrative order and details
  WHEN the viewpoint section ends, the system shall continue the summary as a continuous article divided into `###` chapters whose order follows the video's original narrative order, and the prose shall retain specific facts (numbers, amounts, dates, names, cases, direct quotes) instead of summarizing them away → Acceptance #2 (chapters moved from `####` to `###` during build)

REQ-3 — Long-video articles complete without truncation
  WHEN a video of 20 minutes or longer is summarized, the system shall produce an article that ends completely (no mid-sentence cut or truncation marker), and the batch run shall complete normally → Acceptance #3

REQ-4 — Existing summary files are left untouched
  The system shall not modify any summary file that exists before this change; the new format applies only to videos processed after the change → Acceptance #4

REQ-5 — C-mode compression (40–60% of transcription length)
  The article shall be ~40–60% of the original transcription length, compressing only spoken filler (conjunctions, repetition, filler words) while retaining all numbers, names, dates, quotes and cases → verified end-to-end in build (7 rounds)

REQ-6 — No meta-narration; objective narration
  No sentence may use "the video", "the video author", "the author", "the speaker", "he/she", or "this channel" as subject or possessive modifier (e.g. "the video notes/states/emphasizes", "the video author's interpretation", "the speaker's plan"). Sole exceptions: disclaimer-style source notes for opinionated content (e.g. "The following is the speaker's personal trading plan, not investment advice") and neutral description-box content. Enforced by a positive sentence-subject whitelist (market/index/stock/sector/policy/data/event/institution/specific person) — user decision: no post-processing code, prompt-only enforcement. Verified 0 violations in round 7 → added during build

REQ-7 — Title theme answering (additive)
  The `## 主要觀點` section shall do two things together: distill core positions AND directly answer or explain the question, promise or theme raised by the video title (additive, not either/or). Question titles open with the direct answer/conclusion; promise/theme titles state the conclusion the subject arrives at → Acceptance (new, intent 2026-11-07-key-viewpoints-answer-title)

REQ-8 — Dynamic mermaid diagram limits
  Diagram count is capped by transcription length tier (<500: 1–2, <3000: 2–3, <10000: 3–4, ≥10000: 4–5). One diagram is mandatory for the leading `## 主要觀點` section (as an architecture overview, without a `####` heading to avoid duplicating the H2), remaining diagrams selected only from `###` chapters, never one per chapter. The limit text is language-aware (zh/ja/en) and injected via the `{{mermaid_limit}}` template placeholder (prepending the text was ignored by the model) → added during build

REQ-9 — Three-language sync
  The article-style templates for zh-Hant, en and ja carry the same rules (C-mode, meta-narration ban, title answering, dynamic mermaid limits) → added during build

## Design decision

- Rewrite the built-in article templates (`prompts/builtin/summary-article-{zh-Hant,en,ja}.md`) into the viewpoint-first narrative format, and set `summary.style: article` in the user's `config.yaml` (and document it in `config.example.yaml`). agent-decided — reuses the existing 4-level prompt resolution and the existing `style` field with zero new mechanism; the outline style stays available by switching `style` back.
- Document skeleton: `## 主要觀點` first, then the article body as `###`-headed chapters in original narrative order. agent-decided during build — the mermaid stage matches `###` chapters to insert diagrams; the viewpoint section's own mandatory diagram carries no heading.
- Prose-first: the body is continuous paragraphs; Markdown lists or tables appear only where the source content itself is list-like or comparative. From the user's carried detail "將影片內容逐字稿文章化" and "相對保留細節與原始影片的敘述順序".
- Length: measured before raising any limit. Keep `summary.max_tokens` at its current value initially, run one real video of 20+ minutes through the batch flow, and only raise the limit if the article truncates. Verified non-truncated across 7 end-to-end rounds. agent-decided — measurement-first.
- Meta-narration enforcement: prompt-only positive subject whitelist + rewrite table; no code-side output post-processing (user decision 2026-11-07 — maintainability).
- Dynamic mermaid limits: injected through the `{{mermaid_limit}}` placeholder inside the template body, substituted at Stage 3 by `MermaidPrompt(summary, language, transcriptionLength)`; prepending the limit text before the prompt was tried first and ignored by the model.
- No new config fields beyond the existing `style` field, no changes to the keyword (stage 2) stage, TUI/CLI surface, or Obsidian copy behavior.

## Alternatives considered

- Add a new built-in style name (e.g. `narrative`) with its own template — rejected: adds a resolution path and config surface for what is a content change of the existing article style.
- Replace the outline built-in in place — rejected: destroys the existing outline format the user may switch back to; the intent keeps the format change opt-in via the style field.
- Generate the viewpoint section in a separate LLM stage — rejected: adds a fourth call and cost; one stage-1 call can produce both sections with the viewpoint-first instruction.
- Detect truncation only at write time and warn — rejected: the intent and PRINCIPLES require the run to not write a truncated file as success; prevention (size guidance in the prompt + token limit) plus fail-loud on residual truncation.
- Enforce the meta-narration ban with banned-word lists — rejected after 4 rounds of synonym bypass by the model (4→3→1 violations); the positive sentence-subject whitelist converged to 0.
- Post-process the summary output in code to strip meta-narration — rejected by user decision (maintainability): prompt-only enforcement.
- Prepend the diagram-limit text to the Stage 3 prompt — rejected: model ignored it (output exceeded the cap); template-placeholder injection worked.

## Current state evidence

- Forward: `pipeline/pipeline.go` stage 1 (~line 845) resolves the prompt via `summarizer.ResolvePrompt` and substitutes variables before the first LLM call; Stage 3 (~line 888) calls `summarizer.MermaidPrompt(summaryText, language, transcriptionLength)` which substitutes `{{mermaid_limit}}`.
- Reverse: `summarizer/prompt.go` `loadBuiltinPrompt` maps `style == "article"` to the `summary-article-*` template; `config.SummaryConfig.Style` at `config/config.go:229`.
- Data: summary body today is `## 主要觀點` (H2) followed by `###` chapters; frontmatter built by `output.BuildSummaryFrontmatter` (`output/frontmatter.go:66`).
- Boundary: keyword (stage 2) and TUI/CLI surfaces are unchanged; the change stops at the article templates, the mermaid templates, `MermaidPrompt`/`MermaidLimitText`, the two Stage 3 call sites in the pipeline, and the style selection in config.

## UI flows

Run the summarizer (single video or batch) → the summary file appears in the usual place; opening it, the first section after the frontmatter is `## 主要觀點` stating the speaker's viewpoints and directly answering the title's question/theme, then the article body in `###` chapters following the video's original narrative order, with numbers, names and cases kept in the prose, plus a bounded number of Mermaid diagrams (1 mandatory overview + a few chapter diagrams, capped by transcription length).

- Success: 執行批次或單支影片 → 摘要檔照常產生；frontmatter 之後第一節是「## 主要觀點」，接著是按原敘述順序的「###」章節文章，數字與人名留在句子裡；流程圖數量受轉錄字數上限約束；關鍵字照舊。
- In progress: 處理中 → 現行進度與階段記錄照舊，不新增等待畫面或互動。
- Error: 若文章生成失敗或明顯截斷 → 該影片以明確錯誤標記失敗，不把截斷的摘要檔當成功寫出；恢復方式：檢查輸出上限設定或 provider 後重跑該影片。
- Empty: 影片沒有可用的字幕或轉錄 → 照現行行為略過並記錄，本變更不改變。
- Resume: 重跑一支已有摘要的影片 → 照現行行為跳過，既有檔案內容不變。
