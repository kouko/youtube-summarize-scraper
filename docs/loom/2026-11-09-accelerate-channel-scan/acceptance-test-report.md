# 加速頻道掃描 — 我試了什麼、發生什麼

Tried on 2026-11-09, in a clean copy of the project at 33d70eb. 每一條驗收條件我怎麼試、指令與輸出，都在 `docs/loom/2026-11-09-accelerate-channel-scan/evidence/acceptance-test-evidence.md`。

## 你要求的事，一行一行來

| # | 你要求的事 | Verdict | 發生什麼 | Re-run |
|---|---|---|---|---|
| 1 | 對一個清單內影片全部已處理的頻道做掃描或 dry-run，整個過程不對任何影片發出逐支的完整 metadata 抓取呼叫（執行日誌中不出現逐支的「fetching full metadata」），且所有影片回報為已跳過。 | works | 全部已處理的頻道，掃描與 dry-run 兩種模式都是全程零次逐支 metadata 抓取，3 部影片全部回報「已跳過」，日誌中確實沒有「fetching full metadata」。 | re-tested |
| 2 | 掃描中仍存在的（未處理的）新影片，行為與現狀一致：補抓完整 metadata 後照現有流程產出字幕、逐字稿與摘要。 | works | 新影片確實先補抓一次完整 metadata，接著照既有流程依序產出字幕檔、逐字稿與摘要檔，與改動前行為一致。 | re-tested |
| 3 | dry-run 完成後不產生任何 yt-dlp 的完整 metadata fetch 紀錄，且輸出只顯示「would process (dry run)」而不額外進行任何抓取。 | works | dry-run 全程零次抓取（連 DEBUG 層級的日誌都沒有），只輸出 3 行「would process (dry run)」，結束後輸出資料夾完全沒有建立任何東西。 | re-tested |
| 4 | 頻道的 handle 與輸出資料夾名稱不一致時（例如頻道改過 handle），不出錯、不重複產出：最壞情況退回現狀流程（檢查失敗 → 照現狀補抓 → 後置檢查保底）。 | works | handle 不一致時沒有報錯、沒有重複產出；最壞情況確實照現狀退回：前置檢查未命中 → 補抓一次完整 metadata → 後置檢查判定已處理並跳過。 | re-tested |

每一列的判定依據是對應的已提交測試（pipeline 套件的 `TestProcessVideo_EarlySkip` 與 `TestExtractHandleFromURL`，先在改動前版本跑出失敗、再在本次改動上跑出通過）加上我在乾淨環境用假抓取器對真實管線的獨立試跑；完整自動化測試套件會在變更被接受前執行、失敗即擋下。

## 對你既有的資料做了什麼

Nothing — 此變更只改變處理管線最前方的讀取與判斷順序：已處理影片現在會更早被判定跳過，它們的資料夾與檔案不會被寫入或修改；新影片仍照既有格式寫入同一個輸出結構。你既有的輸出保持原樣、可繼續讀取，沒有備份需求。

## 我幫你做了決定

- **`--force` 時不做新的前置跳過** — 我沿用既有 `--force` 的語意（強制重跑必須真的重跑），所以「強制重跑」仍會照舊抓取並重做；若新檢查連 `--force` 都擋，會破壞這個旗標的意義。以後想改變，需要動 `--force` 的行為。
- **handle 只從 config 網址裡的 `@handle` 解析** — 網址不含 `@handle`（如 `/channel/UC…`、`/c/…`、`/user/…`）的頻道會直接跳過新前置檢查、維持原本流程。這與你確認過的方案一致（前置檢查的 handle 固定來自 config 網址），也寫在你確認過的規劃風險裡，沒有行為退化。

## 你叫我略過的步驟

Nothing — 沒有任何步驟被你指示略過。

## 我不確定你還要什麼

Nothing。
