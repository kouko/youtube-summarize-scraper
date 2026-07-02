# Brief: 區網多台 LM Studio 的 HA 支援（openai-compat 多實例，R2 置換）

> Brainstorming 在對話中完成，本檔為其結構化結論（5 軸）。
> 設計幾經迭代，最終定為 **R2：直接把 `openai-compat` 改為多實例 map**（破壞向後相容，使用者已同意）。

## Problem

區網上可能同時跑多台 LM Studio server（不同主機 / IP，各自一個 OpenAI 相容端點）。
現有結構中一個 provider 名稱只對應一份設定（`cfg.OpenAICompat` 是單一 struct），
所以無法同時定址兩個以上的 openai-compat 端點，也就無法做「主力一台、掛了換下一台」的故障轉移。

## Users

自架本機 / 區網 LLM 的操作者，想用多台 LM Studio（或任何 OpenAI 相容 server）取得高可用，而非單點。

## Smallest End State

- `llm.openai-compat` 本身由「單一 struct」改為 **map（name → OpenAICompatConfig）**。
- provider 引用語法：
  - bare `openai-compat` → 解為實例名 `default`。
  - `openai-compat:<name>` → 解為具名實例。
- HA 完全重用既有 `FallbackSummarizer` + 每台獨立 circuit breaker（breaker 以 name 為 key，天然隔離）；
  不寫新調度邏輯。例：`provider: ["openai-compat:box1", "openai-compat:box2"]`。

## Decisions（已定案）

1. **方案 R2（置換，非附加）**：`openai-compat` struct → map。**破壞向後相容**——既有
   `openai-compat: { endpoint: ... }` 需遷移為 `openai-compat: { default: { endpoint: ... } }`。
   使用者明確同意破壞。
2. **命名語法**：bare `openai-compat` = `default` 實例；`openai-compat:<name>` = 具名實例。
   config key 與 provider 前綴一字不差地對齊，無新名稱、無重複 key、無 misnomer。
   - 實例名由使用者自訂（`box1` / `office` / 任意），數量不限。
   - **`default` 為保留名**：唯一作用是 bare `openai-compat` 的解析目標；若不使用 bare 形可不設。
   - **名稱不可含冒號 `:`**（前綴與實例名的分隔符）。**僅以文件/註解約束，不在碼中強制**——
     含冒號只會讓該名稱無法被乾淨引用（良性後果），不值得為它加驗證碼（Simplicity）。
3. **零複製**：每個實例是完整 `OpenAICompatConfig`（endpoint/model/api_key/timeout 各自獨立），
   impl 重用既有 `OpenAICompatSummarizer`。
4. **未知實例名** → 回含實例名的明確錯誤。涵蓋三種：`openai-compat:foo` 但 map 無 foo；
   bare `openai-compat` 但無 `default` 實例；`openai-compat:`（空實例名）。三者皆走同一錯誤路徑。
5. 更新 `config.example.yaml`（遷移成 map + `default` + 多 box HA 範例）與 `--llm` flag help 文字。
6. `DefaultConfig()` 產出 `OpenAICompat: map[string]OpenAICompatConfig{"default": {...}}`，
   讓 bare `openai-compat` 開箱即用。

## Out of Scope（明確不做）

- 負載分散 / round-robin。
- 自動發現（mDNS / 區網掃描）。
- 為 LM Studio / vLLM / oMLX 另立廠商專屬 key（R2 一個 `openai-compat` key 通吃）。
- 改 `SummarizeResult.Provider` 的回報字串（仍硬寫 `"openai-compat"`；哪台答的由 breaker 日誌識別）。
- 提供向後相容過渡層 / polymorphic 解析（已選擇直接破壞，不做雙形相容）。
- 在碼中強制「實例名不可含冒號」（僅以文件/註解約束）。

## 破壞性與遷移盤點

型別 `OpenAICompatConfig` → `map[string]OpenAICompatConfig` 會打到的既有點（已盤點，量小）：

- `config/config.go:174` 欄位型別；`config/config.go:377` `DefaultConfig()` struct 字面 → map。
- `summarizer/summarizer.go:170-177` `cfg.OpenAICompat.X` 欄位存取 → 改實例解析。
- `summarizer/summarizer_test.go:35,81` 既有測試塞 struct 字面 → 遷移成 map。
- `config.example.yaml` 既有 `openai-compat:` struct 段 → map 形（含 `default`）。

## Open Questions

None — 全數於對話中解決（含「破壞向後相容可接受」已確認）。
