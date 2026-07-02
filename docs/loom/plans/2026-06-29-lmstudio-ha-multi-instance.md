# Plan: 區網多台 LM Studio HA（openai-compat 多實例，R2 置換）

**Source brief**: docs/loom/specs/2026-06-29-lmstudio-ha-multi-instance.md
**Total tasks**: 4
**Critical-path depth**: 3 (≤5 ✓)
**Execution order**: parallel-where-possible
**Plan-document-reviewer verdict**: PASS (2026-06-30)

## Task 1 — 將 LLMConfig.OpenAICompat 由 struct 改為 map

- **Description**: 把 `LLMConfig.OpenAICompat` 欄位型別由 `OpenAICompatConfig` 改為 `map[string]OpenAICompatConfig`（yaml tag 維持 `openai-compat`）。同步把 `DefaultConfig()`（config.go:377）的 struct 字面改為 `map[string]OpenAICompatConfig{"default": {Endpoint: "http://localhost:8000/v1", Timeout: 900}}`。`OpenAICompatConfig` 結構本身不動。
- **Module**: config/config.go
- **Files touched**: config/config.go, config/config_test.go
- **Context paths**:
  - /Users/kouko/GitHub/youtube-summarize-scraper/config/config.go (OpenAICompat 欄位 L174, OpenAICompatConfig L220, DefaultConfig L377)
  - /Users/kouko/GitHub/youtube-summarize-scraper/config/config_test.go (解析測試慣例)
- **Acceptance**:
  - **RED**: `config_test.go::TestLLMConfig_OpenAICompat_Map_Parse` — unmarshal 一段含 `openai-compat: {default: {endpoint, model}, box1: {endpoint}}` 的 YAML，斷言 `cfg.LLM.OpenAICompat["default"].Endpoint` 與 `["box1"].Endpoint` 等於預期（型別仍是 struct → 編譯/斷言失敗）。
  - **GREEN**: 欄位為 map 並正確解析；`DefaultConfig()` 回 `default` 實例；`go test ./config/` 全綠（config 套件自足，可獨立通過）。
- **Dependencies**: none
- **Independent**: false
- **Brief item covered**: Decision 1「openai-compat struct → map（破壞向後相容）」+ Decision 6「DefaultConfig 產出 default 實例」

## Task 2 — newSingleProvider 解析 openai-compat 家族（default / 具名 / 未知錯誤）

- **Description**: 重寫 `newSingleProvider` 的 openai-compat 分支：偵測 `name == "openai-compat"`（實例名取 `default`）或前綴 `openai-compat:`（實例名取後綴），查 `cfg.OpenAICompat[instance]`，命中則用既有 `OpenAICompatSummarizer` 建構（重用 L169-179 的 timeout 預設邏輯）；查無 → 回含實例名的錯誤。一併遷移既有測試 `summarizer_test.go:35,81` 的 struct 字面為 map 形。
- **Module**: summarizer/summarizer.go
- **Files touched**: summarizer/summarizer.go, summarizer/summarizer_test.go
- **Context paths**:
  - /Users/kouko/GitHub/youtube-summarize-scraper/summarizer/summarizer.go (newSingleProvider L108-183, case "openai-compat" L169-179)
  - /Users/kouko/GitHub/youtube-summarize-scraper/summarizer/openai_compat.go (OpenAICompatSummarizer 欄位)
  - /Users/kouko/GitHub/youtube-summarize-scraper/summarizer/summarizer_test.go (TestNewSingleProvider_AllProviders L35、DefaultTimeouts L81 既有 struct 字面需遷移)
  - /Users/kouko/GitHub/youtube-summarize-scraper/config/config.go (Task 1 後的 map 型別)
- **Acceptance**:
  - **RED**: `summarizer_test.go::TestNewSingleProvider_OpenAICompatInstances` — table-driven：(a) bare `openai-compat` 回非 nil 且 endpoint 來自 `OpenAICompat["default"]`；(b) `openai-compat:box1` 回非 nil 且 endpoint 來自 `["box1"]`；(c) `openai-compat:missing` 回 error 且訊息含 "missing"；(d) bare `openai-compat` 但 map 無 `default` → error 且訊息含 "default"；(e) `openai-compat:`（空實例名）→ error。初始失敗（欄位現為 struct，存取 `.Endpoint` 編譯失敗 / 行為錯）。
  - **GREEN**: 五 case 皆過（c/d/e 走同一未知實例錯誤路徑）；既有 `TestNewSingleProvider_AllProviders`、`TestNewSingleProvider_DefaultTimeouts` 改 map 形後仍綠；`go test ./summarizer/` 全綠。
- **Dependencies**: Task 1 completes first
- **Independent**: false
- **Brief item covered**: Decision 2「bare=default、openai-compat:<name>=具名」+ Decision 3「impl 重用 OpenAICompatSummarizer，零複製」+ Decision 4「未知實例名 → 明確錯誤」

## Task 3 — config.example.yaml 遷移成 map + default + 多 box HA 範例 + 載入守護測試

- **Description**: 把 `config.example.yaml` 既有 `openai-compat:` struct 段遷移為 map 形：一個 `default:` 實例（沿用現值）+ box1/box2 兩台 LM Studio（預設 port 1234，IP 不同），並加一段 `provider: ["openai-compat:box1", "openai-compat:box2"]` HA 範例註解。**註解須寫明命名規則**：實例名使用者自訂、數量不限；`default` 為保留名（bare `openai-compat` 的解析目標，不用 bare 形可省略）；名稱不可含冒號 `:`。新增測試載入該範例檔，斷言實例解析正確（防範例漂移）。
- **Module**: config.example.yaml
- **Files touched**: config.example.yaml, config/example_load_test.go
- **Context paths**:
  - /Users/kouko/GitHub/youtube-summarize-scraper/config.example.yaml (現有 llm.openai-compat 段 L109-113)
  - /Users/kouko/GitHub/youtube-summarize-scraper/config/config.go (Load 函式 L298)
- **Acceptance**:
  - **RED**: `config/example_load_test.go::TestExampleConfig_LoadsOpenAICompatInstances` — `Load("../config.example.yaml")` 後斷言 `OpenAICompat` 含 `default`/`box1`/`box2`，且 box 的 endpoint 為 `http://...:1234/v1`。檔案尚為 struct 形 → 失敗。
  - **GREEN**: 範例檔遷移為 map（含 default + 兩 box）+ HA provider 註解 + 命名規則註解（default 保留名 / 自訂名 / 不可含冒號）；測試綠；檔案整體仍能 `Load` 無誤。
- **Dependencies**: Task 2 completes first
- **Independent**: true
- **Brief item covered**: Decision 5「更新 config.example.yaml（遷移成 map + default + 多 box HA 範例）」

## Task 4 — 更新 --llm flag help 文字

- **Description**: 在 `cmd/root.go` 的 `--llm` flag usage 字串標註具名實例語法 `openai-compat:<name>`（bare `openai-compat` = default 實例），讓 CLI help 反映新用法。
- **Module**: cmd/root.go
- **Files touched**: cmd/root.go, cmd/root_test.go
- **Context paths**:
  - /Users/kouko/GitHub/youtube-summarize-scraper/cmd/root.go (--llm flag 註冊 L39)
- **Acceptance**:
  - **RED**: `cmd/root_test.go::TestRootCmd_LLMFlagUsage_MentionsInstanceSyntax` — 取 `rootCmd.PersistentFlags().Lookup("llm").Usage`，斷言含子字串 `openai-compat:<name>`。目前 usage 無此字串 → 失敗。
  - **GREEN**: usage 字串更新；測試綠。
- **Dependencies**: Task 2 completes first
- **Independent**: true
- **Brief item covered**: Decision 5「更新 --llm flag help 文字」

## Notes

- 依賴圖：Task 1 → Task 2 →（Task 3 ∥ Task 4）。最長鏈深度 3。
- Task 3 與 Task 4 為 `Independent: true` 對：Files touched 互斥（config.example.yaml + config/example_load_test.go ｜ cmd/root.go + cmd/root_test.go），無語意相依，兩者皆只待 Task 2，可同一波並行派發。
- **跨套件破壞的中間態**：Task 1 改型別後，summarizer 套件在 Task 2 完成前會編譯失敗（讀 `.OpenAICompat.X` 與測試 struct 字面）。因兩者是不同 Go 套件，`go test ./config/`（Task 1 驗收）仍可獨立通過；summarizer 套件由 Task 2 修復。整支 branch 僅在全部任務完成後為「done」。
- HA 行為由既有 `FallbackSummarizer` 提供，本計畫不含相關任務——provider 名字串 `openai-compat:box1` 直接流經 `NewSummarizer`，breaker 以 name 為 key 自動隔離，無需改 `NewSummarizer`。
- nil-map 安全：Go 對 nil map 查 key 回 (zero, false)，未知實例錯誤路徑自然成立，無 panic 風險。
- 設計沿革：經 lm-studio 專名 →（使用者改意）→ R2 直接置換 `openai-compat` 為 map、bare 解為 default。命名糾結就此消除（key = 前綴 = `openai-compat`）。代價為一次性破壞向後相容，使用者已同意。
- PASS 後修訂（schema-safe，沿用 PASS、免重審）：(1) brief Decision 2 補命名規則（`default` 保留名 / 名稱使用者自訂 / 不可含冒號，後者僅文件約束不在碼中強制）；(2) Task 2 RED 增 (d) bare-無-default、(e) 空實例名 兩個錯誤案例（同一錯誤路徑，table 加列）；(3) Task 3 要求範例註解寫明命名規則。皆為既有任務內的附加驗收——任務數、依賴 DAG、深度、欄位結構、模組邊界、Independent 標記全未變。
