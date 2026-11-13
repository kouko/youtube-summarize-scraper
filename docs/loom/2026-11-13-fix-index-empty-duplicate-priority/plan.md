# Fix video index losing to empty older duplicate directory — plan

intent: 2026-11-13-fix-index-empty-duplicate-priority@91e713c2900394c61e5a9b3bb23ccabdade48c0c
charter: 1.1

## Current State Evidence
- Forward: `output/index.go:79` `BuildIndex` 無條件覆寫 `idx.entries[videoID]`；重複資料夾時依 `os.ReadDir` 排序最後者勝出。
- Reverse: `ytss-output/_playlist__PL8-ulaGCrHSWz3nf0SgqWakeFg29SB4vS__.../` 下 `xJ__cqXlboc` 有 22 個重複資料夾，排序最後的 `2022-08-18__...` 是空殼。
- Error: `ytss-output/logs/ytss.log` 顯示 `xJ__cqXlboc` 於 2026-11-13 被完整重跑 12 次；`2021-11-13__xJ__cqXlboc__summary.md` 存在卻未被索引命中。
- Data: `output/index_test.go` 已覆蓋多頻道、playlist、空目錄、skip marker、含 `__` 的 ID；無重複資料夾情境測試。
- Boundary: `pipeline/pipeline.go:1067-1084` 用 `FindVideoDir`+`HasFile(summary.md)` 跳過已處理影片；單一資料夾、`.skipped`、動態 `Add`/`AddFile` 不得回歸。

## Task DAG
**W1-01 BuildIndex prefer completed duplicate directory**  after:  acceptance: 1,2,3,4
- Files: `output/index.go`, `output/index_test.go`
- Test: A1 positive: summaryBeatsEmptyShell; boundary: emptyShellSortsLast. A2 positive: allEmptyReturnsSomeDir; boundary: allEmptyPicksFirstSorted. A3 positive: xjLayoutResolvesSummary; negative: emptyShellNotChosen. A4 positive: fullSuiteGreen; boundary: singleDirUnchanged.
- Risk: agent-decided: 以分數取代覆寫（summary/.skipped=2、任一檔案=1、空=0），同分保留先序資料夾；無呼叫端依賴舊的 last-wins；`Add` 不動。

**W1-02 Keep duplicate candidates, channel-aware IsProcessed, summary ranks above .skipped**  after: W1-01  acceptance: 1,3,4
- Files: `output/index.go`, `output/index_test.go`
- Test: A1 positive: crossChannelSummaryFound (summary in @aaa and @bbb, IsProcessed(bbb) true); negative: skippedShadowedBySummary. A3 positive: xjLayoutResolvesSummary; negative: statDeletedWinnerFallsBackToNextCandidate. A4 positive: fullSuiteGreen; boundary: singleDirUnchanged.
- Risk: agent-decided: `entries[videoID]` 改為候選清單，FindVideoDir 依分數取最佳者（stat 失敗時遞補下一個候選）；IsProcessed 在指定頻道內任一候選有 summary/.skipped 即 true；分數改為 summary=3 > .skipped=2 > 任一檔案=1 > 空=0（符合 intent「優先選有 summary」）。Adversary 實證：舊 last-wins 在跨頻道同分時讓後排序頻道 IsProcessed 為 true，新邏輯須保持此語意，避免每輪重跑且永不收斂。

## Simplicity check
- 掃描時自動刪除空殼資料夾 — declined: 動到使用者既有資料，超出 intent Out of scope
- 只改索引選擇規則（本方案） — taken
- 對手機發現：跨頻道重複資料夾需索引保留所有候選 — declined: 不在本 intent scope；改以 W1-02 修正（見 Risks 4）

## Questions asked
① — what — 這個修復方案（索引優先選有 summary 的資料夾）確認可以嗎？

## Risks
1. 變更僅限 `BuildIndex`/索引查詢；既有測試皆用唯一 video ID，A4 全數通過即回歸防護。
2. 無 ARCHITECTURE.md（standing WARN 已記錄），無架構規則適用。
3. 既有 94 支影片的重複資料夾不清理（intent Out of scope）；修正後索引忽略空殼並停止新增重複資料夾。
4. Adversary 2026-11-13 發現跨頻道 tie 與 .skipped 遮蔽 summary 兩項；以 W1-02 修正（channel-aware 候選清單 + summary 優先於 .skipped）。
