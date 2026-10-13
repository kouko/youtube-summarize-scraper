# Acceptance Test Evidence

## Test Results Summary

All tests pass, including:
- `go test ./tui/... ./cmd/...` (TUI and CLI packages)
- `go test ./pipeline/...` (pipeline package)
- `go test ./...` (full test suite)

## Specific Test Verification

### A1: History beyond previous limit
- `TestRecentEventsRetainBeyondOldLimit` (state_test.go): Confirms 150 lines retained when limit is 1000
- `TestAdversaryEventsPanelScrollsBeyondOld100Limit` (probes/model_events_a1_test.go): Confirms user can scroll to see lines beyond old 100-line limit via TUI interface

### A2: Fixed larger history cap
- `TestRecentEventsCapDropsOldest` (state_test.go): Confirms exactly 1000 lines retained at cap, 1001 lines drops oldest
- `TestAdventEventsPanelScrollsOverNew1000Cap` (probes/model_events_a1_test.go): Confirms with 1100 total lines, oldest retained is event-0100 (lines 0-99 dropped)

### A3: Bottom pinning preserved
- `TestModelViewportKeyboardScroll` (model_test.go): Confirms ↑↓ scrolls events viewport when focused
- `TestModelWheelScrollsPanelUnderPointer` (model_test.go): Confirms wheel scrolls panel under pointer
- `TestEventsNewEventsStayPinned` (model_test.go): New events keep view pinned at bottom
- `TestEventsScrolledUpDoesNotJump` (model_test.go): Scrolled-up view doesn't jump to bottom on new events
- Adversary tests: `TestAdversaryEventsPinnedBottomRapidNewEvents`, `TestAdversaryEventsScrolledUpManyNewEventsStaysPut`, `TestAdversaryEventsScrolledToTopNewEventStays`, `TestAdversaryEventsScrolledUpThenDownToBottomRepins`

## Full Package Test Results
```
ok  	github.com/kouko/youtube-summarize-scraper/cmd	(cached)
ok  	github.com/kouko/youtube-summarize-scraper/config	(cached)
ok  	github.com/kouko/youtube-summarize-scraper/docs/loom/2026-10-13-tui-more-event-history/evidence/probes	(cached)
ok  	github.com/kouko/youtube-summarize-scraper/fetcher	(cached)
ok  	github.com/kouko/youtube-summarize-scraper/lang	(cached)
ok  	github.com/kouko/youtube-summarize-scraper/lang	(cached)
ok  	github.com/kouko/youtube-summarize-scraper/output	(cached)
ok  	github.com/kouko/youtube-summarize-scraper/pipeline	(cached)
ok  	github.com/kouko/youtube-summarize-scraper/prompts/builtin	(cached)
ok  	github.com/kouko/youtube-summarize-scraper/subtitle	(cached)
ok  	github.com/kouko/youtube-summarize-scraper/summarizer	(cached)
ok  	github.com/kouko/youtube-summarize-scraper/transcriber	(cached)
ok  	github.com/kouko/youtube-summarize-scraper/tui	(cached)
```

## Adversarial Test Results
All adversary tests pass:
- `TestAdversaryRecentEventsExactlyAtCapAndOneOver`
- `TestAdversaryRecentEventsJustUnderCap`
- `TestAdversaryRecentEventsMassivelyOverCap`
- `TestAdversaryBridgeBurstDeliversInOrderUpToCap`
- `TestAdversaryBridgeHugeSingleWrite`
- `TestAdversaryBridgeQueueMatchesStateCap`
- `TestAdversaryConcurrentHammer`
- `TestAdversaryEventsPanelScrollsBeyondOld100Limit`
- `TestAdventEventsPanelScrollsOverNew1000Cap`
- `TestAdversaryEventsStateNeverExceedsCapDuringBurst`
- `TestAdversaryEventsPinnedBottomRapidNewEvents`
- `TestAdversaryEventsScrolledUpManyNewEventsStaysPut`
- `TestAdversaryEventsScrolledToTopNewEventStays`
- `TestAdversaryEventsScrolledUpThenDownToBottomRepins`
- `TestAdversaryEventsWheelScrollLargeHistory`
- `TestAdversaryNewAppStatePreallocatesCapacityCorrectly`
- `TestAdversaryAddRecentEventFirstEventAtIndexZero`
- `TestAdversaryStateCapUsesMaxRecentEventsConstant`