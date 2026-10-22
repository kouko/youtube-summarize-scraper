# TUI config display in original order — acceptance test evidence

Tried on 2026-10-22, in a clean copy of the project at 225a3e4 (separate `git`
worktree at `/Users/kouko/.local/share/opencode/worktree/c554d1/at-config-order`,
branch `feat/2026-10-21-tui-config-original-order`; `git status --porcelain`
empty at start).

## Setup and clean-copy check

- The project's own README setup instructions were followed in the clean copy:
  `make build` — downloaded yt-dlp, compiled ffmpeg and whisper-cli from
  source, then built the Go binary. Succeeded (final lines:
  `[SUCCESS] whisper-cli built at .../embedded/bin/darwin-arm64/whisper-cli`,
  `==> Building ytss for darwin-arm64...`).
- `./ytss --help` printed the usage and the four commands, so the change
  installs and loads in a clean copy.
- The real full-screen TUI cannot be driven headlessly in this environment
  (no interactive terminal). Per the station's note, the config panel was
  exercised through the exported config-tree functions the TUI itself calls —
  `NewConfigView` / `Lines` / `RenderTable` / `SetValue` / `Serialized`
  (tui/config.go) — and the save path wrote with `os.WriteFile` exactly as the
  TUI save handler does (tui/model.go:1129-1138). The batch path was exercised
  by constructing a real pipeline (`NewPipeline`) and calling
  `ProcessBatchStreaming` (pipeline/pipeline.go:194).
- Probes are Go tests in a throwaway `acceptanceprobe/` package inside the
  clean worktree (untracked, never committed). Each probe is named
  `test_<unit>_<state>_<expected>`.

## 1. TUI 設定畫面最上層 section 的順序與設定檔中的順序一致（不再隨機）。

- How I tried it:
  `go test ./acceptanceprobe/ -run 'Test_configpanel_withRandomOrderKey|Test_configpanel_reparsed25x|Test_configpanel_vaultConfigCopy' -v`
- What came back — the panel's content lines for a config that still carries a
  top-level `random_order: true` (probe
  `Test_configpanel_withRandomOrderKey_rendersSectionsInFileOrder`):
  ```
  random_order    true
  output_dir      ~/ytss-output
  preferred_languages (2)
    [1]           zh
    [2]           ja
  default_count   5
  whisper
    max_duration  1800
  llm
    provider      claude-api
  batch
    random_order  true
    watch         false
  obsidian
    enabled       false
  ```
  The legacy top-level key renders as an ordinary first row in file order
  (it is not swallowed), and every top-level key appears in the file's order.
- 25 fresh parses of the same config rendered byte-identical content lines
  (probe `Test_configpanel_reparsed25x_rendersSameOrderEveryLaunch`):
  `25 fresh parses rendered identical content lines (file order, deterministic)`.
- The user's real vault config (44,852 bytes) renders with its 13 top-level
  sections in file order (probe
  `Test_configpanel_vaultConfigCopy_rendersSectionsInFileOrder`); the only
  `random_order` occurrence is the nested `batch.random_order` row. First
  rows of the rendered panel:
  ```
  │ output_dir        │ ~/youtube-summarize-scraper/ytss-output │
  │ preferred_languages (3)                                     │
  │   [1]             │ ja                                      │
  ...
  ```
- Evidence: named probes above; all PASS. The rendered-table assertion uses
  the panel's untruncated content lines because the table view caps the key
  column width (pre-existing behaviour, `TestConfigTableKeyColumnCapped`).

## 2. 在 TUI 編輯任一設定值並儲存後，實際設定檔的最上層順序與註解保持原樣（目前會被永久打亂）。

- How I tried it: `go test ./acceptanceprobe/ -run 'Test_configedit' -v`.
  Both probes parse a commented config, edit one scalar
  (`default_count` 5 → 7), serialize, and write to a file with `os.WriteFile`
  exactly like the TUI save handler, then re-read the written file. The
  second probe runs the same flow on a copy of the user's real vault config.
- What came back — diff of the commented fixture before vs after edit+save
  (probe `Test_configedit_commentedConfig_writeBackKeepsOrderAndComments`),
  generated with `diff -u` on the fixture vs the written-back file:
  ```diff
  --- /tmp/fixture-original.yaml
  +++ /tmp/written-back.yaml
  @@ -1,16 +1,16 @@
   # YTSS config — acceptance probe fixture
  -random_order: true   # legacy top-level switch (must render/save as ordinary data)
  -output_dir: "~/ytss-output"   # where summaries go
  +random_order: true # legacy top-level switch (must render/save as ordinary data)
  +output_dir: "~/ytss-output" # where summaries go
   preferred_languages:
  -  - zh
  -  - ja
  -default_count: 5
  +    - zh
  +    - ja
  +default_count: 7
   whisper:
  -  max_duration: 1800   # cap long videos
  +    max_duration: 1800 # cap long videos
   llm:
  -  provider: "claude-api"   # primary backend
  +    provider: "claude-api" # primary backend
   batch:
  -  random_order: true   # shuffle processing order
  -  watch: false
  +    random_order: true # shuffle processing order
  +    watch: false
   obsidian:
  -  enabled: false
  +    enabled: false
  ```
  The only content changes are the edited value `default_count: 5` →
  `default_count: 7` and indentation normalization (2 → 4 spaces) with
  comment spacing tightened; every top-level key stays in place, all six
  comments survive, and the legacy top-level `random_order: true` line
  survives as ordinary data (both it and `batch.random_order` still decode
  as `true` after the write).
  Note: the indentation normalization is the pre-existing write-back style
  (yaml.v3 Marshal), identical at base bd111a4 — the dismissed nit, not
  introduced by this change.
- On the user's real vault config (probe
  `Test_configedit_vaultConfigCopy_writeBackKeepsOrderAndComments`):
  ```
  original top-level keys (13): [output_dir preferred_languages default_count
    video_embed whisper cookie llm summary filter batch obsidian playlists channels]
  original comment lines: 91
  written file: top-level order kept, 91/91 comments kept, batch.random_order still true
  ```
  No top-level `random_order` appeared in the written file; only key names
  and comment counts were logged (no values).
- After the edit the panel redraws in file order (asserted inside
  `Test_configedit_commentedConfig_writeBackKeepsOrderAndComments`).
- Evidence: named probes above; all PASS.

## 3. 設定檔不再帶有最上層的隨機顯示開關；`batch.random_order`（批次處理順序）不受影響、維持有效。

- How I tried it: `go test ./acceptanceprobe/ -run 'Test_configfiles|Test_batchconfig|Test_pipeline' -v -timeout 120s`,
  plus direct inspection of both user config files.
- What came back — file checks (probe
  `Test_configfiles_repoConfigAndVaultConfig_noTopLevelRandomOrder_batchKeyKept`):
  ```
  /Users/kouko/youtube-summarize-scraper/config.yaml: top-level keys
    [output_dir preferred_languages default_count whisper cookie llm summary
     filter batch obsidian playlists channels] — no top-level random_order,
    batch.random_order present
  /Users/kouko/kouko-obsidian-vault/_config/ytss-config.yaml: top-level keys
    [output_dir preferred_languages default_count video_embed whisper cookie
     llm summary filter batch obsidian playlists channels] — no top-level
    random_order, batch.random_order present
  ```
- `batch.random_order` still decodes (probe
  `Test_batchconfig_realConfigs_batchSectionDecodes_randomOrderTrue`):
  ```
  repo config:   batch section decodes; random_order=true delay_min=0  delay_max=30 fetch_concurrency=0
  vault config:  batch section decodes; random_order=true delay_min=0  delay_max=3  fetch_concurrency=5
  ```
  The user's live vault config also loads through the normal config loader
  (`Test_batchconfig_vaultConfig_fullLoad_decodesRandomOrderTrue`):
  `full load OK; Batch.RandomOrder=true channels=123 playlists=9`.
  A minimal config still honours both values
  (`Test_batchconfig_minimalYaml_randomOrderFalseAndTrue_decodes`):
  `random_order: true -> true`, `random_order: false -> false`.
- The batch pipeline still shuffles processing order when the switch is on
  (probe `Test_pipeline_batchRandomOrderTrue_shufflesSources`, dry run with 3
  unreachable local channels):
  ```
  level=INFO msg="shuffled channel order"
  level=INFO msg="streaming: starting producers" playlists=0 channels=3 concurrency=3
  ```
  and does not shuffle when the switch is off
  (`Test_pipeline_batchRandomOrderFalse_preservesSourceOrder`):
  `batch.random_order=false: no shuffle log lines emitted (correct)`.
- Pre-existing observation, not caused by this change: `config.Load` on the
  repo's `config.yaml` fails with
  `yaml: unmarshal errors: line 68: cannot unmarshal !!str into config.OpenAICompatConfig`
  (and lines 69-71) — the `llm.openai-compat` block there still uses the old
  single-server shape the README documents as a breaking change. The batch
  section of that same file decodes fine (above), and config/pipeline code is
  unchanged base..HEAD (`git diff bd111a4..225a3e4 --stat -- config/config.go
  pipeline/pipeline.go` is empty), so this is unrelated to the change.
- Evidence: named probes above; all PASS.

## 4. UI flow — open the TUI and select a config file → panel lists top-level sections in the file's own order (same order every launch)

- How I tried it: probes 1 and 3 above drive the same load + render path the
  panel uses (`NewConfigView` → `RenderTable`); the empty-state card is
  covered by the model-level test below.
- What came back: file order, deterministic across 25 launches; vault config
  in file order. Evidence: named probes; PASS.

## 5. UI flow — press Enter on a value row, type, press Enter to save → file keeps original order and comments; panel redraws in the same order

- How I tried it: probes in section 2 perform the same edit → serialize →
  write sequence on real config copies and re-render the panel after the edit.
- What came back: order and comments preserved; panel redraws in file order.
  The keystroke handling itself was not exercised (headless limitation,
  disclosed in the report). Evidence: named probes; PASS.

## 6. UI flow — start a pipeline run → order still follows batch.random_order (unchanged)

- How I tried it: probes `Test_pipeline_batchRandomOrderTrue_shufflesSources`
  and `Test_pipeline_batchRandomOrderFalse_preservesSourceOrder` run a real
  dry-run batch with 3 unreachable local channels, capturing pipeline logs.
- What came back: `msg="shuffled channel order"` appears only when the switch
  is on. Evidence: named probes; PASS.

## 7. UI flow — empty state → "(no config selected)" before a file is chosen (unchanged)

- How I tried it: `go test ./tui/ -run 'TestModelConfigCardShortHeight|TestEndToEndSmoke' -v`.
- What came back: both PASS
  (`TestModelConfigCardShortHeight` asserts the card shows
  `(no config selected)` before any selection; `TestEndToEndSmoke` asserts
  the four panels plus the empty-state text). Evidence: named tests; PASS.

## 8. UI flow — error state → a malformed YAML file shows its parse error text (unchanged)

- How I tried it: probe `Test_configpanel_malformedYaml_showsParseError`
  feeds `output_dir: [unclosed\n  bad: yaml` to the panel.
- What came back: `panel content: Error parsing config: yaml: line 1: did not
  find expected ',' or ']'`. Evidence: named probe; PASS.

## Negative control at base bd111a4 (do the probes detect the pre-fix bug?)

- How: `git worktree add /private/tmp/ytss-base-bd111a4 bd111a4`, copied the
  TUI probe file in, ran
  `go test ./acceptanceprobe/ -run 'Test_configpanel_withRandomOrderKey|Test_configpanel_reparsed25x' -v`.
- What came back (FAIL, as expected on the pre-fix code):
  ```
  first row is "preferred_languages (2)", want random_order (file order)
  key "output_dir" not found after position 178 in rendered text
  parse 1 rendered a different order than parse 0 — order is not stable
  ```
  So the probes catch both old defects (key swallowed, order unstable) and
  are not vacuous.

## Affected-package test suites (constraint: existing tests keep passing)

- Command run here (criterion scope — the full repo suite `go test ./...` is
  `finalize-review`'s check and was not run by acceptance testing):
  `go test ./tui/ ./config/ ./pipeline/ -count=1 -timeout 300s`
  ```
  ok  github.com/kouko/youtube-summarize-scraper/tui       1.432s
  ok  github.com/kouko/youtube-summarize-scraper/config    0.388s
  ok  github.com/kouko/youtube-summarize-scraper/pipeline  0.732s
  ```
- The change's own regression tests pass:
  `go test ./tui/ -run 'TestConfigTreeRandomOrder' -v` →
  `TestConfigTreeRandomOrderKeyRendersInFileOrder` PASS,
  `TestConfigTreeRandomOrderKeySurvivesWriteBack` PASS.
