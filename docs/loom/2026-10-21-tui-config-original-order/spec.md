# TUI config display in original order — spec
intent: 2026-10-21-tui-config-original-order@3dbd652
confirmed-behavior: 2026-10-22 @f5c2754
pre-build-review: not-required — removes a small display-only feature and its two config-file lines; no security, privacy, public-contract, cross-system, or irreversible-data surface, and the requirements are unambiguous.

## Requirements
REQ-1 — TUI config display follows file order
  WHEN the TUI renders the config panel, the top-level sections shall appear in the same order as the config file (no random shuffle) → Acceptance #1
REQ-2 — Config save preserves the file
  WHEN the user edits a config value in the TUI and saves, the written config file shall keep its original top-level order and comments → Acceptance #2
REQ-3 — No top-level random-order switch; batch shuffle unchanged
  The config shall not carry a top-level random-order switch (the user's request: "我要的隨機只有處理頻道/播放清單實際列出影片/處理影片的順序 而不是設定檔在 TUI 上的顯示順序"), and `batch.random_order` processing-order behaviour shall remain unchanged → Acceptance #3

## Design decision
- Remove the TUI display shuffle entirely: delete `shuffleMapping`, `extractRandomOrderFlag`, the `randomOrder` field, the `init()` rand seed, and the `math/rand`/`time` imports from `tui/config.go`, and drop the flag check from `parse`. agent-decided — the intent removes the feature rather than preserving it, so there is nothing to keep.
- Keep `cv.doc` as the unmodified parsed document (it is both the display source and the write-back target, so it must never be mutated by display concerns). agent-decided — removing the shuffle removes the mutation, which is what fixes REQ-2.
- Remove the top-level `random_order: true` line from `config.yaml` and from `~/kouko-obsidian-vault/_config/ytss-config.yaml` (added for the removed feature). agent-decided — the key is now unused for the TUI and would otherwise render as a stray row; `batch.random_order` is a different, retained key.
- Do not change `batch.random_order` or `pipeline.ProcessBatchStreaming`. agent-decided — out of scope; the user's only wanted randomness is processing order.

## Alternatives considered
- Keep the shuffle but fix the write-back (shuffle a display-only copy): rejected — the user stated the TUI display random order is meaningless, so the feature should not exist.
- Keep the feature behind a default-off flag: rejected — leaves dead code and a meaningless surface.
- Just stop mutating `cv.doc` but keep rendering shuffled: rejected — same as the first alternative; the user wants the original order.

## Current state evidence
- Forward: TUI loads a config → `handleConfigSelected` (tui/model.go) → `NewConfigView` (tui/config.go:123) → `parse` (tui/config.go:136) → `extractRandomOrderFlag` + `shuffleMapping` → `walkMapping` renders rows.
- Reverse: `NewConfigView` callers (tui/model.go:config load and post-edit reload); `Serialized` (tui/config.go:594) → `commitEdit` (tui/model.go:1129) → `os.WriteFile` (tui/model.go:1138).
- Error: `extractRandomOrderFlag` treats any non-empty value as true (tui/config.go:107), so `random_order: false` also shuffles; and the shuffle + flag removal mutate `cv.doc`, so the post-edit `Serialized()` write reorders the file and drops the key.
- Data: `ConfigView.doc *yaml.Node` is both the display source and the write-back target (tui/config.go:141); `SetValue` edits it (tui/config.go:576); `Serialized` marshals it (tui/config.go:594).
- Boundary: this change removes the TUI display shuffle and the two top-level `random_order` lines only; `config.BatchConfig.RandomOrder` (config/config.go:140) and `pipeline.ProcessBatchStreaming` (pipeline/pipeline.go:199) are untouched.

## UI flows
Surface: Config panel (bottom-left) and its inline value editor.

- Open the TUI and select a config file → the Config panel lists top-level sections in the file's own order (same order every launch).
- Press Enter on a value row, type, press Enter to save → the file keeps its original section order and comments; the panel redraws in the same order.
- Start a pipeline run → the order in which channels/playlists are fetched and their videos processed still follows `batch.random_order` (unchanged).
- Empty state → "(no config selected)" before a file is chosen (unchanged).
- Error state → a malformed YAML file shows its parse error text (unchanged).
