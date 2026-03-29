# Exclude Availability Filter

## Problem
Member-only and private videos are included in video lists but fail during processing when no cookies are available. Users need a way to filter them out at the list level.

## Design

### New Config Field

Add `exclude_availability` to `FilterConfig`:

```go
type FilterConfig struct {
    Types               []string `yaml:"types"`
    MinDuration         int      `yaml:"min_duration"`
    MaxDuration         int      `yaml:"max_duration"`
    ExcludeAvailability []string `yaml:"exclude_availability"`
}
```

**Default:** `["members_only", "private"]`

### Config Example

```yaml
# Video filter settings (global default)
# Per-channel "filter:" overrides the ENTIRE block, not individual fields.
# If a channel defines its own filter, all global filter settings are ignored for that channel.
filter:
  exclude_availability:              # Exclude videos by availability (default: ["members_only", "private"])
    - members_only                   # YouTube channel membership content
    - private                        # Private videos
    # - subscriber_only              # Channel subscriber-only content
    # - premium_only                 # YouTube Premium exclusive
    # - needs_auth                   # Requires authentication
```

### Filter Logic

In `FilterVideos()`, after existing duration checks, call `isExcludedAvailability` helper:

```go
if isExcludedAvailability(v.Availability, filter.ExcludeAvailability) {
    continue
}
```

Helper function:

```go
func isExcludedAvailability(availability string, excludeList []string) bool {
    if availability == "" || len(excludeList) == 0 {
        return false
    }
    for _, ea := range excludeList {
        if availability == ea {
            return true
        }
    }
    return false
}
```

### Inheritance

- **Global filter** (`config.filter`): Applied to all playlists and as fallback for channels
- **Per-channel filter** (`channels[].filter`): Overrides the **entire** global `FilterConfig` (existing `EffectiveFilter` behavior), not individual fields
- If per-channel filter is set but `exclude_availability` is omitted, it defaults to `nil` (no availability filtering)
- To override only `exclude_availability` in a per-channel filter, all other global filter fields must be duplicated

### yt-dlp Availability Values

| Value | Description |
|-------|-------------|
| `members_only` | YouTube channel membership content |
| `subscriber_only` | Channel subscriber-only content |
| `premium_only` | YouTube Premium exclusive |
| `needs_auth` | General authentication required |
| `private` | Private videos |
| (empty/other) | Publicly available |

### Files Modified

| File | Change |
|------|--------|
| `config/config.go` | Added `ExcludeAvailability` to `FilterConfig`, set default in `DefaultConfig()` |
| `config/config_test.go` | Added `TestDefaultConfig_ExcludeAvailability` for default value verification |
| `fetcher/filter.go` | Added `isExcludedAvailability` helper, integrated into `FilterVideos()` |
| `fetcher/filter_test.go` | Added `TestFilterVideos_ExcludeAvailability` and `TestFilterVideos_ExcludeAvailabilityEmpty` |
| `config.example.yaml` | Documented `exclude_availability` with all available values and override behavior note |

### Test Coverage

| Scenario | Test |
|----------|------|
| Exclude matching availability values | `TestFilterVideos_ExcludeAvailability` |
| Empty exclude list = no filtering | `TestFilterVideos_ExcludeAvailabilityEmpty` |
| Empty availability string = not filtered | `TestFilterVideos_ExcludeAvailability` (video "public") |
| DefaultConfig default value | `TestDefaultConfig_ExcludeAvailability` |

### Verification

1. `go build ./...` passes
2. `go test ./...` passes (all existing + 3 new tests)
3. `ytss run --dry-run` confirms member-only videos are excluded from list
