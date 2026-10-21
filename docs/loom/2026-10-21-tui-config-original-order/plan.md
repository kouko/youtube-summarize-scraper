# TUI config display in original order — plan
intent: 2026-10-21-tui-config-original-order@3dbd652
spec: docs/loom/2026-10-21-tui-config-original-order/spec.md@21b1eae
charter: 1.1

## Task DAG
<!-- When a spec requirement changes after this commit, the un-landed
     tasks it touches are replaced and the reason is named in the commit
     message. Landed tasks stay as they are. -->

Wave 1 — remove the TUI display shuffle and clean up the config files.

**W1-01 Remove TUI config display shuffle and stray config line**  after: none  acceptance: 1, 2, 3
- Files: tui/config.go, tui/config_test.go, config.yaml
- Test: A1 positive: sections in file order; boundary: random_order key renders normally; A2 positive: SetValue+Serialized keeps order/comments; boundary: key survives write-back; A3 positive: config.yaml no top-level key; boundary: batch.random_order intact.
- Risk: parse() must stop mutating cv.doc (it is the write-back target); remove shuffleMapping, extractRandomOrderFlag, randomOrder field, init() rand seed, math/rand+time imports (spec REQ-1/REQ-2). Vault config line removal is a user-decided one-time data edit outside the repo, disclosed in the acceptance report.

## Simplicity check
- none found

## Questions asked
capture-intent ① — behaviour — 拿掉 TUI 設定畫面的隨機排序功能，讓設定畫面完全依照設定檔的原始排列顯示；同時修好「在 TUI 編輯儲存會弄亂設定檔」的問題。對嗎？ (answer: 對，但「我要的隨機只有處理頻道/播放清單實際列出影片/處理影片的順序，而不是設定檔在 TUI 上的顯示順序」)
capture-intent ① — behaviour — 移除時要不要一併把先前加到設定檔（含你實際用的那份）的最上層 random_order: true 那行清掉？ (answer: 要)
write-spec ② — behaviour — 載入設定檔→Config 面板依檔案原始順序；編輯並儲存→順序與註解保持原樣；跑批次→處理順序仍由 batch.random_order 控制。對嗎？ (answer: 等等，為何要移除 random_order: true 這個 key...)
write-spec ② — consequence — 會改到你的實際設定檔，移除最上層那行 random_order: true，其餘內容與順序不動。確認嗎？ (answer: 要)

## Risks
1. Removing the shuffle also removes the write-back corruption, which is the intended fix; existing order/write-back tests already cover configs without the key, and a new regression test covers configs with it (spec REQ-1/REQ-2).
2. user-decided — the top-level `random_order: true` line is removed from the repo config.yaml and the user's vault config; `batch.random_order` is untouched.
