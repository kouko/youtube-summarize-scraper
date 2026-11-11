# Fix video ID extraction for IDs containing "__"

originator: kouko
kind: engineering
needs-design: no — 既有設計文件 docs/loom/specs/ytss-design.md 的輸出結構章節已涵蓋輸出資料夾命名規則（DATE__videoID__title）；本修正不改變格式，只修正解析邏輯
status: confirmed 2026-11-11
intent_confirmed_sha: 581832dd05331c34bae09227b9fdbe068458a911
publication: automatic — authorized 2026-11-11 by kouko

## Problem
輸出資料夾命名格式為 `DATE__videoID__title`（以雙底線 `__` 分隔三段）。目前 `output/index.go` 的 `extractVideoID` 使用 `strings.SplitN(dirName, "__", 3)` 切分，當 video ID 本身包含 `__`（例如 `tp_F__iRC04`）時，會被切成 `["2023-12-05", "tp_F", "iRC04__課程前導直播..."]`，取到錯誤的 ID `tp_F`。導致索引查找失敗，`IsProcessed` 永遠回傳 false，使得該影片每次掃描都被重複完整處理（字幕→轉錄→LLM），造成資源浪費。

## Proposed outcome
`extractVideoID` 正確從資料夾名稱提取 video ID，不論 video ID 是否包含 `__`。已處理影片能被正確識別並跳過，不再重複處理。

## Acceptance
1. 建立測試資料夾 `2023-12-05__tp_F__iRC04__課程前導直播...`，`extractVideoID` 回傳 `tp_F__iRC04`。
2. 一般 ID（不含 `__`）如 `2026-11-08__wBlVuuHtRSw__標題` 仍回傳正確 `wBlVuuHtRSw`。
3. `IsProcessed("MacroMicro_tw", "tp_F__iRC04")` 在對應資料夾存在且有 summary.md 時回傳 true。
4. 現有測試全部通過，不改變輸出格式或資料夾結構。

## Constraints
- 不改變輸出資料夾命名格式（仍為 `DATE__videoID__title`）。
- 不改變其他模組對 `extractVideoID` 的呼叫簽名。
- 保持向後相容：現有已建立的資料夾仍能被正確識別。

## Out of scope
- 修正其他解析函式（若有）的類似問題。
- 清理已產生的重複處理成品。
- 處理 video ID 含其他特殊字元的情況（僅針對 `__` 分隔符號衝突）。

## Open questions
- none