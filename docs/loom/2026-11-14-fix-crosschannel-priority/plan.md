# Cross-channel duplicate priority and .skipped vs summary ranking — plan

intent: 2026-11-14-fix-crosschannel-priority@5d52c8691304a98461732a2d2815967b1c70d015
charter: 1.1

## Current State Evidence
- Forward: `output/index.go:79-80` W1-01 已實作單一 video ID 的分數選優（summary/.skipped=2、有檔案=1、empty=0），first-wins tie-break。`entries[videoID]` 為單一 entry。
- Reverse: `pipeline/pipeline.go:664` `IsProcessed(channelHandle, id)` 只檢查單一全域 entry 的目錄前綴，跨頻道時若全域 entry 非該頻道則回傳 false。同頻道 `.skipped` 與 `summary.md` 同分 2，視排序而定。
- Error: Adversary 實驗：跨頻道 @aaa/@bbb 皆有 `summary.md`，`IsProcessed(@bbb)` false → 每輪重跑且永不收斂。同頻道 `.skipped` 排序在前時 `HasFile(summary.md)` false → 系統誤判需重新處理。
- Data: `output/index_test.go` 已有 `TestBuildIndex_DuplicateDirs`（7 子測試）。無跨頻道與 `.skipped` vs `summary.md` 的測試案例。
- Boundary: `FindVideoDir`、`HasFile`、`IsProcessed`、`Add`、`AddFile`、`StatCheck` 皆依賴 `entries[videoID]`。動態更新（`Add`/`AddFile`）在處理過程中即時更新檔案標記，需保留此行為。

## Task DAG
**W2-01 Channel-aware duplicate candidates in BuildIndex**  after:  acceptance: 1,2,3,4
- Files: `output/index.go`, `output/index_test.go`
- Test: A1 positive: crossChannelSummaryBothTrue; boundary: crossChannelEmptyVsSummary. A2 positive: skippedShadowedBySummary; boundary: summaryRanksAboveSkipped. A3 positive: singleDirUnchanged; boundary: dynamicAddFileUpdates. A4 positive: fullSuiteGreen; boundary: gofmtClean.
- Risk: agent-decided: entries 改為候選清單 map[string][]*VideoEntry，查找依 channel handle 篩選；無 channel 時回退最高分；Add/AddFile 更新匹配候選。

**W2-02 Fix stale lastAdded fabricating terminal marks after stat-drop**  after: W2-01  acceptance: 3,4
- Files: `output/index.go`, `output/index_test.go`
- Test: A3 positive: lastAddedClearedOnDrop; negative: addFileAfterStatDropMarksNothing. A4 positive: fullSuiteGreen; boundary: gofmtClean.
- Risk: agent-decided: FindVideoDir 丟棄候選時同步清除指向該目錄的 lastAdded；AddFile 在 lastAdded 已記錄但候選不存在時 return 而非回退 bestEntry（回退僅限從未 Add 的影片）。Adversary 評級 nit、pipeline 現行呼叫順序不可達，修復屬防禦性正確性。

## Simplicity check
- 完全保留所有重複資料夾不做選擇 — declined: 記憶體成長，FindVideoDir 需要線性掃描
- 改為 channel-aware 索引（本方案） — taken

## Questions asked
① — what — 確認啟動 W2-01 跨頻道與 skipped-sort？

## Risks
1. 結構變更 `entries` 為切片影響所有查找路徑；完整套件測試 (A4) 為回歸防護。
2. 動態 `Add`/`AddFile` 在候選清單中尋找匹配目錄更新標記；需確認不破壞 resume。
3. 現有 94 支重複資料夾影片不清理（Out of scope）；修正後索引正確跳過，不再產生新重複。
4. 無 ARCHITECTURE.md，無架構規則適用。