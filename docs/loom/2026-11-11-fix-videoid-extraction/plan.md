# Fix video ID extraction for IDs containing "__" — plan

intent: 2026-11-11-fix-videoid-extraction@a0367f98d0b296b57007be93206b7705190874ed
charter: 1.1

## Current State Evidence
- **Forward:** `output/index.go:87-95` `extractVideoID` 使用 `strings.SplitN(dirName, "__", 3)`，取 `parts[1]`。當 video ID 含 `__`（如 `tp_F__iRC04`），資料夾名稱 `2023-12-05__tp_F__iRC04__標題` 被切成三段，取到 `tp_F` 而非完整 ID。
- **Reverse:** `BuildIndex` 將錯誤的 `tp_F` 存入索引；下游 `IsProcessed("MacroMicro_tw", "tp_F__iRC04")` 查不到 → 回傳 false → 影片被重複完整處理（字幕→轉錄→LLM）。
- **Error:** 真實案例：`@MacroMicro_tw` 的直播 ID `tp_F__iRC04`，每次掃描都重新處理，每輪約 3–4 分鐘；今天已累積 13 次重複處理。
- **Data:** 測試案例已覆蓋正常 ID、`__` 含 ID、playlist dir、無分隔符 dir、skip marker 等。
- **Boundary:** `FindVideoDir`、`HasFile`、`IsProcessed` 都依賴 `extractVideoID` 解析結果；修正後必須保持向後相容（既有資料夾仍可被識別）。

## Task DAG
**W1-01 — 修正 extractVideoID 實作**  after: acceptance: 1,2,3,4
- **Files:** `output/index.go`（修正解析邏輯）、`output/index_test.go`（新增測試）
- **Test:** A1 positive: video ID 含 `__` 時 `extractVideoID` 回傳正確 ID；A2 positive: 一般 ID 仍正確；A3 positive: `BuildIndex` → `IsProcessed` 正確回傳 true；A4 positive: 既有測試全部通過。
- **Risk:** 1. 解析邏輯變更可能影響既有資料夾識別（已用單元測試覆蓋正常/邊界情況）。2. video ID 含多重 `__`（如 `a__b__c`）需取第一個 `__` 到最後一個 `__` 之間的完整字串（已在測試驗證）。

## Questions asked
① — what — 複雜度檢查：是否需加入正規表示式或更複雜解析？（agent-decided：用 first/last index 即可，簡單且線性）
① — what — 是否需修正其他地方同類解析？（agent-decided：目前僅 `extractVideoID` 一處；grep 確認無其他同類模式）

## Risks
1. 既有資料夾名稱格式若不完全符合 `DATE__videoID__title`（如缺段），新邏輯可能行為不同（已有 `TestBuildIndex_InvalidDirName`、`no separator`、`only one __` 測試覆蓋邊界）。
2. playlist 目錄名稱 `_playlist__WL__Watch_Later` 不是影片目錄，不經 `extractVideoID` 處理（在 `BuildIndex` 的第一層循環過濾），無影響。

## Acceptance (mapped to intent)
1. 建立測試資料夾 `2023-12-05__tp_F__iRC04__課程前導直播...`，`extractVideoID` 回傳 `tp_F__iRC04`。
2. 一般 ID（不含 `__`）如 `2026-11-08__wBlVuuHtRSw__標題` 仍回傳正確 `wBlVuuHtRSw`。
3. `IsProcessed("MacroMicro_tw", "tp_F__iRC04")` 在對應資料夾存在且有 summary.md 時回傳 true。
4. 現有測試全部通過，不改變輸出格式或資料夾結構。