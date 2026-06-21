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
# Per-channel/per-playlist "filter:" uses field-level merge with the global filter.
# Only explicitly set fields override the global value; omitted fields inherit from global.
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

- **Global filter** (`config.filter`): Base filter for all channels and playlists
- **Per-channel filter** (`channels[].filter`): Field-level merge with global filter via `FilterOverride` (pointer-based). Only explicitly set fields override global; omitted fields (nil pointers) inherit from global.
- **Per-playlist filter** (`playlists[].filter`): Same field-level merge behavior as per-channel filter via `EffectivePlaylistFilter`.
- `FilterOverride` uses pointer fields (`*int`, `*[]string`) to distinguish "not set" (nil → inherit) from "explicitly set to zero/empty" (non-nil → override). For example, `min_duration: 0` in YAML overrides a global `min_duration: 300`, while omitting `min_duration` inherits the global value.

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
| `config/config.go` | Added `ExcludeAvailability` to `FilterConfig`, set default in `DefaultConfig()`. Added `FilterOverride` struct with pointer fields for field-level merge. Changed `ChannelConfig.Filter` to `*FilterOverride`, added `Filter *FilterOverride` to `PlaylistConfig`. Added `mergeFilter()` helper, `EffectivePlaylistFilter()`. Rewrote `EffectiveFilter()` to use `mergeFilter()`. |
| `config/config_test.go` | Added `TestDefaultConfig_ExcludeAvailability` for default value verification. Added comprehensive merge tests: `NilOverride`, `PartialOverride_*`, `ExplicitZero_*`, `ExplicitEmpty_*`, `EmptyOverride`, `FullOverride`, `PlaylistFilter_*`, `Load_FilterOverrideMerge`, `ReloadPartial_FilterMergeAfterReload`, `YAMLPointerDistinction` (channel + playlist). |
| `fetcher/filter.go` | Added `isExcludedAvailability` helper, integrated into `FilterVideos()` |
| `fetcher/filter_test.go` | Added `TestFilterVideos_ExcludeAvailability` and `TestFilterVideos_ExcludeAvailabilityEmpty` |
| `pipeline/pipeline.go` | Changed playlist filter call sites (L365, L857) to use `EffectivePlaylistFilter`. Added `Filter` field propagation in `playlistToChannelCfg`. |
| `pipeline/playlist_convert_test.go` | Added `TestPlaylistToChannelCfg_PropagatesFilter`, `_NilFilter`, `_Nil`. |
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
