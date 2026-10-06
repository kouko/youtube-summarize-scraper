# Browser cookie keychain preflight — acceptance test evidence

Tried on 2026-10-06, in a clean copy of the project at 1b588d7. This is a re-run after the
wrong-key hint was added in commit e014268 (branch `feat/2026-10-06-cookie-keychain-preflight`);
only Acceptance line 4 is re-tested here — the new binary is `ytss`, the fake `security` helpers
are the same `fake36`/`fakeempty` scripts, and the WL playlist config is `cfg-chrome.yaml`.

## Setup (clean copy)

- Clean copy: `git worktree add --detach <scratch>/at-clean 1b588d7` (fresh tree, no build artefacts).
- README "Build from Source" path: `make download-deps` downloaded yt-dlp 2026.08.19 (exit 0).
  `make build` would then compile ffmpeg, which needs `nasm`; `nasm` is not installed on this host
  (`which nasm` → not found) and installing packages on the user's machine was out of bounds. So the
  already-built `ffmpeg` and `whisper-cli` were copied from the user's main checkout into
  `embedded/bin/darwin-arm64/`, after which `make build` skipped both ("already exists, skipping build")
  and compiled `ytss` (version `v0.5.4-31-g1b588d7`). The change itself builds and loads; the missing
  `nasm` is a host prerequisite, not caused by this change.
- Baseline for "same as today": the pre-change code (`0925c16`, parent of the first code commit) built
  the same way with the same embedded tools → `ytss-base`. The change under test → `ytss-new`.
- Isolation: every run used `HOME=<scratch>/at/home` (with `Library` symlinked to the real
  `~/Library`, so Chrome's cookie store and the real login keychain stay reachable) so embedded tools
  extract to a scratch `~/.ytss/bin`, never the user's. Configs and `output_dir` are scratch files;
  no `copy_to`, no vault. All list runs used `--dry-run` and counts of 1–2.
- Keychain states were simulated by prepending a scratch dir to `PATH` holding a fake `security`
  script (ytss and yt-dlp both run `security` by name):
  - `fake36`: prints "User interaction is not allowed." to stderr, exit 36 (locked keychain).
  - `fake44`: "item could not be found", exit 44.
  - `fakesleep`: `sleep 120` (an unanswered access dialog).
  - `fakeflip`: first call exit 0 with no output, every later call exit 36 (startup check passes,
    yt-dlp's own lookup then fails); every call logged.
  - `fakeempty`: every call exit 0 with no output (yt-dlp gets an empty key).
- The real keychain was never locked; the one real lookup ran as
  `security find-generic-password -w -a Chrome -s "Chrome Safe Storage" >/dev/null 2>&1` → exit 0.
  The key was never printed or captured.

Configs used (all in scratch):
- `cfg-chrome.yaml`: `cookie.browser: chrome`, one playlist `https://www.youtube.com/playlist?list=WL` count 2.
- `cfg-plcookie.yaml`: no global cookie; the WL playlist has its own `cookie.browser: "Chrome:Default"`.
- `cfg-nocookie.yaml`: no cookie block; channel `@NASA` count 1.
- `cfg-cookiefile.yaml`: `cookie.file` set to an empty Netscape cookie file AND `cookie.browser: chrome`; channel `@NASA` count 1.

## 1. 在 macOS 上、設定使用 Chrome 登入資料、登入資料無法讀取（鑰匙圈鎖住）時執行程式：程式在處理任何頻道或清單之前就停止，結束代碼不是 0，訊息說明登入資料讀不到，並附上可直接執行的修復指令。

- How I tried it (all with `HOME=<scratch>/at/home`):
  - `PATH=fake36:$PATH ytss-new run -c cfg-chrome.yaml` (and again with `--dry-run`)
  - `PATH=fake36:$PATH ytss-new channel @NASA -n 1 -c cfg-nocookie.yaml --cookie-browser 'Chrome:Default' --dry-run`
  - `PATH=fake36:$PATH ytss-new video 'https://www.youtube.com/watch?v=dQw4w9WgXcQ' -c cfg-chrome.yaml --dry-run`
  - `PATH=fake36:$PATH ytss-new run -c cfg-plcookie.yaml` (playlist-only browser cookie)
  - `PATH=fake44:$PATH ytss-new run -c cfg-chrome.yaml` (item missing)
  - `PATH=fakesleep:$PATH ytss-new run -c cfg-chrome.yaml --dry-run` (hang), timed with `date +%T`
  - Contrast: `PATH=fake36:$PATH ytss-base run -c cfg-chrome.yaml --dry-run`
- What came back:
  - run / run --dry-run / channel / video / playlist-only cookie, each the single line below, exit 1:
    ```
    Error: cannot read Chrome cookies: the macOS keychain item "Chrome Safe Storage" is not accessible (keychain locked?). Unlock it and re-run: security unlock-keychain ~/Library/Keychains/login.keychain-db
    ```
    No usage/help block, no log lines. After these runs the scratch HOME held only `Library` (no
    `.ytss` — embedded tools were never even extracted) and `output_dir` held 0 entries.
  - Item missing (44), exit 1:
    ```
    Error: cannot read Chrome cookies: no "Chrome Safe Storage" item in the macOS keychain. Open Chrome and sign in to YouTube once, then re-run.
    ```
  - Hang: started 12:48:10, same "not accessible" error at 12:48:40 (30 s), exit 1. (The fake
    script's own `sleep` child was left orphaned — an artefact of the shell-script fake; the real
    `security` has no child process. Killed by hand.)
  - The fix command's target `~/Library/Keychains/login.keychain-db` exists on this Mac. The unlock
    command itself was not run (the keychain is already unlocked; running it would prompt for the
    user's password).
  - Contrast, pre-change code in the same state: runs the batch, logs
    `playlist fetch failed ... WARNING: find-generic-password failed / WARNING: cannot decrypt v10 cookies: no key found / ERROR: ... The playlist does not exist.`
    and ends `completed: 0 success, 0 skipped, 0 partial, 0 failed`, exit 0 — the incident reproduced.
- Evidence: captured outputs above; tests `TestRunCmd_LockedPlaylistChromeCookie_StopsBeforePipeline`,
  `TestCheckBrowserCookieKeychain_Unreadable`, `TestCheckBrowserCookieKeychain_Timeout` pass.

## 2. 同樣的設定、登入資料可以讀取時執行程式：行為與現在相同，沒有多出的停止或警告。

- How I tried it: real keychain (unlocked, real `security`):
  `ytss-new run -c cfg-chrome.yaml --dry-run 2>&1 | tee a2-new.log` and the same with `ytss-base`
  → `a2-base.log`; then `diff` after stripping `time=` prefixes and `duration=` values.
- What came back: both exit 0; both fetched the Watch Later playlist (2 videos), both listed 2 videos
  as "would process (dry run)", both ended `completed: 0 success, 2 skipped, 0 partial, 0 failed`.
  `diff` → identical. No extra line, no warning; nothing resembling the key appears in output (the
  check discards the lookup's output — `fetcher/cookie.go` `LookupMacKeychain` leaves `cmd.Stdout` nil).
- Evidence: `diff` identical; tests `TestPreflight_Readable_Proceeds`,
  `TestCheckBrowserCookieKeychain_Readable` pass.

## 3. 沒有設定任何瀏覽器登入資料（不用 cookie，或改用 cookie 檔案）時執行程式：行為與現在相同。

- How I tried it: with the locked-keychain fake (`fake36`) first on `PATH` — so any keychain check
  would stop the run — ran `ytss-new` and `ytss-base` `run --dry-run` on `cfg-nocookie.yaml` and on
  `cfg-cookiefile.yaml` (file set and browser set, file wins); diffed each pair as in 2.
- What came back: all four runs exit 0, fetched `@NASA` (videos/streams/shorts tabs), listed 2 videos,
  `completed: 0 success, 2 skipped, 0 partial, 0 failed`. New vs base: identical for both configs.
- Evidence: `diff` identical; tests `TestPreflight_NoCookies_NoLookup`,
  `TestPreflight_CookieFileWins_NoLookup`, `TestCheckBrowserCookieKeychain_FileSkipped` pass.

## 4. 執行過程中抓取失敗、而失敗原因是登入資料解密失敗時：錯誤訊息明確指出是登入資料讀不到，而不是只顯示「清單不存在」。

- How I tried it: `fakeflip` on `PATH` — ytss's startup lookup succeeds, yt-dlp's lookup fails like a
  locked keychain: `ytss-new run -c cfg-chrome.yaml --dry-run`, then the same with `ytss-base`.
  Variant: `fakeempty` (yt-dlp receives an empty key) with `ytss-new`.
- What came back:
  - new: two `security` calls logged (12:53:46 startup check, 12:54:00 yt-dlp). Exit 0 (run continues
    as before); the logged error now starts with the hint, then keeps the original yt-dlp output:
    ```
    level=ERROR msg="playlist fetch failed" url="https://www.youtube.com/playlist?list=WL" error="fetching playlist videos: fetching playlist videos: cannot read browser cookies (keychain locked?) — run: security unlock-keychain ~/Library/Keychains/login.keychain-db\nyt-dlp [... --cookies-from-browser chrome ...]: exit status 1\nstderr: WARNING: find-generic-password failed\nWARNING: cannot decrypt v10 cookies: no key found\nWARNING: [youtube:tab] YouTube said: The playlist does not exist.\nERROR: [youtube:tab] WL: YouTube said: The playlist does not exist.\n"
    ```
  - base, same fake: only one `security` call (no startup check), so yt-dlp got an empty key and printed
    `WARNING: failed to decrypt cookie (AES-CBC) because UTF-8 decoding failed. Possibly the key is wrong?`
    then "The playlist does not exist."
  - new + `fakeempty` (same empty-key situation): error now starts with the wrong-key hint, then keeps
    the original yt-dlp output:
    ```
    time=2026-10-06T21:19:37.650+08:00 level=ERROR msg="playlist fetch failed" url="https://www.youtube.com/playlist?list=WL" error="fetching playlist videos: fetching playlist videos: cannot decrypt browser cookies with the keychain key (wrong key? browser profile copied from another machine?) — sign in to YouTube in the browser again\nyt-dlp [--flat-playlist --dump-json --extractor-args youtubetab:approximate_date --playlist-end 2 --cookies-from-browser chrome https://www.youtube.com/playlist?list=WL]: exit status 1\nstderr: WARNING: failed to decrypt cookie (AES-CBC) because UTF-8 decoding failed. Possibly the key is wrong?\nWARNING: [youtube:tab] YouTube said: The playlist does not exist.\nERROR: [youtube:tab] WL: YouTube said: The playlist does not exist.\n"
    ```
    No unlock command is present. The detector now matches both `cannot decrypt`/`find-generic-password failed`
    (unreadable key) and `failed to decrypt cookie` (wrong key) in `fetcher/fetcher.go` `cookieDecryptHint`.
- Evidence: captured outputs above; test `TestYtDlpError_CookieDecryptHint` passes.

## Package tests

- `go test -race ./...` in the clean copy (run once at the station's request): all packages `ok`
  (cmd, config, fetcher, lang, output, pipeline, subtitle, summarizer), exit 0.
- Focused: `go test -race -run 'Keychain|Cookie|Preflight|YtDlpError|DecryptHint' -v ./cmd/ ./fetcher/`
  → 6 cmd tests and 15 fetcher tests PASS, including the adversary probe
  `TestPreflight_ChannelOnlyBrowserCookie_NoLookup`.
