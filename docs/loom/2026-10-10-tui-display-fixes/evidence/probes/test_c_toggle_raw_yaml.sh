#!/usr/bin/env bash
# concern: 'c' key must toggle between structured config tree and raw YAML view
# REQ-2: Display parsed configuration → Acceptance #3
#   WHEN a configuration file is selected, the bottom-left panel displays the parsed
#   configuration as a sectioned tree... Pressing 'c' toggles between this tree and
#   the raw YAML.
# REQ-5: Keyboard controls → Acceptance #5
#   The TUI shall support: c : toggle between the structured config tree and the raw YAML view

set -euo pipefail
cd "$(dirname "$0")/../../../../.."
go test -race ./tui -run TestModelCToggleRawYAML -count=1