# Product principles
ratified-by: kouko 2026-10-06

## Who
- People who batch-summarise YouTube channels and playlists into notes (e.g. an Obsidian vault), often running ytss unattended on a home machine. Today they read the generated summaries instead of watching every video.

## Non-negotiables (ordered)
1. Never fail silently — when something the user configured cannot be fetched, the run says so plainly, with the cause and the fix.
2. Never leak secrets — cookies, keychain contents and API keys never appear in output, logs or summaries.
3. Unattended runs keep working — a run must not wait for interactive input.

## Won't do
- Store or ask for the user's system password.
- Bespoke scheduling / load balancing beyond the existing fallback chain.

## Failure we must avoid
- Weeks of missing summaries that nobody notices, because a run "succeeded" while skipping what the user asked for.

## Fixed choices
- Go CLI distributed via Homebrew; yt-dlp is the fetch engine; configuration is a YAML file.
