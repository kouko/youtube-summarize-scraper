# 文章式摘要：觀點章節優先，正文依原始敘事順序保留細節
originator: kouko
kind: product
needs-design: yes — summary.md 是使用者會在 Obsidian 閱讀的檔案產物，目前沒有 DESIGN.md 或 ui-flows 涵蓋摘要格式
status: confirmed 2026-11-06
intent_confirmed_sha: 8a621d3 # written at confirmation: `git hash-object` over this file WITHOUT its status: line and WITHOUT this line itself
publication: automatic — authorized 2026-11-06 by kouko

## Problem

每天批次處理的影片摘要目前是「概述＋章節條列＋重點整理」的大綱體：影片被拆成零散要點，原始敘述順序與具體細節被壓縮。讀者想回顧講者完整的論述脈絡時，摘要無法還原，也無法在讀正文前先掌握講者自己的觀點。每篇新摘要都持續以這個不適合閱讀的格式產出，越積越多。

## Proposed outcome

影片摘要改為「文章式」：開頭先有一節呈現講者的觀點與論述，正文則依影片原始敘述順序寫成保留細節的連貫文章；輸出長度經過真實長片實測，確認完整不截斷。

## Acceptance

1. 新產生的摘要，正文以「影片觀點」章節開頭（在 frontmatter 之後），內容陳述講者的觀點與論述，而不是逐段內容回顧。
2. 摘要正文為連貫文章，段落順序與影片原始敘述順序一致；影片中的具體細節（數字、金額、日期、人名、案例、引述）保留在文中，未被概括為模糊描述。
3. 以至少一支 20 分鐘以上的長片實測，輸出的文章完整結束、未被截斷（無中途斷句或截斷記號），批次流程正常完成。
4. 已產生的既有摘要檔不被改寫（以檔案內容比對確認不變）。

## Constraints

- 輸出語言維持繁體中文（現行設定）。
- 無人值守批次執行必須照常完成，不得要求互動輸入。
- 若輸出發生截斷或生成失敗，流程必須明確回報，不可靜默通過。
- 既有 CLI、批次流程與 Obsidian 輸出位置不變。

## Value case

受益者：每天閱讀這些摘要的本人。急迫性：每天批次都持續產出不合閱讀格式的新摘要，格式問題隨時間累積。現有替代方案：重看影片（耗時）、現有文章式模板（接近但缺少「觀點」章節與保留原始敘事順序的明確要求）。本輪不做其他摘要相關改進（關鍵字品質、圖表密度）。GO：每天都被影響、改造成本可控、且已有相近模板可演化。

## Out of scope

- 關鍵字（frontmatter tags）的內容、語言與數量
- Mermaid 流程圖的數量、樣式與插入行為
- 簡體轉繁體的強制校正機制
- 其他語言（en、ja）的內建摘要模板
- 既有摘要檔的重生成或回填
- TUI、CLI 與其他介面的變更

## Open questions

- none
