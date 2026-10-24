package probes

// concern: after a run that ends by itself (batch finished or failed), the
// model's internal isRunning flag stays true while the status view reports
// Stopped — a later s must stay a no-op and the status hints must not
// advertise [s] Stop, or the TUI lies about a run that is not running.

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kouko/youtube-summarize-scraper/tui"
)

// stripAnsi removes ANSI escape sequences so UI labels can be matched on
// plain text.
func stripAnsi(s string) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		if r == '\x1b' {
			in = true
			continue
		}
		if in {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				in = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// Test_sstopkey_idle_after_selfending_run pins that pressing s after a run
// that ended by itself does nothing and the status hints drop [s] Stop — the
// UI must not keep advertising a stop key for a pipeline its own status panel
// reports as Stopped. A real run is started through StartRunMsg with a config
// path that fails to load, so the run goroutine ends itself (the app state's
// IsRunning back to false) while the model still believes it is running.
func Test_sstopkey_idle_after_selfending_run(t *testing.T) {
	state := tui.NewAppState()
	state.UpdateConfig("/nonexistent/tui-stop-key-probe.yaml", "x")
	m := tui.NewModel(state)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.Update(tui.StartRunMsg{})

	deadline := time.Now().Add(3 * time.Second)
	for state.Snapshot().IsRunning && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if state.Snapshot().IsRunning {
		t.Fatal("run goroutine never finished")
	}

	content := stripAnsi(m.View().Content)
	if !strings.Contains(content, "Status: Stopped") {
		t.Fatalf("setup: status view does not report Stopped after the run ended; content=%q", content)
	}

	if strings.Contains(content, "[s] Stop") {
		t.Error("status hints advertise [s] Stop while the status view reports Stopped")
	}

	_, cmd := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	if cmd != nil {
		t.Errorf("s after a self-ended run emitted a command %T, want a no-op", cmd())
	}
}
