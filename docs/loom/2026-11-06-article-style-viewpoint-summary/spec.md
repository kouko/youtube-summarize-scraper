# Article-style summary with a viewpoint-first section — spec
intent: 2026-11-06-article-style-viewpoint-summary@548bb47
pre-build-review: not-required — prompt-content change routed through the existing 4-level prompt resolution; no security or privacy surface, no irreversible action on existing files (Acceptance #4 pins them untouched), no public API contract, requirements confirmed by the user

## Requirements

REQ-1 — Viewpoint section leads the summary body
  WHEN a video is summarized, the system shall produce a summary whose body begins, immediately after the frontmatter, with a `### 影片觀點` section that states the speaker's viewpoints and argumentation rather than a segment-by-segment recap → Acceptance #1

REQ-2 — Article body preserves original narrative order and details
  WHEN the viewpoint section ends, the system shall continue the summary as a continuous article divided into `####` sections whose order follows the video's original narrative order, and the prose shall retain specific facts (numbers, amounts, dates, names, cases, direct quotes) instead of summarizing them away → Acceptance #2

REQ-3 — Long-video articles complete without truncation
  WHEN a video of 20 minutes or longer is summarized, the system shall produce an article that ends completely (no mid-sentence cut or truncation marker), and the batch run shall complete normally → Acceptance #3

REQ-4 — Existing summary files are left untouched
  The system shall not modify any summary file that exists before this change; the new format applies only to videos processed after the change → Acceptance #4

## Design decision

- Rewrite the built-in zh-Hant article template (`prompts/builtin/summary-article-zh-Hant.md`) into the viewpoint-first narrative format, and set `summary.style: article` in the user's `config.yaml` (and document it in `config.example.yaml`). agent-decided — reuses the existing 4-level prompt resolution and the existing `style` field with zero new mechanism; the outline style stays available by switching `style` back.
- Document skeleton: `### 影片觀點` first, then the article body as `####`-headed sections in original narrative order. agent-decided — the mermaid stage already matches `####` headings to insert diagrams, so its behavior keeps working unchanged; the heading hierarchy matches the existing outline convention.
- Prose-first: the body is continuous paragraphs; Markdown lists or tables appear only where the source content itself is list-like or comparative. From the user's carried detail "將影片內容逐字稿文章化" and "相對保留細節與原始影片的敘述順序".
- Length: measure before raising any limit. Keep `summary.max_tokens` at its current value initially, run one real video of 20+ minutes through the batch flow, and only raise the limit if the article truncates. If a provider ignores the token limit (CLI providers pass no output cap), verify actual behavior on the sample; any truncation the pipeline cannot prevent must fail the video loudly rather than write a truncated file as success — pinned by PRINCIPLES.md non-negotiable 1 ("Never fail silently"). agent-decided — measurement-first, per the user's stated uncertainty ("可能要試試看才知道").
- No new config fields, no changes to the keyword (stage 2) or mermaid (stage 3) stages, TUI/CLI surface, or Obsidian copy behavior.

## Alternatives considered

- Add a new built-in style name (e.g. `narrative`) with its own template — rejected: adds a resolution path and config surface for what is a content change of the existing article style.
- Replace the outline built-in in place — rejected: destroys the existing outline format the user may switch back to; the intent keeps the format change opt-in via the style field.
- Generate the viewpoint section in a separate LLM stage — rejected: adds a fourth call and cost; one stage-1 call can produce both sections with the viewpoint-first instruction.
- Detect truncation only at write time and warn — rejected: the intent and PRINCIPLES require the run to not write a truncated file as success; prevention (size guidance in the prompt + token limit) plus fail-loud on residual truncation.

## Current state evidence

- Forward: `pipeline/pipeline.go` stage 1 (~line 845) resolves the prompt via `summarizer.ResolvePrompt` and substitutes variables before the first LLM call.
- Reverse: `summarizer/prompt.go` `loadBuiltinPrompt` maps `style == "article"` to the `summary-article-*` template; `config.SummaryConfig.Style` at `config/config.go:229`.
- Error: an empty stage-1 response fails via `emptyResponseError` (`pipeline/pipeline.go` ~852); truncation today is not detected — the outline output usually fits.
- Data: summary body today is `### 概述` / `### 章節摘要` (`####` chapters) / `### 重點整理`; frontmatter built by `output.BuildSummaryFrontmatter` (`output/frontmatter.go:66`).
- Boundary: keyword (stage 2) and mermaid (stage 3) stages, Obsidian copy, and TUI/CLI surfaces are unchanged; the change stops at the zh-Hant article template and the style selection in config.

## UI flows

Run the summarizer (single video or batch) → the summary file appears in the usual place; opening it, the first section after the frontmatter is `### 影片觀點` stating the speaker's viewpoints, then the article body in `####` sections following the video's original narrative order, with numbers, names and cases kept in the prose. Mermaid diagrams and keyword tags continue to appear as they do today.

- Success: 執行批次或單支影片 → 摘要檔照常產生；frontmatter 之後第一節是「### 影片觀點」，接著是按原敘述順序的「####」章節文章，數字與人名留在句子裡；流程圖與關鍵字照舊。
- In progress: 處理中 → 現行進度與階段記錄照舊，不新增等待畫面或互動。
- Error: 若文章生成失敗或明顯截斷 → 該影片以明確錯誤標記失敗，不把截斷的摘要檔當成功寫出；恢復方式：檢查輸出上限設定或 provider 後重跑該影片。
- Empty: 影片沒有可用的字幕或轉錄 → 照現行行為略過並記錄，本變更不改變。
- Resume: 重跑一支已有摘要的影片 → 照現行行為跳過，既有檔案內容不變。
