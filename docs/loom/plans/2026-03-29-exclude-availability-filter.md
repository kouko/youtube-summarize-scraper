# Exclude Availability Filter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `exclude_availability` filter to skip member-only and private videos during batch processing.

**Architecture:** New `ExcludeAvailability` field in `FilterConfig`, checked inside `FilterVideos()`. Default excludes `members_only` and `private`. Per-channel override follows existing `EffectiveFilter` behavior (entire FilterConfig replaced).

**Tech Stack:** Go, yt-dlp availability field

---

### Task 1: Add test for availability filtering

**Files:**
- Modify: `fetcher/filter_test.go`

- [ ] **Step 1: Write failing tests**

Append to `fetcher/filter_test.go`:

```go
func TestFilterVideos_ExcludeAvailability(t *testing.T) {
	videos := []VideoMeta{
		{ID: "public", Duration: 600, Availability: ""},
		{ID: "members", Duration: 600, Availability: "members_only"},
		{ID: "private", Duration: 600, Availability: "private"},
		{ID: "premium", Duration: 600, Availability: "premium_only"},
		{ID: "unlisted", Duration: 600, Availability: "unlisted"},
	}
	filter := config.FilterConfig{
		ExcludeAvailability: []string{"members_only", "private"},
	}
	got := FilterVideos(videos, filter)
	if len(got) != 3 {
		t.Errorf("ExcludeAvailability: got %d videos, want 3", len(got))
	}
	ids := map[string]bool{}
	for _, v := range got {
		ids[v.ID] = true
	}
	if !ids["public"] {
		t.Error("ExcludeAvailability: 'public' should be included")
	}
	if !ids["premium"] {
		t.Error("ExcludeAvailability: 'premium' should be included (not in exclude list)")
	}
	if !ids["unlisted"] {
		t.Error("ExcludeAvailability: 'unlisted' should be included")
	}
	if ids["members"] {
		t.Error("ExcludeAvailability: 'members' should be filtered out")
	}
	if ids["private"] {
		t.Error("ExcludeAvailability: 'private' should be filtered out")
	}
}

func TestFilterVideos_ExcludeAvailabilityEmpty(t *testing.T) {
	videos := []VideoMeta{
		{ID: "members", Duration: 600, Availability: "members_only"},
		{ID: "public", Duration: 600, Availability: ""},
	}
	// Empty exclude list = no availability filtering.
	got := FilterVideos(videos, config.FilterConfig{})
	if len(got) != 2 {
		t.Errorf("ExcludeAvailabilityEmpty: got %d videos, want 2", len(got))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./fetcher/ -run TestFilterVideos_ExcludeAvailability -v`
Expected: FAIL — `ExcludeAvailability` field does not exist on `FilterConfig`

- [ ] **Step 3: Commit**

```bash
git add fetcher/filter_test.go
git commit -m "test: add availability filter test cases (red)"
```

---

### Task 2: Add ExcludeAvailability to FilterConfig

**Files:**
- Modify: `config/config.go:223-227` (FilterConfig struct)
- Modify: `config/config.go:350-352` (DefaultConfig Filter)

- [ ] **Step 1: Add field to FilterConfig**

In `config/config.go`, change `FilterConfig`:

```go
type FilterConfig struct {
	Types               []string `yaml:"types"`
	MinDuration         int      `yaml:"min_duration"`
	MaxDuration         int      `yaml:"max_duration"`
	ExcludeAvailability []string `yaml:"exclude_availability"`
}
```

- [ ] **Step 2: Set default value in DefaultConfig()**

In `config/config.go`, change the `Filter` block in `DefaultConfig()`:

```go
Filter: FilterConfig{
	Types:               []string{"video", "live", "short"},
	ExcludeAvailability: []string{"members_only", "private"},
},
```

- [ ] **Step 3: Verify compilation**

Run: `go build ./...`
Expected: Success

- [ ] **Step 4: Commit**

```bash
git add config/config.go
git commit -m "feat: add ExcludeAvailability to FilterConfig with default"
```

---

### Task 3: Implement availability check in FilterVideos

**Files:**
- Modify: `fetcher/filter.go:8-24` (FilterVideos function)

- [ ] **Step 1: Add availability filtering logic**

Replace the `FilterVideos` function in `fetcher/filter.go`:

```go
func FilterVideos(videos []VideoMeta, filter config.FilterConfig) []VideoMeta {
	var result []VideoMeta
	for _, v := range videos {
		// Skip upcoming and currently live streams (cannot be downloaded).
		if v.LiveStatus == "is_upcoming" || v.LiveStatus == "is_live" {
			continue
		}
		if filter.MinDuration > 0 && v.Duration < float64(filter.MinDuration) {
			continue
		}
		if filter.MaxDuration > 0 && v.Duration > float64(filter.MaxDuration) {
			continue
		}
		if isExcludedAvailability(v.Availability, filter.ExcludeAvailability) {
			continue
		}
		result = append(result, v)
	}
	return result
}

// isExcludedAvailability returns true if the video's availability matches
// any value in the exclude list.
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

- [ ] **Step 2: Run all filter tests**

Run: `go test ./fetcher/ -run TestFilterVideos -v`
Expected: All PASS (including new `ExcludeAvailability` tests)

- [ ] **Step 3: Run full test suite**

Run: `go test ./...`
Expected: All PASS

- [ ] **Step 4: Commit**

```bash
git add fetcher/filter.go
git commit -m "feat: implement exclude_availability filtering in FilterVideos"
```

---

### Task 4: Update config.example.yaml

**Files:**
- Modify: `config.example.yaml:106-110`

- [ ] **Step 1: Add exclude_availability documentation**

Replace the filter section in `config.example.yaml`:

```yaml
# Video filter settings
filter:
  types: ["video", "live", "short"]  # video / live / short (default: all types)
  min_duration: 0                    # Min seconds (0 = no filter)
  max_duration: 0                    # Max seconds (0 = no limit)
  exclude_availability:              # Exclude videos by availability (default: ["members_only", "private"])
    - members_only                   # YouTube channel membership content
    - private                        # Private videos
    # - subscriber_only              # Channel subscriber-only content
    # - premium_only                 # YouTube Premium exclusive
    # - needs_auth                   # Requires authentication
```

- [ ] **Step 2: Verify build still passes**

Run: `go build ./...`
Expected: Success

- [ ] **Step 3: Commit**

```bash
git add config.example.yaml
git commit -m "docs: add exclude_availability to config.example.yaml"
```
