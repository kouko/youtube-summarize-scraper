#!/usr/bin/env bash
# concern: config table rows must use display-width (not rune count) so CJK characters
#          (width=2) do not misalign the table borders.
# W5-02 A3 boundary (spec amend2): config table rows use display-width (not rune count)
#   so CJK characters (width=2) do not misalign the table borders.

set -euo pipefail
cd "$(dirname "$0")/../../../../.."
go test -race ./tui -run TestModelConfigTableCJKWidth -count=1