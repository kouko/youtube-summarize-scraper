#!/usr/bin/env bash
# concern: a channel entry's own browser cookie must not stop a run at startup
set -euo pipefail
cd "$(dirname "$0")/../../../../.."
go test -race ./cmd -run TestPreflight_ChannelOnlyBrowserCookie_NoLookup -count=1
