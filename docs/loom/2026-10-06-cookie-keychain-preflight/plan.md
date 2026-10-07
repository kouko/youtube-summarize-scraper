# Browser cookie keychain preflight — plan
intent: 2026-10-06-cookie-keychain-preflight@390153e
spec: docs/loom/2026-10-06-cookie-keychain-preflight/spec.md@d9a4e37
charter: 1.1

## Task DAG

Wave 1 — keychain check primitive (fetcher package), then the yt-dlp error hint that reuses its fix-command text.
Wave 2 — CLI wiring of the startup check.

**W1-01 Keychain readability check**  after: none  acceptance: 1, 2, 3
- Files: fetcher/cookie.go, fetcher/cookie_test.go
- Test: A1 positive: lookup exit 36 for `Chrome:Default` returns not-accessible error naming Chrome; negative: firefox skipped. A2 positive: exit 0 returns nil; boundary: 30s timeout returns not-accessible. A3 positive: file set skips; negative: linux GOOS skips.
- Risk: Injected GOOS and lookup function so CI on Linux tests every exit code (spec REQ-1 design, agent-decided). Timeout result is injectable so tests avoid real 30s waits. Real lookup sends stdout to null device; key never read.

**W1-02 Name cookie-decrypt cause in yt-dlp errors**  after: W1-01  acceptance: 4
- Files: fetcher/fetcher.go, fetcher/fetcher_test.go
- Test: A4 positive: darwin stderr containing `cannot decrypt v10 cookies` yields error starting with the cookie hint plus original stderr; negative: same stderr on linux leaves error unchanged.
- Risk: Shares fix-command constant from W1-01. Hint built by a pure (goos, stderr) function, applied in runYtDlpWithTimeout; existing fetcher_test.go cases preserved (spec REQ-4 design, agent-decided).

**W2-01 Startup preflight in run, channel and video**  after: W1-01  acceptance: 1, 2, 3
- Files: cmd/helpers.go, cmd/helpers_test.go, cmd/run.go, cmd/channel.go, cmd/video.go
- Test: A1 positive: locked playlist chrome cookie → one-line error before pipeline, SilenceUsage; negative: channel ignores playlist browser. A2 positive: readable proceeds; boundary: error omits stdout. A3 positive: no cookies, no lookup; negative: cookie-file wins, no lookup.
- Risk: Runs after applyOverrides, includes dry-run; run collects global plus playlist and channel cookies, channel/video only global (spec REQ-1 design, agent-decided).

## Simplicity check
- Put startup preflight helper in existing cmd/helpers.go instead of new cmd/preflight.go — taken
- Put keychain check in existing fetcher/cookie.go instead of new fetcher/keychain.go — taken
- Merge W2-01 into W1-01 via root PersistentPreRunE — declined: applyOverrides runs inside each RunE, spec requires check after overrides

## Questions asked
① — what — 程式啟動時如果發現 Chrome cookie 讀不到（keychain 鎖住），你希望怎麼處理？（answer: 直接停止）
① — consequence — 以上需求和產品原則都正確嗎？（含：直接停止會讓不需登入的頻道也整批停掉；回答「對」授權審查通過後自動推送並開 Ready PR，合併另議）（answer: 對）
② — behaviour — 你執行 ytss 時：keychain 鎖住→只印一行錯誤與解鎖指令、結束代碼 1；從未登入→提示登入、結束代碼 1；卡住→30 秒後停止；讀得到/沒設定→照舊；（Mac）執行中解密失敗→錯誤訊息開頭寫讀不到 cookie。這樣對嗎？（answer: 對）
② — behaviour — （審查修正後重新確認，含 Chrome:Default 寫法、只印一行、執行中提示僅 Mac、--cookie-browser 與 --dry-run 也檢查）這樣對嗎？（answer: 好，繼續）
② — behaviour — （對抗測試後修正：頻道底下的 cookie 不檢查、執行中提示改為「包含」）請再確認一次畫面行為。（answer: 對）
② — what — 還沒推送的 commit e1a4af0 要怎麼處理？A 一起推／B 從 PR 排除／C 你先自己推送（answer: C）
② — behaviour — （結案審查發現「金鑰讀得到但不對」沒有提示後，你選了 B 一起修）金鑰不對時錯誤訊息會寫「無法用金鑰解密 cookie（金鑰不對？設定檔從別台電腦複製？）— 請在瀏覽器重新登入 YouTube」，不解鎖指令；其他行為不變。這樣對嗎？（answer: B 繼續）

## Risks
1. Exit codes 36 (locked) and 44 (not found) come from the incident and Apple docs, not independently re-measured; acceptance testing on this Mac must reproduce locked and unlocked runs.
2. The not-found stop can block a run that works today for public lists with a browser configured but never signed in; user confirmed this row at decision point ②.
