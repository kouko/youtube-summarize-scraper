package probes

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kouko/youtube-summarize-scraper/tui"
)

// ATTACK: A3 positive — viewport pinned at bottom, many new events arrive
// in rapid succession. The view must stay pinned at the bottom showing the
// newest event each time.
func TestAdversaryEventsPinnedBottomRapidNewEvents(t *testing.T) {
	state := tui.NewAppState()
	m := newSizedEventsModelWithState(state)

	// Pre-fill with 50 events
	for i := range 50 {
		state.AddRecentEvent(fmt.Sprintf("event-%02d", i))
	}

	// Render once to establish the viewport (pinned at bottom)
	viewContent(t, m)
	if !strings.Contains(m.View().Content, "event-49") {
		t.Error("initial view missing newest event-49")
	}

	// Now add 200 more events rapidly
	for i := 50; i < 250; i++ {
		state.AddRecentEvent(fmt.Sprintf("event-%03d", i))
		// After EACH addition, render and verify newest is visible
		after := m.View().Content
		if !strings.Contains(after, fmt.Sprintf("event-%03d", i)) {
			t.Errorf("A3 violated: after adding event-%03d, newest not visible (pinned failed); content=%q", i, after[:200])
		}
	}
}

// ATTACK: A3 negative — user has scrolled up, new events arrive.
// The viewport must NOT jump to the bottom: the newest event stays out of
// view (content-based assertion, no internal offset needed).
func TestAdversaryEventsScrolledUpManyNewEventsStaysPut(t *testing.T) {
	state := tui.NewAppState()
	m := newSizedEventsModelWithState(state)

	// Pre-fill with 200 events (viewport shows roughly the last ~20 lines)
	for i := range 200 {
		state.AddRecentEvent(fmt.Sprintf("event-%03d", i))
	}
	viewContent(t, m)

	// Scroll up 50 lines: the view now shows a window well above the newest.
	for i := 0; i < 50; i++ {
		m.Update(pressKey("up"))
	}

	// Add 300 new events while scrolled up.
	for i := 200; i < 500; i++ {
		state.AddRecentEvent(fmt.Sprintf("event-%03d", i))
	}

	// Render. If the viewport stayed put, event-499 (the newest) is NOT
	// visible — it sits far below the viewport window. A jump to bottom
	// would put event-499 in view.
	scrolled := m.View().Content
	if strings.Contains(scrolled, "event-499") {
		t.Error("A3 violated: viewport jumped to the bottom while the user was scrolled up (newest event visible)")
	}
}

// ATTACK: A3 edge case — user scrolled to the VERY TOP of a 100-event
// history (oldest visible). A new event arrives. The view must stay at the
// top showing event-000, not jump to the bottom.
func TestAdversaryEventsScrolledToTopNewEventStays(t *testing.T) {
	state := tui.NewAppState()
	m := newSizedEventsModelWithState(state)

	for i := range 100 {
		state.AddRecentEvent(fmt.Sprintf("event-%03d", i))
	}
	viewContent(t, m)

	// Scroll all the way to top
	for i := 0; i < 150; i++ {
		m.Update(pressKey("up"))
	}
	before := m.View().Content
	if !strings.Contains(before, "event-000") {
		t.Fatalf("scroll-to-top failed: event-000 not visible; content=%q", before[:200])
	}

	// New event arrives
	state.AddRecentEvent("event-100")

	// View must NOT have jumped: event-000 still visible, newest event-100
	// NOT visible (it is far below the top-pinned window).
	after := m.View().Content
	if !strings.Contains(after, "event-000") {
		t.Error("A3 violated: view jumped away from the top after a new event arrived")
	}
	if strings.Contains(after, "event-100") {
		t.Error("A3 violated: newest event jumped into view while the user was at the top")
	}
}

// ATTACK: A3 edge case — user scrolled up, then scrolled BACK DOWN to bottom
// (manually). New event should then be pinned again (view jumps to bottom).
// This tests the re-pinning logic.
func TestAdversaryEventsScrolledUpThenDownToBottomRepins(t *testing.T) {
	state := tui.NewAppState()
	m := newSizedEventsModelWithState(state)

	for i := range 50 {
		state.AddRecentEvent(fmt.Sprintf("event-%02d", i))
	}
	viewContent(t, m)

	// Scroll up 10
	for i := 0; i < 10; i++ {
		m.Update(pressKey("up"))
	}

	// Scroll down back to bottom (more than needed to clamp)
	for i := 0; i < 15; i++ {
		m.Update(pressKey("down"))
	}
	bottom := m.View().Content
	if !strings.Contains(bottom, "event-49") {
		t.Fatalf("scroll down did not return to the newest event; content=%q", bottom[:200])
	}

	// Add new event — should stay pinned at bottom (show newest)
	state.AddRecentEvent("new-event-after-repin")
	after := m.View().Content
	if !strings.Contains(after, "new-event-after-repin") {
		t.Errorf("A3 violated: after re-pinning to bottom, new event not visible; content=%q", after[:200])
	}
}

// ATTACK: A3 — mouse wheel scrolling with a full 1000-event history.
// Wheel up must reach the oldest retained line; wheel down must return to
// the newest; and a new event at the bottom must stay pinned.
func TestAdversaryEventsWheelScrollLargeHistory(t *testing.T) {
	state := tui.NewAppState()
	m := newSizedEventsModelWithState(state)

	// Add 1000 events (at cap)
	for i := range 1000 {
		state.AddRecentEvent(fmt.Sprintf("event-%04d", i))
	}
	viewContent(t, m)

	// Wheel up many times — should be able to reach the oldest
	for i := 0; i < 1000; i++ {
		m.Update(tea.MouseWheelMsg{X: 90, Y: 20, Button: tea.MouseWheelUp})
	}
	content := m.View().Content
	if !strings.Contains(content, "event-0000") {
		t.Error("wheel up did not reach oldest event-0000 in 1000-event history")
	}

	// Wheel down — should return to bottom (newest)
	for i := 0; i < 1000; i++ {
		m.Update(tea.MouseWheelMsg{X: 90, Y: 20, Button: tea.MouseWheelDown})
	}
	content = m.View().Content
	if !strings.Contains(content, "event-0999") {
		t.Error("wheel down did not return to newest event-0999")
	}

	// Now add more events while at bottom — should stay pinned
	state.AddRecentEvent("event-1000")
	content = m.View().Content
	if !strings.Contains(content, "event-1000") {
		t.Error("after wheel down to bottom, new event not pinned")
	}
}