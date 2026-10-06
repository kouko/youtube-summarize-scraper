# Browser cookie keychain preflight — spec
intent: 2026-10-06-cookie-keychain-preflight@390153e
confirmed-behavior: 2026-10-06 @7bfb0dd
pre-build-review: required — the check reads a macOS keychain secret (the browser's cookie decryption key); leaking it to output or logs is a privacy/security failure

## Requirements
REQ-1 — Stop before work when browser cookies are unreadable
  WHEN `ytss run`, `ytss channel` or `ytss video` starts on macOS with a configuration that reads cookies from a Chrome-family browser and that browser's cookie decryption key cannot be read from the keychain, ytss shall stop before fetching any channel, playlist or video, exit with a non-zero code, and print a message that names the browser, says its cookies cannot be read, and gives a copy-pasteable fix command (carried detail: "直接停止 — 印出錯誤和修復指令後整個程式結束，什麼都不處理。") → Acceptance #1

REQ-2 — No change when browser cookies are readable
  WHEN ytss starts with a configuration that reads cookies from a Chrome-family browser and the decryption key can be read, ytss shall process exactly as it does today, printing no additional warning and never printing the key → Acceptance #2

REQ-3 — No change without browser cookies
  WHERE no configuration entry reads cookies from a browser (no cookie settings, or a cookie file is set), ytss shall perform no keychain check and behave exactly as it does today → Acceptance #3

REQ-4 — Name the real cause of a mid-run cookie failure
  IF, on macOS, a channel-list, playlist-list or video-metadata fetch fails during a run and yt-dlp reported that browser cookies could not be decrypted, THEN the error ytss reports for that channel, playlist or video shall state that the browser cookies could not be read and give the same fix command, in addition to the original yt-dlp output → Acceptance #4

## Design decision
- Startup stop, not warn-and-continue — user-decided (decision point ①, 2026-10-06: "直接停止").
- Which settings trigger the check — agent-decided: an entry "reads cookies from a browser" exactly when its cookie file is empty and its browser is set, because that is the precedence the existing cookie-argument builders already apply (file wins over browser). `run` checks the global cookie plus every configured playlist's own cookie — channel entries' own `cookie:` blocks are not checked because no fetch consumes them (pipeline/pipeline.go reads only the global and playlist cookies; found by the Build adversary); `channel` and `video` check only the global cookie, since they never use per-entry settings. Each distinct browser is checked once. The browser name is taken from the configured value the way yt-dlp parses it: the text before the first `+` or `:`, trimmed and lowercased — so `Chrome:Default` and `chrome:Profile 1` both check Chrome. The check runs after CLI overrides (`--cookie-browser` / `--cookie-file`) are applied, and also under `--dry-run` (dry-run still fetches lists).
- How the check reads the keychain — agent-decided: run the same `security find-generic-password -w -a <Name> -s "<Name> Safe Storage"` lookup yt-dlp itself performs, so the preflight passes exactly when yt-dlp would get the key. Stdout (the key) is sent to the null device and never read; only the exit code and the timeout are used (PRINCIPLES non-negotiable 2). Exit 0 = readable; exit 44 = item not found; any other result = unreadable.
- Chrome-family names follow yt-dlp's macOS keyring table: chrome→Chrome, chromium→Chromium, brave→Brave, edge→Microsoft Edge, opera→Opera, vivaldi→Vivaldi, whale→Whale. Other browsers (firefox, safari) are skipped — they do not use this keychain item (intent Out of scope).
- Non-macOS hosts skip the check entirely — agent-decided; intent Out of scope.
- Bounded wait — agent-decided: the lookup has a 30-second timeout and the child process is killed when it expires; on timeout ytss stops with the same unreadable message, because an unattended run must never wait for input (PRINCIPLES non-negotiable 3). If macOS shows an access dialog, yt-dlp's own lookup would show the same one, so the preflight adds no new prompt.
- Watch mode checks once at startup, not every iteration — agent-decided; a keychain stays unlocked until logout/reboot (observed `no-timeout` setting), which restarts the process; REQ-4 covers anything that changes mid-run, including browser entries added by the per-iteration config reload and keychains configured to lock on sleep/idle (accepted).
- REQ-4 detection is on yt-dlp stderr markers `cannot decrypt` and `find-generic-password failed`, applied in the fetcher package's single yt-dlp wrapper (fetcher/fetcher.go:192), so channel-list, playlist-list and metadata fetches inherit it; macOS only, because yt-dlp prints `cannot decrypt` on Linux/Windows too and the unlock command would be wrong there. Subtitle and audio downloads (subtitle/subtitle.go:79, transcriber/transcriber.go:111) are not covered: they run after the list/metadata fetch, which already fails first on the same cookies, and the startup check covers the common case — agent-decided.
- Fix command shown: `security unlock-keychain ~/Library/Keychains/login.keychain-db` for unreadable; for not-found, open the browser and sign in once — agent-decided from the diagnosed incident.
- Preflight errors are printed as the single `Error: …` line shown in UI flows, with cobra's usage block suppressed (the root command does not silence usage today, which would bury the fix command) — agent-decided, keeps the user-confirmed output.
- Testability — agent-decided: the check takes the OS name and the keychain lookup function as injected parameters, so the readable / locked (36) / not-found (44) / timeout cases are unit-tested on CI's Linux runner without calling `security`.
- Messages are English, matching ytss's existing CLI output — agent-decided.

## Alternatives considered
- Warn and continue — rejected by the user at decision point ①.
- Probe by running yt-dlp against a private playlist at startup — rejected: needs network, slow, and fails for reasons other than the keychain.
- `security find-generic-password` without `-w` — rejected: it succeeds while the keychain is locked (observed this session), so it would not detect the failure.
- Checking `security show-keychain-info` lock state — rejected: it reports the keychain, not whether this browser's item is readable, and itself failed with "User interaction is not allowed" in the incident.
- Automatically unlocking the keychain — intent Out of scope (would require storing the user's password).

## Current state evidence
- Forward: cmd/run.go:19 `runCmd.RunE` → `pipeline.NewPipeline` (pipeline/pipeline.go:78) → `ProcessBatch`; cmd/channel.go:21 and cmd/video.go:21 follow the same load-config → NewPipeline path.
- Reverse: cookie args are built by `fetcher.(*Fetcher).cookieArgs` (fetcher/cookie.go:13) and `pipeline.buildCookieArgs` (pipeline/pipeline.go:1512); playlist fetches retry with global cookies at pipeline/pipeline.go:362 and :867.
- Error: `runYtDlpWithTimeout` (fetcher/fetcher.go:192) wraps failures as `yt-dlp <args>: <err>\nstderr: <stderr>`; batch logs it as `playlist fetch failed` (pipeline/pipeline.go:285). With a locked keychain stderr reads "WARNING: find-generic-password failed / WARNING: cannot decrypt v10 cookies: no key found / ERROR: ... The playlist does not exist." and the run otherwise completes.
- Data: `config.CookieConfig{File, Browser, ChromeProfile}` (config/config.go:158); global `Config.Cookie`, optional `*CookieConfig` on channel (config/config.go:283) and playlist (config/config.go:294) entries.
- Boundary: changes stop at cmd startup + the yt-dlp error wrapper; no change to cookie argument construction, retries, Linux/Windows behaviour, or config schema.

## UI flows

Surface: terminal output (the error is the only line printed; no usage/help block follows) of `ytss run` / `ytss channel` / `ytss video` at startup (macOS).

| case | what the user does | what they see |
|---|---|---|
| Keychain locked / key unreadable | runs `ytss run` with `cookie.browser: chrome` (or `Chrome:Default`) | before any channel is processed: `Error: cannot read Chrome cookies: the macOS keychain item "Chrome Safe Storage" is not accessible (keychain locked?). Unlock it and re-run: security unlock-keychain ~/Library/Keychains/login.keychain-db` — nothing is fetched or written; exit code 1 |
| Key item missing | runs `ytss run` with `cookie.browser: chrome`, Chrome never signed in on this Mac | `Error: cannot read Chrome cookies: no "Chrome Safe Storage" item in the macOS keychain. Open Chrome and sign in to YouTube once, then re-run.` — exit code 1 |
| Check hangs (e.g. an access dialog nobody answers) | runs ytss unattended | after 30 s: the same "not accessible" error as the locked case — exit code 1 |
| Key readable | runs `ytss run` | output identical to today; no extra line; the key is never shown |
| No browser cookies | runs `ytss run` with no cookie settings or with `cookie.file` set | output identical to today; no check runs |
| Mid-run cookie failure | (macOS) a playlist fetch fails because yt-dlp could not decrypt cookies | the logged error for that playlist contains `cannot read browser cookies (keychain locked?) — run: security unlock-keychain ~/Library/Keychains/login.keychain-db` followed by the original yt-dlp output; the run continues with the other sources as today |

- Empty: N/A — the check has no list to show; with nothing to check it prints nothing (row "No browser cookies").
- In progress: the check is a single local lookup, normally instant; bounded at 30 s.
- Error → way out: every error row names the command or action that fixes it; after fixing, the user re-runs the same ytss command and lands in the "Key readable" row. Nothing is partially processed, so there is nothing to resume or clean up.
