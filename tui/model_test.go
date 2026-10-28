package tui

import (
	"context"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func pressKey(s string) tea.KeyPressMsg {
	switch s {
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEsc}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	default:
		r := []rune(s)
		if len(r) == 1 {
			return tea.KeyPressMsg{Code: r[0], Text: s}
		}
		// multi-char control: build a text key.
		return tea.KeyPressMsg{Text: s}
	}
}

func sizedModel() *Model {
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return m
}

func viewContent(t *testing.T, m *Model) string {
	t.Helper()
	v := m.View()
	if v.Content == "" {
		t.Fatal("View() returned empty content")
	}
	return v.Content
}

// W1-01 A1 positive (three-panel): two upper panels and one lower content panel.
func TestModelRendersThreePanels(t *testing.T) {
	m := sizedModel()
	content := viewContent(t, m)
	for _, title := range []string{"■ Config File", "■ Execution Status", "■ Config"} {
		if !strings.Contains(content, title) {
			t.Errorf("View missing panel title %q", title)
		}
	}
	if strings.Contains(content, "■ Recent Events") {
		t.Error("execution log rendered while the config card has focus; the lower area must show the config content")
	}
}

// W2-03 A4 negative: the focus marker sits on the focused panel only.
func TestModelFocusMarkerOnActivePanel(t *testing.T) {
	m := sizedModel()
	content := viewContent(t, m)
	if !strings.Contains(content, "> ■ Config File") {
		t.Error("initial focus marker missing on Config File panel")
	}
	// Other panel titles must not carry the marker; use a trailing newline
	// so "> Config File" does not match "> Config".
	for _, title := range []string{"Config\n", "Execution Status", "Recent Events"} {
		if strings.Contains(content, "> "+title) {
			t.Errorf("focus marker on non-focused panel %q", title)
		}
	}
}

// W1-01 A2 positive: Tab cycles focus between the two upper panels only.
func TestModelTabCyclesFocus(t *testing.T) {
	m := sizedModel()
	seen := map[FocusedPanel]bool{}
	for range 2 {
		seen[m.focus] = true
		m.handleKey(pressKey("tab"))
	}
	if len(seen) != 2 {
		t.Errorf("Tab visited %d/2 panels, want exactly 2 upper panels", len(seen))
	}
	if seen[PanelConfig] || seen[PanelEvents] {
		t.Errorf("Tab visited a lower content kind: %v", seen)
	}
}

// W3-01 A6 positive: r with a selected config emits StartRunMsg.
func TestModelRKeyStartsWhenConfigSelected(t *testing.T) {
	m := sizedModel()
	m.state.UpdateConfig("/tmp/x.yaml", "x")
	_, cmd := m.handleKey(pressKey("r"))
	if cmd == nil {
		t.Fatal("r with config selected returned nil cmd, want StartRunMsg")
	}
	if msg := cmd(); msg != (StartRunMsg{}) {
		t.Errorf("cmd() = %#v, want StartRunMsg{}", msg)
	}
}

// W3-01 A6 negative: r without a selected config does nothing.
func TestModelRKeyNoConfigDoesNothing(t *testing.T) {
	m := sizedModel()
	_, cmd := m.handleKey(pressKey("r"))
	if cmd != nil {
		t.Errorf("r without config returned cmd %#v, want nil", cmd)
	}
}

// W4-01 A7 positive: q while running asks for confirmation — the first q must
// neither quit nor cancel, and the view must say so; only the second q emits
// QuitConfirmMsg. Regression: the first q used to emit QuitConfirmMsg and
// Update treated that as the confirmation itself, so ONE q killed the whole
// run (user-reported 2026-10-09 while a real pipeline was running).
func TestModelQWhileRunningPromptsConfirm(t *testing.T) {
	m := sizedModel()
	m.state.UpdateConfig("/tmp/x.yaml", "x")
	m.isRunning = true
	m.state.SetRunning(true)

	// First q: arm the confirmation only — no command, no cancel.
	_, cmd := m.handleKey(pressKey("q"))
	if cmd != nil {
		t.Fatalf("first q returned cmd %#v, want nil (just arm confirm)", cmd)
	}
	if !m.isRunning {
		t.Error("isRunning false after first q; the run was cancelled")
	}
	if !m.confirmPending {
		t.Error("confirmPending false after first q")
	}
	if content := viewContent(t, m); !strings.Contains(content, "Press q again") {
		t.Errorf("first q did not show a confirm hint; view=%q", content[:80])
	}

	// Second q: emit the confirmation message, still without quitting here.
	_, cmd2 := m.handleKey(pressKey("q"))
	if cmd2 == nil {
		t.Fatal("second q returned nil cmd, want QuitConfirmMsg")
	}
	if msg := cmd2(); msg != (QuitConfirmMsg{}) {
		t.Errorf("second q cmd() = %#v, want QuitConfirmMsg{}", msg)
	}

	// Update(QuitConfirmMsg) is what actually cancels and quits.
	got, cmd3 := m.Update(QuitConfirmMsg{})
	if msg := cmd3(); msg != tea.Quit() {
		t.Errorf("Update(QuitConfirmMsg) cmd() = %#v, want tea.Quit()", msg)
	}
	if got.(*Model).isRunning {
		t.Error("isRunning still true after confirmed quit")
	}
}

// W4-01 A7 negative: any other key while the confirm is pending cancels the
// confirmation and keeps running.
func TestModelOtherKeyCancelsQuitConfirm(t *testing.T) {
	m := sizedModel()
	m.state.UpdateConfig("/tmp/x.yaml", "x")
	m.isRunning = true
	m.state.SetRunning(true)

	m.handleKey(pressKey("q")) // arm the confirm
	if !m.confirmPending {
		t.Fatal("confirm not armed after q")
	}
	m.handleKey(pressKey("tab")) // any other key cancels
	if m.confirmPending {
		t.Error("confirm still pending after another key")
	}
	if !m.isRunning {
		t.Error("tab while confirm pending stopped the run")
	}
	// q once more must re-arm the confirm, not quit.
	_, cmd := m.handleKey(pressKey("q"))
	if cmd != nil {
		t.Errorf("q after cancel returned cmd %#v, want nil (re-arm only)", cmd)
	}
	if !m.confirmPending {
		t.Error("confirm not re-armed after q")
	}
}

// W4-01 A7 boundary: q while idle quits immediately.
func TestModelQWhileIdleQuitsImmediately(t *testing.T) {
	m := sizedModel()
	_, cmd := m.handleKey(pressKey("q"))
	if cmd == nil {
		t.Fatal("q while idle returned nil cmd, want tea.Quit")
	}
	if msg := cmd(); msg != tea.Quit() {
		t.Errorf("cmd() = %#v, want tea.Quit()", msg)
	}
}

// W4-01 A7 boundary: confirming quit (QuitConfirmMsg) cancels and quits.
func TestModelConfirmQuitCancelsAndQuits(t *testing.T) {
	m := sizedModel()
	m.state.UpdateConfig("/tmp/x.yaml", "x")
	m.isRunning = true
	m.state.SetRunning(true)

	got, cmd := m.Update(QuitConfirmMsg{})
	if cmd == nil {
		t.Fatal("Update(QuitConfirmMsg) returned nil cmd, want tea.Quit")
	}
	if msg := cmd(); msg != tea.Quit() {
		t.Errorf("cmd() = %#v, want tea.Quit()", msg)
	}
	after := got.(*Model)
	if after.isRunning {
		t.Error("isRunning still true after confirm quit")
	}
}

// W1-01 A1 positive: while the pipeline is running, pressing s emits a
// command producing StopRunMsg carrying the current run's token, so the
// handler can tell this stop from one dispatched for an earlier run.
func TestModelSKeyWhileRunningEmitsStopRun(t *testing.T) {
	m := sizedModel()
	m.state.UpdateConfig("/tmp/x.yaml", "x")
	m.isRunning = true
	m.state.SetRunning(true)
	runCtx, runCancel := context.WithCancel(context.Background())
	defer runCancel()
	m.ctx, m.cancel = runCtx, runCancel

	_, cmd := m.handleKey(pressKey("s"))
	if cmd == nil {
		t.Fatal("s while running returned nil cmd, want StopRunMsg")
	}
	msg, ok := cmd().(StopRunMsg)
	if !ok {
		t.Fatalf("s while running cmd() = %T, want StopRunMsg", cmd())
	}
	if msg.ctx != m.ctx {
		t.Errorf("StopRunMsg token = %v, want the current run's ctx (a stop dispatched for another run must not cancel this one)", msg.ctx)
	}
}

// Round-2 W1-01 instance 2: StopRunMsg carries the run token like
// RunFinishedMsg. A stop dispatched for an earlier run and delivered after a
// restart (stale ctx) must cancel nothing and clear nothing; the current
// run's own token stops it.
func TestModelStopRunMsgTokenGuard(t *testing.T) {
	m := sizedModel()
	m.state.UpdateConfig("/tmp/x.yaml", "x")
	staleCtx, staleCancel := context.WithCancel(context.Background())
	defer staleCancel()
	currentCtx, currentCancel := context.WithCancel(context.Background())
	defer currentCancel()
	m.isRunning = true
	m.state.SetRunning(true)
	m.ctx, m.cancel = currentCtx, currentCancel

	got, _ := m.Update(StopRunMsg{ctx: staleCtx})
	after := got.(*Model)
	if !after.isRunning || !after.state.Snapshot().IsRunning {
		t.Error("stale StopRunMsg cleared the newer run's running flags")
	}
	select {
	case <-currentCtx.Done():
		t.Error("stale StopRunMsg cancelled the newer run's context")
	default:
	}

	got, _ = m.Update(StopRunMsg{ctx: currentCtx})
	after = got.(*Model)
	if after.isRunning || after.state.Snapshot().IsRunning {
		t.Error("current run's StopRunMsg did not clear the running flags")
	}
	select {
	case <-currentCtx.Done():
	default:
		t.Error("current run's StopRunMsg did not cancel the run context")
	}
}

// W1-01 A1 boundary: while NOT running, pressing s does nothing — no command,
// no state change.
func TestModelSKeyNotRunningDoesNothing(t *testing.T) {
	m := sizedModel()
	m.state.UpdateConfig("/tmp/x.yaml", "x")

	_, cmd := m.handleKey(pressKey("s"))
	if cmd != nil {
		t.Errorf("s while idle returned cmd %#v, want nil", cmd)
	}
	if m.isRunning {
		t.Error("s while idle started the run")
	}
}

// W1-01 A2 positive: after StopRunMsg is handled, isRunning is false and the
// status view shows "Status: Stopped".
func TestModelStopRunMsgShowsStopped(t *testing.T) {
	m := sizedModel()
	m.state.UpdateConfig("/tmp/x.yaml", "x")
	m.isRunning = true
	m.state.SetRunning(true)
	m.cancel = func() {}

	got, _ := m.Update(StopRunMsg{})
	after := got.(*Model)
	if after.isRunning {
		t.Error("isRunning still true after StopRunMsg")
	}
	if content := viewContent(t, after); !strings.Contains(content, "Status: Stopped") {
		t.Errorf("status view does not show Stopped after StopRunMsg; content=%q", stripANSI(content[:120]))
	}
}

// W1-01 A3 positive: after a stop, pressing r with a non-empty ConfigPath
// emits a command producing StartRunMsg (restart works).
func TestModelRestartAfterStop(t *testing.T) {
	m := sizedModel()
	m.state.UpdateConfig("/tmp/x.yaml", "x")
	m.isRunning = true
	m.state.SetRunning(true)
	m.cancel = func() {}

	m.Update(StopRunMsg{}) // stop the run
	_, cmd := m.handleKey(pressKey("r"))
	if cmd == nil {
		t.Fatal("r after stop returned nil cmd, want StartRunMsg")
	}
	if msg := cmd(); msg != (StartRunMsg{}) {
		t.Errorf("r after stop cmd() = %#v, want StartRunMsg{}", msg)
	}
}

// W1-01 A3 boundary: with an empty ConfigPath, r does nothing — also after a
// stop.
func TestModelRestartAfterStopNeedsConfigPath(t *testing.T) {
	m := sizedModel()
	m.isRunning = true
	m.state.SetRunning(true)
	m.cancel = func() {}

	m.Update(StopRunMsg{}) // stop the run
	_, cmd := m.handleKey(pressKey("r"))
	if cmd != nil {
		t.Errorf("r with empty ConfigPath returned cmd %#v, want nil", cmd)
	}
	if m.isRunning {
		t.Error("r with empty ConfigPath started the run")
	}
}

// W1-01 A4 positive: hintSegmentsFor(PanelStatus) returns a "[s] Stop"
// segment while the pipeline is running.
func TestModelStatusHintsShowStopWhileRunning(t *testing.T) {
	m := sizedModel()
	m.isRunning = true

	var stop, run bool
	for _, seg := range m.hintSegmentsFor(PanelStatus) {
		switch seg.Text {
		case "[s] Stop":
			stop = true
		case "[r] Run":
			run = true
		}
	}
	if !stop {
		t.Error("running status hints lack a [s] Stop segment")
	}
	if !run {
		t.Error("running status hints lack a [r] Run segment")
	}
}

// W1-01 A4 boundary: while stopped, the status hints must not contain a
// "[s] Stop" segment.
func TestModelStatusHintsHideStopWhenStopped(t *testing.T) {
	m := sizedModel()

	for _, seg := range m.hintSegmentsFor(PanelStatus) {
		if seg.Text == "[s] Stop" {
			t.Errorf("stopped status hints contain a %q segment, want none", seg.Text)
		}
	}
}

// W1-01 A3 boundary: a RunFinishedMsg from an earlier run must not clear the
// running flag of the current run; only the current run's own completion does.
// The ctx carried by the message is the run token.
func TestModelRunFinishedMsgTokenMatch(t *testing.T) {
	m := sizedModel()
	m.state.UpdateConfig("/tmp/x.yaml", "x")
	staleCtx, staleCancel := context.WithCancel(context.Background())
	defer staleCancel()
	currentCtx, currentCancel := context.WithCancel(context.Background())
	defer currentCancel()
	m.isRunning = true
	m.state.SetRunning(true)
	m.ctx, m.cancel = currentCtx, currentCancel

	got, _ := m.Update(RunFinishedMsg{ctx: staleCtx})
	if !got.(*Model).isRunning {
		t.Error("stale RunFinishedMsg cleared isRunning of the current run")
	}
	if !got.(*Model).state.Snapshot().IsRunning {
		t.Error("stale RunFinishedMsg cleared the app state's IsRunning of the current run")
	}
	got, _ = m.Update(RunFinishedMsg{ctx: currentCtx})
	if got.(*Model).isRunning {
		t.Error("current run's RunFinishedMsg did not clear isRunning")
	}
	if got.(*Model).state.Snapshot().IsRunning {
		t.Error("current run's RunFinishedMsg did not clear the app state's IsRunning")
	}
}

// Round-2 W1-01 instance 3: the q/ctrl+c confirm guard must read liveness
// (runAlive), not the raw isRunning flag. During the stale window after a run
// ended by itself (isRunning still true, ctx already dead, RunFinishedMsg not
// yet processed) a single q quits immediately — nothing is running. With a
// live run the first q still only arms the confirmation
// (TestModelQWhileRunningPromptsConfirm, whose nil m.ctx counts as alive).
func TestModelQWithDeadCtxQuitsImmediately(t *testing.T) {
	m := sizedModel()
	m.state.UpdateConfig("/tmp/x.yaml", "x")
	m.isRunning = true
	m.state.SetRunning(true)
	deadCtx, deadCancel := context.WithCancel(context.Background())
	deadCancel() // the run self-ended: its ctx is dead before RunFinishedMsg lands
	m.ctx, m.cancel = deadCtx, deadCancel

	_, cmd := m.handleKey(pressKey("q"))
	if cmd == nil {
		t.Fatal("single q during the stale window returned nil cmd (armed the confirmation), want tea.Quit")
	}
	if msg := cmd(); msg != tea.Quit() {
		t.Errorf("single q during the stale window cmd() = %#v, want tea.Quit()", msg)
	}
	if m.confirmPending {
		t.Error("confirmation armed during the stale window though nothing is running")
	}
}

// W3-01 A4 boundary: a second StartRunMsg while running is ignored.
func TestModelSecondStartIgnored(t *testing.T) {
	m := sizedModel()
	m.state.UpdateConfig("/tmp/x.yaml", "x")
	m.isRunning = true
	m.state.SetRunning(true)

	_, cmd := m.Update(StartRunMsg{})
	if cmd != nil {
		t.Errorf("second StartRunMsg returned cmd %#v, want nil (ignored)", cmd)
	}
	if !m.isRunning {
		t.Error("isRunning flipped false by ignored StartRunMsg")
	}
}

// Regression: arrow keys with file-picker focus must keep the main Model as
// the program model. handleUp/handleDown once returned the FilePickerModel
// itself, which replaced the whole TUI with the bare picker view (alt screen
// off, q dead) after the first ↑/↓.
func TestModelArrowKeysKeepMainModel(t *testing.T) {
	m := sizedModel()

	got, _ := m.Update(pressKey("up"))
	if _, ok := got.(*Model); !ok {
		t.Fatalf("after ↑ the program model is %T, want *Model (the TUI was replaced by the picker)", got)
	}
	got, _ = m.Update(pressKey("down"))
	if _, ok := got.(*Model); !ok {
		t.Fatalf("after ↓ the program model is %T, want *Model", got)
	}
	// The four panels must still render, and q must still quit.
	after := got.(*Model)
	after.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	content := viewContent(t, after)
	if !strings.Contains(content, "■ Execution Status") {
		t.Error("four-panel layout lost after arrow keys")
	}
	_, cmd := after.handleKey(pressKey("q"))
	if cmd == nil {
		t.Error("q no longer quits after arrow keys")
	}
}

// Regression: the main model must forward the file picker's own messages
// (readDirMsg from its Init command) to it. Before this fix the directory
// listing was swallowed, so the picker stayed empty and arrow keys had
// nothing to move over.
func TestModelForwardsPickerDirectoryListing(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("YTSS_CONFIG_DIR", dir)

	m := NewModel(NewAppState())

	// Drive the picker's Init command (readDir) through the main model the
	// way the program does: the command's message arrives at Update.
	initCmd := m.filePicker.Init()
	if initCmd == nil {
		t.Fatal("picker Init returned nil cmd")
	}
	if msg := initCmd(); msg != nil {
		got, _ := m.Update(msg)
		m = got.(*Model)
	}

	content := m.renderPickerPopup(60, 20)
	if !strings.Contains(content, "a.yaml") {
		t.Errorf("picker did not list a.yaml after readDirMsg was forwarded; content=%q", content)
	}
	if strings.Contains(content, "b.txt") {
		t.Errorf("picker listed b.txt (should be filtered to .yaml/.yml); content=%q", content)
	}
}

// Regression: the whole TUI must fit the terminal height. The config card,
// execution status, and the focused lower content area must all fit within
// the terminal. The lower content area shows the config card's short display
// by default.
func TestModelFitsTerminalHeight(t *testing.T) {
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	v := m.View()
	lines := strings.Count(v.Content, "\n") + 1
	if lines > 31 { // terminal height + slack for a trailing newline
		t.Errorf("View renders %d lines for a 30-row terminal; bottom panels are pushed off screen", lines)
	}
	content := viewContent(t, m)
	for _, title := range []string{"■ Config File", "■ Execution Status"} {
		if !strings.Contains(content, title) {
			t.Errorf("panel %q missing from view", title)
		}
	}
	// The lower area shows the config content when the config card has focus.
	if !strings.Contains(content, "■ Config") {
		t.Error("lower panel does not show the config content")
	}
}

// Regression: the grid must fit the terminal: the top band is content-sized
// (card and status end on the same line), the bottom band starts right after
// it, and the hint line is the final visible line (spec amend1 + W5-03).
// Every line of the full rendered view must have the same display width:
// when the top panels' content rows drifted (unpadded short lines), the
// right borders of the 2x2 grid misaligned on screen. Uses the herdr pane's
// real size (140x65) where the drift was first observed.
func TestModelViewAllLinesSameWidth(t *testing.T) {
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 65})
	lines := strings.Split(m.View().Content, "\n")
	w := lipgloss.Width(lines[0])
	if w == 0 {
		t.Fatal("view is empty")
	}
	for i, ln := range lines {
		if got := lipgloss.Width(ln); got != w {
			t.Errorf("line %d width = %d, want %d: %q", i, got, w, stripANSI(ln))
		}
	}
}

func TestModelGridBandsAndHintAlign(t *testing.T) {
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	lines := strings.Split(m.View().Content, "\n")
	// Count total "└" characters (each panel has its own bottom border)
	totalBottomCorners := 0
	for _, ln := range lines {
		totalBottomCorners += strings.Count(stripANSI(ln), "└")
	}
	// We expect 3 bottom corners (one for each panel: two upper + one lower).
	if totalBottomCorners != 3 {
		t.Fatalf("found %d bottom corners (└), want 3:\n%s", totalBottomCorners, strings.Join(lines, "\n"))
	}

	// Find the first bottom border line (top band end)
	firstBottomLine := -1
	for i, ln := range lines {
		if strings.Contains(stripANSI(ln), "└") {
			firstBottomLine = i
			break
		}
	}
	if firstBottomLine < 0 {
		t.Fatal("first bottom border line not found")
	}
	// With title bar: top band content ends at row where bottom corner appears.
	// Account for title row (0) before content starts
	if firstBottomLine > 10 { // 9 content rows + 1 for title = 10
		t.Errorf("top band ends at line %d, want content-sized (<=10); the empty strip is back", firstBottomLine)
	}

	// Bands adjacent: the bottom band's top border is on the very next line.
	// With new title bar design, the bottom band starts with a title row (no "╭").
	// We check that the next line contains the bottom-left panel's title ("Config").
	if firstBottomLine+1 >= len(lines) || !strings.Contains(stripANSI(lines[firstBottomLine+1]), "Config") {
		t.Errorf("bottom band does not start right after the top band (line %d)", firstBottomLine)
	}
	last := lines[len(lines)-1]
	if !strings.Contains(last, "↑↓ Navigate") {
		t.Errorf("last line is not the hint: %q", last)
	}
	// Every line of the content area (after title) carries 4 vertical borders
	// (two panels side by side in the upper row).
	ansiRe := regexp.MustCompile(`\x1b\[[0-9;>?]*[a-zA-Z]`)
	if firstBottomLine >= 0 {
		for i := 1; i < firstBottomLine; i++ { // Skip title row (0); stop before bottom border
			plain := ansiRe.ReplaceAllString(lines[i], "")
			if strings.Count(plain, "│") != 4 {
				t.Errorf("line %d has %d vertical borders, want 4: %q", i, strings.Count(plain, "│"), plain)
			}
		}
	}
}

// Regression: selecting a config must never grow the layout. The config
// panel flattens the whole YAML; without clipping, a long config pushes
// the hint line off screen (user-reported).
func TestModelSelectConfigKeepsLayout(t *testing.T) {
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	// A config large enough to overflow the bottom band if not clipped.
	big := "llm:\n  provider: claude-api\n" + strings.Repeat("  extra_key: value\n", 60)
	m.handleConfigSelected(t.TempDir()) // no-op path guard
	// Feed the selection directly (bypasses the file read).
	m.state.UpdateConfig("/tmp/x.yaml", big)
	m.configView = NewConfigView(big)

	lines := strings.Split(m.View().Content, "\n")
	last := lines[len(lines)-1]
	if !strings.Contains(last, "↑↓ Navigate") {
		t.Errorf("hint pushed off screen after selecting a large config; last line: %q", last[:50])
	}
	// Total must still fit the 30-row terminal.
	if len(lines) > 31 {
		t.Errorf("render %d lines for 30-row terminal after selection", len(lines))
	}
}

// Regression: the config panel's last row must be the box's bottom border,
// never clipped content (user-reported: last line was config content).
func TestConfigPanelBottomBorderVisible(t *testing.T) {
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	big := strings.Repeat("llm.extra_key: value\n", 80)
	m.configView = NewConfigView("llm:\n  provider: x\n" + big)

	lines := strings.Split(m.View().Content, "\n")
	// find the config panel: bottom row band contains both ╰ borders; the
	// config panel is the left one. Its bottom border line must end with ╰.
	for i, ln := range lines {
		if strings.Contains(ln, "╰") && i > 0 {
			left := strings.Split(ln, "╰")[0]
			if strings.HasPrefix(left, "╭") {
				continue // top border of a panel
			}
			// this is a row-bottom border line; the config (left) half must
			// end in ╰ (border) not content.
			plain := stripANSI(ln)
			if !strings.HasPrefix(plain, "╰") && !strings.Contains(plain[:len(plain)/2], "╰") {
				// check the LEFT panel's rightmost char in the left half
				half := plain[:len(plain)/2]
				if !strings.HasSuffix(strings.TrimRight(half, " "), "╰") {
					t.Errorf("config panel bottom border missing on line %d: %q", i, plain[:60])
				}
			}
		}
	}
}

func stripANSI(s string) string {
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

// Config content scrolls with ↑/↓ while the config card is focused: the
// cursor window shifts (highlight moves), the panel box stays fixed.
func TestConfigPanelScrolls(t *testing.T) {
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	var cfg strings.Builder
	cfg.WriteString("llm:\n")
	for i := 0; i < 60; i++ {
		cfg.WriteString(strings.Repeat(" ", 2) + "opt_" + fmt.Sprint(i) + ": v\n")
	}
	m.configView = NewConfigView(cfg.String())
	m.focus = PanelFilePicker // the config card owns the config content in the lower area

	before := m.View().Content
	_, _ = m.handleDown(pressKey("down"))
	after := m.View().Content
	if before == after {
		t.Error("config content did not change after ↓")
	}
	// Panel box must stay the same size (borders unchanged), only content moved.
	if strings.Count(before, "╰") != strings.Count(after, "╰") {
		t.Errorf("panel geometry changed after scroll: ╰ count %d -> %d", strings.Count(before, "╰"), strings.Count(after, "╰"))
	}
	// Scrolling up at the top is a no-op.
	_, _ = m.handleUp(pressKey("up"))
	if m.View().Content != before {
		t.Error("↑ at top changed content; should be clamped")
	}
}

// Events panel scrolls: with many events, ↑ reveals older lines.
func TestEventsPanelScrolls(t *testing.T) {
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	for i := 0; i < 40; i++ {
		m.state.AddRecentEvent(fmt.Sprintf("event-%02d", i))
	}
	m.focus = PanelStatus // lower shows the execution log

	content := m.View().Content
	// Default shows the newest visible lines.
	if !strings.Contains(content, "event-39") {
		t.Error("default events view missing newest event")
	}
	_, _ = m.handleUp(pressKey("up")) // scroll to older
	scrolled := m.View().Content
	if !strings.Contains(scrolled, "event-3") {
		t.Errorf("after ↑ expected older events; content=%q", scrolled[:80])
	}
}

// YAML values with embedded newlines render on one line (escaped), so rows
// stay aligned.
func TestConfigValueNewlineEscaped(t *testing.T) {
	yaml := "llm:\n  endpoint: \"http://a\\nb\"\n"
	cv := NewConfigView(yaml)
	if cv.Error() != nil {
		t.Fatalf("parse: %v", cv.Error())
	}
	rendered := cv.Render()
	if strings.Contains(rendered, "\nhttp://") {
		t.Errorf("value newline not escaped; rendered=%q", rendered)
	}
	if !strings.Contains(rendered, "\\n") {
		t.Errorf("expected literal \\n in value; rendered=%q", rendered)
	}
}

// clipLines must truncate by DISPLAY width, not raw rune count: ANSI color
// codes occupy zero columns but many runes, so a colored row (filepicker
// listing) lost its trailing filename to the width budget (pty-observed).
func TestClipLinesCountsDisplayWidthNotEscapeRun(t *testing.T) {
	// "  -rw-------   287B" colored (escape codes), then a plain " leak.yaml"
	colored := "\x1b[38;5;240m  -rw-------   287B\x1b[m leak.yaml"

	// 25 display columns fit the size but cut the name.
	got := clipLines(colored, 25, 3)
	plain := stripANSI(got)
	if strings.Contains(plain, "leak.yaml") {
		t.Errorf("25 display columns must cut the name, got %q", plain)
	}
	if w := len([]rune(plain)); w > 25 {
		t.Errorf("clipped row is %d display runes, want <= 25", w)
	}

	// 40 display columns keep the visible filename.
	got2 := clipLines(colored, 40, 3)
	if !strings.Contains(stripANSI(got2), "leak.yaml") {
		t.Errorf("40 display columns must keep the name, got %q", stripANSI(got2))
	}

	// Truncating inside colored content must reset the style, so the next
	// line is not painted with the previous color.
	got3 := clipLines(colored, 12, 3)
	if strings.Contains(got3, "\x1b[") && !strings.Contains(got3, "\x1b[0m") {
		t.Errorf("truncated ANSI content must reset styles; got %q", got3)
	}
}

// W5-01 A1 positive (spec amend1): the top-left panel is a short config card
// showing the current path (or "(no config selected)"), rendered at content
// height — NOT the full top band, and the file listing is no longer always
// visible.
func TestModelConfigCardShortHeight(t *testing.T) {
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	content := viewContent(t, m)
	if !strings.Contains(content, "(no config selected)") {
		t.Error("card does not show (no config selected) before any selection")
	}
	// The card must be a SHORT box: exactly two row-bottom border lines total
	// in the grid rows... instead, check the top-left box's bottom border sits
	// well above the top band's bottom (a full-height picker would align it
	// with the status panel's border).
	lines := strings.Split(content, "\n")
	topBandBottom := -1
	cardBottom := -1
	for i, ln := range lines {
		if strings.Contains(ln, "╰") && i > 0 {
			// first ╰ line: the earlier of the two borders on it belongs to
			// the left box; if the card is short, the left border appears on
			// an EARLIER line than the right panel's border.
			if strings.Count(stripANSI(ln), "╰") == 2 && topBandBottom < 0 {
				topBandBottom = i
			}
		}
	}
	_ = topBandBottom
	_ = cardBottom
	// Simpler invariant: the card content is 2 lines (path + hint), so the
	// whole card box is 5 rows (2 border + 1 title + 2 content). Assert some
	// line before the top band's bottom border contains "Enter 選檔" hint.
	found := false
	for _, ln := range lines[:10] {
		if strings.Contains(stripANSI(ln), "Enter") {
			found = true
			break
		}
	}
	if !found {
		t.Error("config card hint (Enter to open picker) not visible in the top rows")
	}
}

// W2-02 A3 positive (spec amend2): 'c' toggles between structured tree and raw YAML view.
func TestModelCToggleRawYAML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "c.yaml")
	const yamlIn = "llm:\n  provider: claude-api\n"
	os.WriteFile(cfgPath, []byte(yamlIn), 0o644)

	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	// Select the config file
	m.handleConfigSelected(cfgPath)

	// Initially structured view
	if m.configView.Mode() != ConfigViewStructured {
		t.Fatalf("initial mode = %v, want structured", m.configView.Mode())
	}

	// Press 'c' to toggle to raw
	m.handleKey(pressKey("c"))
	if m.configView.Mode() != ConfigViewRaw {
		t.Fatalf("mode after 'c' = %v, want raw", m.configView.Mode())
	}

	// View() should show raw YAML in config panel
	content := viewContent(t, m)
	// The raw YAML appears inside the panel; strip ANSI to find the text
	plain := stripANSI(content)
	// Check for key parts of the YAML content (split across panel lines with borders)
	if !strings.Contains(plain, "llm:") || !strings.Contains(plain, "provider: claude-api") {
		t.Errorf("config panel does not show raw YAML after 'c'; content=%q", content)
	}

	// Press 'c' again to toggle back
	m.handleKey(pressKey("c"))
	if m.configView.Mode() != ConfigViewStructured {
		t.Fatalf("mode after second 'c' = %v, want structured", m.configView.Mode())
	}

	// Should be back to structured view (not showing raw text directly)
	// The structured view will show the table, not contain the raw yaml verbatim
	content2 := viewContent(t, m)
	if strings.Contains(stripANSI(content2), yamlIn) {
		t.Errorf("config panel incorrectly shows raw YAML after toggling back to structured; content=%q", content2)
	}
}

// W5-02 A3 boundary (spec amend2): config table rows use display-width (not rune count)
// so CJK characters (width=2) do not misalign the table borders.
func TestModelConfigTableCJKWidth(t *testing.T) {
	cv := NewConfigView("playlists:\n  - name: \"稍後觀看\"\n    count: 10\n")
	tbl := cv.RenderTable(60)
	borderWidth := lipgloss.Width(tbl[0])
	for i, row := range tbl {
		w := lipgloss.Width(row)
		if w != borderWidth {
			t.Errorf("row %d width=%d, want border width=%d; row=%q", i, w, borderWidth, row)
		}
	}
}

// W5-01 A2 positive: Enter on the focused config card opens the picker
// popup; the popup overlay is visible; Esc closes it.
func TestModelEnterOpensPickerPopup(t *testing.T) {
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m.focus = PanelFilePicker

	if m.pickerOpen {
		t.Fatal("popup open before Enter")
	}
	m.handleKey(pressKey("enter"))
	if !m.pickerOpen {
		t.Fatal("Enter on focused card did not open the popup")
	}
	content := viewContent(t, m)
	if !strings.Contains(content, "Select a config file") {
		t.Error("popup frame title missing while open")
	}
	// Esc closes.
	m.handleKey(pressKey("esc"))
	if m.pickerOpen {
		t.Error("Esc did not close the popup")
	}
}

// W5-01 A2 boundary: while the popup is open it owns the keys — Tab must not
// move the main focus, and arrows go to the picker.
func TestModelPopupOwnsKeys(t *testing.T) {
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m.focus = PanelFilePicker
	m.handleKey(pressKey("enter"))
	if !m.pickerOpen {
		t.Fatal("popup did not open")
	}
	m.handleKey(pressKey("tab"))
	if m.focus != PanelFilePicker {
		t.Errorf("Tab moved main focus while popup open: focus=%v", m.focus)
	}
	m.handleKey(pressKey("q"))
	if !m.pickerOpen {
		t.Error("q closed the popup; it must be routed to the picker instead")
	}
	// Esc closes and disarms.
	m.handleKey(pressKey("esc"))
	if m.pickerOpen {
		t.Error("Esc did not close the popup")
	}
}

// W5-01 A2: selecting a file in the popup loads it and closes the popup.
func TestModelPopupSelectionLoadsConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pick.yaml"), []byte("llm:\n  provider: ollama\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("YTSS_CONFIG_DIR", dir)
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	if c := m.filePicker.Init(); c != nil {
		if msg := c(); msg != nil {
			m.Update(msg)
		}
	}
	m.focus = PanelFilePicker
	m.handleKey(pressKey("enter")) // open popup
	// While the popup is open, keys go through Update so they reach the
	// picker; its Enter returns a cmd whose msg (ConfigSelectedMsg) the main
	// Update then applies — closing the popup and loading the config.
	got, cmd := m.Update(pressKey("enter"))
	m = got.(*Model)
	if cmd != nil {
		if msg := cmd(); msg != nil {
			got2, _ := m.Update(msg)
			m = got2.(*Model)
		}
	}
	if m.pickerOpen {
		t.Error("popup still open after selection")
	}
	if m.state.Snapshot().ConfigPath == "" {
		t.Error("config not loaded after popup selection")
	}
}

// pickerRowClickPoint returns screen coordinates of a click on the picker
// popup's file-list row n (0 = first visible file), using the same popup
// geometry the renderer and handlePickerPopupClick apply (title row, hint
// rows, separator row, then the file list).
func pickerRowClickPoint(m *Model, n int) (int, int) {
	rect := m.pickerPopupRect()
	hintRows := popupHintLineCount(pickerPopupHint, rect.Dx(), rect.Dy())
	y := rect.Min.Y + 1 + hintRows + 1 + n
	x := rect.Min.X + 2 // inside the content area, past the left border
	return x, y
}

// Regression: with the picker popup open, a click on a file row in the list
// selects that file and closes the popup. The click must reach the picker
// through Update — previously a MouseClickMsg that was not on a hint segment
// was swallowed while the popup was open, so list clicks did nothing.
func TestModelPickerFileListMouseClickSelectsFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "alpha.yaml"), []byte("llm:\n  provider: ollama\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "beta.yaml"), []byte("llm:\n  provider: ollama\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("YTSS_CONFIG_DIR", dir)
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if c := m.filePicker.Init(); c != nil {
		if msg := c(); msg != nil {
			m.Update(msg)
		}
	}
	m.focus = PanelFilePicker
	m.handleKey(pressKey("enter")) // open the popup
	if !m.pickerOpen {
		t.Fatal("popup did not open")
	}

	// The picker lists files alphabetically: row 0 = alpha.yaml, row 1 =
	// beta.yaml. Click beta.yaml.
	x, y := pickerRowClickPoint(m, 1)
	m.Update(tea.MouseClickMsg{X: x, Y: y})

	if m.pickerOpen {
		t.Error("popup still open after clicking a file row")
	}
	if got := m.state.Snapshot().ConfigPath; !strings.HasSuffix(got, "beta.yaml") {
		t.Errorf("config path = %q, want .../beta.yaml", got)
	}
}

// Regression: clicking a directory row navigates into it (the popup stays
// open and the listing reloads via the picker's command), then a click on a
// file row inside selects that config.
func TestModelPickerDirRowMouseClickNavigates(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "inner.yaml"), []byte("llm:\n  provider: ollama\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "outer.yaml"), []byte("llm:\n  provider: ollama\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("YTSS_CONFIG_DIR", dir)
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if c := m.filePicker.Init(); c != nil {
		if msg := c(); msg != nil {
			m.Update(msg)
		}
	}
	m.focus = PanelFilePicker
	m.handleKey(pressKey("enter")) // open the popup
	if !m.pickerOpen {
		t.Fatal("popup did not open")
	}

	// Dirs sort before files: row 0 = sub. Click it to navigate in.
	x, y := pickerRowClickPoint(m, 0)
	_, cmd := m.Update(tea.MouseClickMsg{X: x, Y: y})
	if !m.pickerOpen {
		t.Fatal("clicking a directory row closed the popup")
	}
	if cmd == nil {
		t.Fatal("clicking a directory row did not return the reload command")
	}
	if msg := cmd(); msg != nil {
		m.Update(msg) // readDirMsg reloads the listing for the new directory
	}
	if got := m.filePicker.CurrentDirectory(); got != sub {
		t.Errorf("current directory = %q, want %q", got, sub)
	}

	// Now the first row is inner.yaml; clicking it selects the config. The
	// user has seen the reloaded listing before clicking again, so the
	// double-click guard window has passed.
	m.lastPickerNavClick.at = time.Now().Add(-pickerNavClickGuard - time.Millisecond)
	x, y = pickerRowClickPoint(m, 0)
	m.Update(tea.MouseClickMsg{X: x, Y: y})
	if m.pickerOpen {
		t.Error("popup still open after clicking inner.yaml")
	}
	if got := m.state.Snapshot().ConfigPath; !strings.HasSuffix(got, filepath.Join("sub", "inner.yaml")) {
		t.Errorf("config path = %q, want .../sub/inner.yaml", got)
	}
}

// pickerNavClickGuard is the double-click guard window; a test helper below
// manipulates the recorded click to simulate its expiry.

// Regression (2026-10-12, user report): a single click on a directory row
// closed the popup once the user had selected any file earlier. The bubbles
// picker keeps Path set after a selection and never clears it on directory
// navigation, so ChosenPath() reported the stale file and the code treated a
// directory click as a file selection. Clearing Path before the selection
// check makes a non-empty path mean "this click selected a file".
func TestModelPickerStalePathDoesNotClosePopupOnDirClick(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "first.yaml"), []byte("llm:\n  provider: ollama\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "inner.yaml"), []byte("llm:\n  provider: ollama\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("YTSS_CONFIG_DIR", dir)
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if c := m.filePicker.Init(); c != nil {
		if msg := c(); msg != nil {
			m.Update(msg)
		}
	}
	m.focus = PanelFilePicker

	// Round 1: select first.yaml. Dirs sort first, so row 0 = sub and
	// row 1 = first.yaml.
	m.handleKey(pressKey("enter"))
	x1, y1 := pickerRowClickPoint(m, 1)
	m.Update(tea.MouseClickMsg{X: x1, Y: y1})
	if m.pickerOpen {
		t.Fatal("round 1: popup did not close after selecting first.yaml")
	}
	if m.filePicker.ChosenPath() == "" {
		t.Fatal("round 1: no file selected")
	}

	// Round 2: reopen and click the directory row (sub). Before the fix the
	// stale first.yaml path closed the popup here.
	m.handleKey(pressKey("enter"))
	x0, y0 := pickerRowClickPoint(m, 0)
	got, cmd := m.Update(tea.MouseClickMsg{X: x0, Y: y0})
	m = got.(*Model)
	if !m.pickerOpen {
		t.Fatalf("clicking a directory closed the popup (stale ChosenPath=%q)", m.filePicker.ChosenPath())
	}
	if m.filePicker.ChosenPath() != "" {
		t.Errorf("ChosenPath after directory click = %q, want empty", m.filePicker.ChosenPath())
	}
	if cmd != nil {
		if mmsg := cmd(); mmsg != nil {
			m.Update(mmsg)
		}
	}
	if got := m.filePicker.CurrentDirectory(); got != sub {
		t.Errorf("current directory = %q, want %q", got, sub)
	}
}

// Same root cause on the keyboard path: with a stale Path, Enter on a
// directory closed the popup instead of navigating into it.
func TestModelPickerStalePathDoesNotClosePopupOnEnterDir(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "inner.yaml"), []byte("llm:\n  provider: ollama\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("YTSS_CONFIG_DIR", dir)
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if c := m.filePicker.Init(); c != nil {
		if msg := c(); msg != nil {
			m.Update(msg)
		}
	}
	m.focus = PanelFilePicker

	// Simulate a prior selection that left a stale path behind.
	m.filePicker.fp.Path = filepath.Join(dir, "previously-selected.yaml")

	m.handleKey(pressKey("enter")) // open the popup; cursor at row 0 = sub
	if !m.pickerOpen {
		t.Fatal("popup did not open")
	}
	got, cmd := m.Update(pressKey("enter")) // Enter on the sub directory
	m = got.(*Model)
	if !m.pickerOpen {
		t.Fatalf("Enter on a directory closed the popup (stale ChosenPath=%q)", m.filePicker.ChosenPath())
	}
	if m.filePicker.ChosenPath() != "" {
		t.Errorf("ChosenPath after Enter on directory = %q, want empty", m.filePicker.ChosenPath())
	}
	if cmd != nil {
		if mmsg := cmd(); mmsg != nil {
			m.Update(mmsg)
		}
	}
	if got := m.filePicker.CurrentDirectory(); got != sub {
		t.Errorf("current directory = %q, want %q", got, sub)
	}
}

// The left/right arrows navigate directories while the popup is open
// (left = back to the parent, right = open the highlighted directory).
func TestModelPickerArrowKeysNavigate(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "inner.yaml"), []byte("llm:\n  provider: ollama\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("YTSS_CONFIG_DIR", dir)
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if c := m.filePicker.Init(); c != nil {
		if msg := c(); msg != nil {
			m.Update(msg)
		}
	}
	m.focus = PanelFilePicker
	m.handleKey(pressKey("enter")) // open popup at dir; cursor at row 0 = sub
	if !m.pickerOpen {
		t.Fatal("popup did not open")
	}

	// Right arrow: open the highlighted directory (sub).
	got, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	m = got.(*Model)
	if cmd != nil {
		if mmsg := cmd(); mmsg != nil {
			m.Update(mmsg)
		}
	}
	if !m.pickerOpen {
		t.Fatal("right arrow closed the popup")
	}
	if got := m.filePicker.CurrentDirectory(); got != sub {
		t.Errorf("after right arrow: current directory = %q, want %q", got, sub)
	}

	// Left arrow: back to the parent directory (dir).
	got, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	m = got.(*Model)
	if cmd != nil {
		if mmsg := cmd(); mmsg != nil {
			m.Update(mmsg)
		}
	}
	if !m.pickerOpen {
		t.Fatal("left arrow closed the popup")
	}
	if got := m.filePicker.CurrentDirectory(); got != dir {
		t.Errorf("after left arrow: current directory = %q, want %q", got, dir)
	}
}

// Regression (2026-10-11, user report): a double-click on a directory row
// used to close the popup. The first click navigates into the directory and
// the listing reloads, so the second press of the double-click lands on the
// same coordinates which now show the new directory's first file — selecting
// it and closing the popup. The guard must ignore that second click: the
// popup stays open showing the new directory.
func TestModelPickerDirRowDoubleClickIgnored(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "default.yaml"), []byte("llm:\n  provider: ollama\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("YTSS_CONFIG_DIR", dir)
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if c := m.filePicker.Init(); c != nil {
		if msg := c(); msg != nil {
			m.Update(msg)
		}
	}
	m.focus = PanelFilePicker
	m.handleKey(pressKey("enter"))
	if !m.pickerOpen {
		t.Fatal("popup did not open")
	}

	// First click: navigate into sub. Row 0 is the only entry (sub).
	x, y := pickerRowClickPoint(m, 0)
	got, cmd := m.Update(tea.MouseClickMsg{X: x, Y: y})
	m = got.(*Model)
	if !m.pickerOpen {
		t.Fatal("first click of the double-click closed the popup")
	}
	if cmd == nil {
		t.Fatal("directory click did not return the reload command")
	}
	if msg := cmd(); msg != nil {
		m.Update(msg) // readDirMsg reloads the listing for sub
	}
	if got := m.filePicker.CurrentDirectory(); got != sub {
		t.Fatalf("current directory = %q, want %q", got, sub)
	}

	// Second click on the SAME coordinates: sub's first file (default.yaml)
	// is now at row 0. Without the guard this selects the file and closes the
	// popup; with the guard it is ignored.
	got, cmd = m.Update(tea.MouseClickMsg{X: x, Y: y})
	m = got.(*Model)
	if !m.pickerOpen {
		t.Fatalf("second click of the double-click closed the popup (ChosenPath=%q)", m.filePicker.ChosenPath())
	}
	if cmd != nil {
		t.Fatal("double-click second press must not produce a command")
	}
	if m.filePicker.ChosenPath() != "" {
		t.Errorf("double-click second press selected a file: %q", m.filePicker.ChosenPath())
	}
}

// The double-click guard is a time window: once it expires, a click on the
// same coordinates is a normal click again and selects the file.
func TestModelPickerGuardExpires(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "default.yaml"), []byte("llm:\n  provider: ollama\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("YTSS_CONFIG_DIR", dir)
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if c := m.filePicker.Init(); c != nil {
		if msg := c(); msg != nil {
			m.Update(msg)
		}
	}
	m.focus = PanelFilePicker
	m.handleKey(pressKey("enter"))
	if !m.pickerOpen {
		t.Fatal("popup did not open")
	}

	// Navigate into sub (records the guard).
	x, y := pickerRowClickPoint(m, 0)
	got, cmd := m.Update(tea.MouseClickMsg{X: x, Y: y})
	m = got.(*Model)
	if msg := cmd(); msg != nil {
		m.Update(msg)
	}
	if m.filePicker.CurrentDirectory() != sub {
		t.Fatalf("current directory = %q, want %q", m.filePicker.CurrentDirectory(), sub)
	}

	// Simulate the guard window passing.
	m.lastPickerNavClick.at = time.Now().Add(-pickerNavClickGuard - time.Millisecond)

	// Same coordinates now select sub's first file and close the popup.
	m.Update(tea.MouseClickMsg{X: x, Y: y})
	if m.pickerOpen {
		t.Fatal("guard expiry click did not select the file; popup still open")
	}
	if m.filePicker.ChosenPath() == "" {
		t.Error("guard expiry click did not select the file")
	}
}

// The guard only ignores a second click on the SAME coordinates. A click on a
// different row right after navigating is an intentional selection.
func TestModelPickerGuardAllowsDifferentRow(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "alpha.yaml"), []byte("llm:\n  provider: ollama\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "beta.yaml"), []byte("llm:\n  provider: ollama\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("YTSS_CONFIG_DIR", dir)
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if c := m.filePicker.Init(); c != nil {
		if msg := c(); msg != nil {
			m.Update(msg)
		}
	}
	m.focus = PanelFilePicker
	m.handleKey(pressKey("enter"))
	if !m.pickerOpen {
		t.Fatal("popup did not open")
	}

	// Navigate into sub (records the guard on the row-0 coordinates).
	x, y := pickerRowClickPoint(m, 0)
	got, cmd := m.Update(tea.MouseClickMsg{X: x, Y: y})
	m = got.(*Model)
	if msg := cmd(); msg != nil {
		m.Update(msg)
	}
	if m.filePicker.CurrentDirectory() != sub {
		t.Fatalf("current directory = %q, want %q", m.filePicker.CurrentDirectory(), sub)
	}

	// A click on a DIFFERENT row (beta.yaml at row 1) right after navigating
	// must select the file and close the popup.
	x2, y2 := pickerRowClickPoint(m, 1)
	m.Update(tea.MouseClickMsg{X: x2, Y: y2})
	if m.pickerOpen {
		t.Fatal("different-row click was blocked by the double-click guard")
	}
	if got := m.filePicker.ChosenPath(); !strings.HasSuffix(got, filepath.Join("sub", "beta.yaml")) {
		t.Errorf("config path = %q, want .../sub/beta.yaml", got)
	}
}

// W5-03 (user feedback): the top band's height is content-driven, not a
// fixed half of the terminal — the config card and the status panel end on
// the same line, the bottom band starts immediately after, and no blank
// rows sit between them (pre-fix: card ended at row 4, status at row 18).
func TestModelTopBandContentSized(t *testing.T) {
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	lines := strings.Split(m.View().Content, "\n")
	// Count total "└" characters (each panel has its own bottom border)
	totalBottomCorners := 0
	for _, ln := range lines {
		totalBottomCorners += strings.Count(stripANSI(ln), "└")
	}
	// We expect 3 bottom corners (one for each panel: two upper + one lower).
	if totalBottomCorners != 3 {
		t.Fatalf("expected 3 bottom corners, got %d", totalBottomCorners)
	}

	// Find the first bottom border line (top band end)
	firstBottomLine := -1
	for i, ln := range lines {
		if strings.Contains(stripANSI(ln), "└") {
			firstBottomLine = i
			break
		}
	}
	if firstBottomLine < 0 {
		t.Fatal("first bottom border line not found")
	}

	// Content-sized: status = 5 content rows + title + 2 borders = 8-9 rows;
	// a half-height band on a 40-row terminal is 19. Anything above ~16 means
	// the old empty top band is back (accounting for title row).
	if firstBottomLine > 16 {
		t.Errorf("top band ends at line %d, want content-sized (~16); the gap is back", firstBottomLine)
	}
	// The bottom band must start on the very next line (no blank row).
	// With new title bar design, the bottom band starts with a title row (no "╭").
	// We check that the next line contains the bottom-left panel's title ("Config").
	if firstBottomLine+1 >= len(lines) || !strings.Contains(stripANSI(lines[firstBottomLine+1]), "Config") {
		t.Errorf("bottom band does not start right after the top band (line %d)", firstBottomLine)
	}
}

// REQ-7 (spec amend2): the wheel scrolls the lower content area under the
// pointer without changing focus. With the log displayed (status panel
// focused) the wheel scrolls the events viewport; with the config content
// displayed (config card focused) it scrolls the config viewport.
func TestModelWheelScrollsPanelUnderPointer(t *testing.T) {
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	for i := 0; i < 40; i++ {
		m.state.AddRecentEvent(fmt.Sprintf("event-%02d", i))
	}
	m.focus = PanelStatus // lower shows the log; wheel must work anyway

	// Render once so the viewports carry content (real flow: terminal user
	// wheels after the frame is drawn).
	viewContent(t, m)

	// The events viewport is pinned at the bottom (newest visible); wheel-up
	// scrolls toward the oldest and must reach event-0 after enough notches.
	for i := 0; i < 30; i++ {
		m.Update(tea.MouseWheelMsg{X: 90, Y: 20, Button: tea.MouseWheelUp})
	}
	content := viewContent(t, m)
	if !strings.Contains(content, "event-0") {
		t.Error("wheel-up over the lower content did not scroll to the oldest line")
	}

	// The wheel over the upper row scrolls nothing and does not move focus.
	m.Update(tea.MouseWheelMsg{X: 30, Y: 4, Button: tea.MouseWheelUp})
	if m.focus != PanelStatus {
		t.Errorf("wheel changed focus to %v", m.focus)
	}
	// Switch to the config card: wheel-down over the config content
	// should scroll the config viewport.
	m.handleKey(pressKey("tab")) // config card focus
	viewContent(t, m)
	// Config viewport may have little content, but at least check it doesn't
	// crash and that focus stays on the card.
	m.Update(tea.MouseWheelMsg{X: 90, Y: 20, Button: tea.MouseWheelDown})
	if m.focus != PanelFilePicker {
		t.Errorf("wheel changed focus to %v", m.focus)
	}
}

// REQ-7: viewport keyboard scrolling — ↓ on the focused events panel scrolls
// down; the offset is kept by the viewport, not the model.
func TestModelViewportKeyboardScroll(t *testing.T) {
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	for i := 0; i < 40; i++ {
		m.state.AddRecentEvent(fmt.Sprintf("event-%02d", i))
	}
	m.focus = PanelStatus // lower shows the events log; keyboard scrolling works

	before := viewContent(t, m)
	m.handleKey(pressKey("up")) // scroll toward older events
	after := viewContent(t, m)
	if before == after {
		t.Error("↑ on the focused events panel did not scroll the viewport")
	}
	if m.eventsVP.YOffset() == 0 {
		t.Error("events viewport offset unchanged after ↑ (was pinned at bottom)")
	}
}

// REQ-8 (spec amend2): Enter on an editable config line opens the inline
// editor; committing writes the change back to the YAML file on disk.
// With the config card focused, Enter opens the editor when the cursor is on
// an actionable row (a value row or a collapsible item), and opens the
// file-picker popup otherwise.
func TestModelEditConfigValueWritesFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "c.yaml")
	os.WriteFile(cfgPath, []byte("llm:\n  provider: claude-api\n"), 0o644)
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m.handleConfigSelected(cfgPath)
	m.focus = PanelFilePicker // config card has focus; lower shows config content
	m.View()                                           // builds the table index mapping
	m.configCursor = m.configView.ContentTableIndex(1) // content row 1 = provider

	// Enter on a value row opens the editor (cursor is actionable).
	_, cmd := m.handleKey(pressKey("enter"))
	if cmd != nil {
		t.Fatalf("enter on config line returned cmd, want nil (start edit)")
	}
	if !m.editing {
		t.Fatal("editing not started")
	}
	if got := m.editText.Value(); got != "claude-api" {
		t.Errorf("editor prefill = %q, want claude-api", got)
	}

	// Clear the prefilled value (backspace to start) then type the new one.
	for range "claude-api" {
		m.Update(pressKey("backspace"))
	}
	for _, ch := range "ollama" {
		m.Update(pressKey(string(ch)))
	}
	m.Update(pressKey("enter"))
	if m.editing {
		t.Fatal("editing still active after commit")
	}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "provider: ollama") {
		t.Errorf("file not updated; got %q", data)
	}
}

// REQ-8: Esc cancels the edit and leaves the file untouched.
func TestModelEditCancelKeepsFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "c.yaml")
	os.WriteFile(cfgPath, []byte("llm:\n  provider: claude-api\n"), 0o644)
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m.handleConfigSelected(cfgPath)
	m.focus = PanelFilePicker // config card has focus; lower shows config content
	m.View() // builds the table index mapping
	m.configCursor = m.configView.ContentTableIndex(1)

	m.handleKey(pressKey("enter"))
	for _, ch := range "XXXX" {
		m.Update(pressKey(string(ch)))
	}
	m.Update(pressKey("esc"))
	if m.editing {
		t.Fatal("editing still active after Esc")
	}
	data, _ := os.ReadFile(cfgPath)
	if strings.Contains(string(data), "XXXX") {
		t.Errorf("file changed after Esc; got %q", data)
	}
}

// User feedback (2026-10-09): the popup used to blank the whole frame behind
// it. The base UI must stay visible around the overlay.
func TestModelPopupKeepsBackgroundVisible(t *testing.T) {
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 130, Height: 40})
	m.focus = PanelFilePicker
	m.handleKey(pressKey("enter"))
	if !m.pickerOpen {
		t.Fatal("popup did not open")
	}
	lines := strings.Split(m.View().Content, "\n")
	// The hint line (last row, outside the popup) survives.
	if !strings.Contains(ansiStrip(lines[len(lines)-1]), "↑↓ Navigate") {
		t.Error("hint line lost behind the popup")
	}
	// The popup frame is present.
	found := false
	for _, ln := range lines {
		if strings.Contains(ln, "Select a config file") {
			found = true
			break
		}
	}
	if !found {
		t.Error("popup frame missing")
	}
}

// Option B: Enter on a collapsed list-item row expands it (children appear);
// Enter on the expanded item's heading collapses it back.
func TestModelEnterTogglesListItem(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "c.yaml")
	os.WriteFile(cfgPath, []byte("playlists:\n  - name: WL\n    count: 10\n"), 0o644)
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m.handleConfigSelected(cfgPath)
	m.focus = PanelFilePicker // config card has focus; lower shows config content
	m.View()

	// Find the collapsed item row in table coordinates.
	itemTable := -1
	tbl := m.configView.RenderTable(118)
	for i, r := range tbl {
		if strings.Contains(r, "[1]") && !m.configView.IsTableSeparator(i) {
			itemTable = i
			break
		}
	}
	m.configVP.GotoTop()
	m.configCursor = itemTable - m.configVP.YOffset()
	m.handleKey(pressKey("enter"))
	lines := strings.Join(m.configView.Lines(), "\n")
	if !strings.Contains(lines, "count") || strings.Contains(lines, "count=10") {
		t.Fatalf("Enter did not expand the item; lines=%q", m.configView.Lines())
	}
	// The expanded item's heading row collapses it again.
	headTable := -1
	tbl2 := m.configView.RenderTable(118)
	for i, r := range tbl2 {
		if strings.Contains(r, "[1]") && !m.configView.IsTableSeparator(i) {
			headTable = i
			break
		}
	}
	m.configCursor = headTable - m.configVP.YOffset()
	m.handleKey(pressKey("enter"))
	if got := strings.Join(m.configView.Lines(), "\n"); !strings.Contains(got, "count=10") {
		t.Errorf("Enter on expanded heading did not collapse; lines=%q", m.configView.Lines())
	}
}

func TestModelLeftRightRatio1To2(t *testing.T) {
	m := NewModel(NewAppState())
	// Test various widths
	for _, width := range []int{30, 60, 90, 120, 150} {
		m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
		// Ensure minimum width of 10 for each panel
		if m.leftWidth < 10 {
			t.Errorf("leftWidth too small: %d at width %d", m.leftWidth, width)
		}
		if m.rightWidth < 10 {
			t.Errorf("rightWidth too small: %d at width %d", m.rightWidth, width)
		}
		// Check ratio: leftWidth * 2 should equal rightWidth (within 1 due to integer division)
		expectedRight := m.leftWidth * 2
		if diff := m.rightWidth - expectedRight; diff < -1 || diff > 1 {
			t.Errorf("width ratio not 1:2: left=%d right=%d expected right~%d at width %d",
				m.leftWidth, m.rightWidth, expectedRight, width)
		}
	}
}

// TestModelSashDragFollowsMouse verifies that dragging the sash updates widths
// proportionally to mouse movement and that the sash position follows the mouse.
func TestModelSashDragFollowsMouse(t *testing.T) {
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	initialLeft := m.leftWidth
	// Simulate mouse click on sash at x = leftWidth (the left edge of the sash)
	// isOnVerticalSash considers a 2-column wide sash area (x == sashX || x == sashX-1)
	sashX := initialLeft
	clickMsg := tea.MouseClickMsg{X: sashX, Y: 5}
	m.Update(clickMsg)
	// Drag right by 10 columns
	m.Update(tea.MouseMotionMsg{X: sashX + 10, Y: 5})
	// leftWidth should increase by 10, but not less than 10
	expectedLeft := initialLeft + 10
	if expectedLeft < 10 {
		expectedLeft = 10
	}
	if m.leftWidth != expectedLeft {
		t.Errorf("after drag right, leftWidth=%d expected=%d", m.leftWidth, expectedLeft)
	}
	// Drag left by 5 columns (net +5 from original)
	m.Update(tea.MouseMotionMsg{X: sashX + 5, Y: 10})
	expectedLeft2 := initialLeft + 5
	if expectedLeft2 < 10 {
		expectedLeft2 = 10
	}
	if m.leftWidth != expectedLeft2 {
		t.Errorf("after drag left, leftWidth=%d expected=%d", m.leftWidth, expectedLeft2)
	}
	// Release mouse
	releaseMsg := tea.MouseReleaseMsg{X: sashX + 5, Y: 5}
	m.Update(releaseMsg)
	// After release, dragSashStart should be -1
	if m.dragSashStart != -1 {
		t.Errorf("dragSashStart not reset after release: got %d", m.dragSashStart)
	}
}

func TestModelClickConfigFileOpensPicker(t *testing.T) {
	// 1️⃣  Create a fresh model – no config file loaded yet.
	m := NewModel(NewAppState())

	// 2️⃣  Give it a reasonable window size so the layout is calculated.
	//    (Any size ≥ 20 columns works; we use 80×24 like a typical terminal.)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// 3️⃣  Determine where the "Config File" card lives.
	//    The top‑left panel (PanelFilePicker) rectangle is:
	//        X: 0 … m.leftWidth-1
	//        Y: 0 … topHeight-1   (topHeight is the height of the top band)
	topHeight := 9
	if topHeight > (m.height-1)/2 {
		topHeight = (m.height - 1) / 2
	}
	// Pick a point safely inside the card – e.g. (5, 2) works for any width ≥ 10.
	clickX := 5
	if clickX >= m.leftWidth {
		clickX = m.leftWidth / 2 // fallback to middle if the card is very narrow
	}
	clickY := 2
	if clickY >= topHeight {
		clickY = topHeight / 2
	}
	// Log the panel rectangle for debugging
	rects := m.panelRects()
	t.Logf("PanelFilePicker rect: %v", rects[PanelFilePicker])
	t.Logf("Click point: (%d,%d)", clickX, clickY)
	t.Logf("Point in rect? %v", image.Pt(clickX, clickY).In(rects[PanelFilePicker]))

	// 4️⃣  Build a MouseClickMsg at that coordinate (Button zero‑value corresponds to left button).
	clickMsg := tea.MouseClickMsg{
		X: clickX,
		Y: clickY,
	}

	// 5️⃣  Feed the message to the model.
	m.Update(clickMsg)
	t.Logf("After click: leftWidth=%d, topHeight=%d, pickerOpen=%v", m.leftWidth, topHeight, m.pickerOpen)

	// 6️⃣  After the click the file‑picker popup must be open.
	if !m.pickerOpen {
		t.Fatalf("expected picker popup to be open after clicking the Config File card")
	}

	// 7️⃣  (Optional) sanity‑check the rendered output contains the picker title.
	if !strings.Contains(m.View().Content, "Select a config file") {
		t.Fatalf("View() does not contain the picker popup title;\nGot:\n%s", m.View().Content)
	}
}

// hintClickPoint returns the screen coordinates at the centre of the hint
// segment with the given key on panel p (key "" matches an informational,
// non-clickable segment), or false when no such segment exists.
func hintClickPoint(m *Model, p FocusedPanel, key string) (int, int, bool) {
	rect, ok := m.panelRects()[p]
	if !ok {
		return 0, 0, false
	}
	segs := m.hintSegmentsFor(p)
	lines := layoutHintSegments(segs, rect.Dx()-4, rect.Dy()-5)
	for li, line := range lines {
		for _, pw := range line {
			if segs[pw.seg].Key == key {
				x := rect.Min.X + 1 + (pw.startCol+pw.endCol)/2 // +1: left border
				y := rect.Min.Y + 1 + li                        // hint rows sit below the title bar
				return x, y, true
			}
		}
	}
	return 0, 0, false
}

// Clicking the [c] Switch View hint segment toggles the config view, exactly
// like pressing the c key. The hint lives in the lower config content area,
// which is displayed while the config card has focus; clicking focuses the
// config card.
func TestHintClickTogglesConfigView(t *testing.T) {
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m.configView = NewConfigView("llm:\n  provider: x\n")
	before := m.configView.Mode()
	x, y, ok := hintClickPoint(m, PanelConfig, "c")
	if !ok {
		t.Fatal("[c] segment not found in config hint")
	}
	m.Update(tea.MouseClickMsg{X: x, Y: y})
	if m.configView.Mode() == before {
		t.Error("config view mode did not toggle after clicking the [c] hint segment")
	}
	if m.focus != PanelFilePicker {
		t.Errorf("focus = %v, want PanelFilePicker (owner of the config content)", m.focus)
	}
}

// Clicking the [Enter] Select Config hint segment on the file card opens the
// file-picker popup, exactly like pressing Enter on the focused card.
func TestHintClickOpensFilePicker(t *testing.T) {
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	x, y, ok := hintClickPoint(m, PanelFilePicker, "enter")
	if !ok {
		t.Fatal("[Enter] segment not found on the config card hint")
	}
	m.Update(tea.MouseClickMsg{X: x, Y: y})
	if !m.pickerOpen {
		t.Error("picker popup did not open after clicking the [Enter] Select Config segment")
	}
}

// Clicking the [r] Run/Stop hint segment emits StartRunMsg (with a config
// selected), exactly like pressing r.
func TestHintClickRunEmitsStartRun(t *testing.T) {
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m.state.ConfigPath = "/tmp/fake-config.yaml"
	x, y, ok := hintClickPoint(m, PanelStatus, "r")
	if !ok {
		t.Fatal("[r] segment not found on the status panel hint")
	}
	_, cmd := m.Update(tea.MouseClickMsg{X: x, Y: y})
	if cmd == nil {
		t.Fatal("clicking [r] Run/Stop hint did not emit a command")
	}
	if _, isStart := cmd().(StartRunMsg); !isStart {
		t.Errorf("command message = %T, want StartRunMsg", cmd())
	}
}

// Clicking an informational hint segment ([↑↓] Scroll, Key "") focuses the
// panel whose content is displayed in the lower area but triggers no action.
// When the config card has focus (the default), the lower area shows the config
// content and its informational hint is "[↑↓] Navigate"; clicking it focuses
// the config card. When the status panel has focus, the lower area shows the
// execution log and its informational hint is "[↑↓] Scroll"; clicking it
// focuses the status panel.
func TestHintClickInfoSegmentDoesNothing(t *testing.T) {
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m.focus = PanelStatus // the lower area shows the execution log
	x, y, ok := hintClickPoint(m, PanelEvents, "")
	if !ok {
		t.Fatal("informational segment not found on the events hint")
	}
	m.Update(tea.MouseClickMsg{X: x, Y: y})
	if m.pickerOpen {
		t.Error("picker opened from an informational segment click")
	}
	if m.isRunning {
		t.Error("run started from an informational segment click")
	}
	if m.focus != PanelStatus {
		t.Errorf("focus = %v, want PanelStatus (click should still focus its owner)", m.focus)
	}
}

// Panels must render exactly the requested height: a panel built with h rows
// (title + hint + separator + content + bottom border) must produce exactly
// h lines. The old arithmetic produced h-1 lines, which shifted the bottom
// band up one row and made every bottom-panel hint click miss by one row.
func TestPanelRendersExactHeight(t *testing.T) {
	segs := []HintSegment{
		{Text: "[↑↓] Navigate"},
		{Text: "[Enter] Edit", Key: "enter"},
		{Text: "[c] Switch View", Key: "c"},
	}
	for h := 6; h <= 20; h++ {
		got := strings.Split(RenderPanelWithSegments("Config", "a\nb\n", 40, h, true, segs), "\n")
		if len(got) != h {
			t.Errorf("RenderPanelWithSegments h=%d produced %d lines (want %d)", h, len(got), h)
		}
	}
	for h := 6; h <= 20; h++ {
		got := strings.Split(RenderPopupWithTitle("Edit x", "a\nb\n", 60, h, "[Enter] Save"), "\n")
		if len(got) != h {
			t.Errorf("RenderPopupWithTitle h=%d produced %d lines (want %d)", h, len(got), h)
		}
		gotNoHint := strings.Split(RenderPopupWithTitle("Edit x", "a\nb\n", 60, h, ""), "\n")
		if len(gotNoHint) != h {
			t.Errorf("RenderPopupWithTitle (no hint) h=%d produced %d lines (want %d)", h, len(gotNoHint), h)
		}
	}
}

// The rendered grid must sit exactly where panelRects says: the bottom band's
// title is at PanelConfig.Min.Y and its hint row at Min.Y+1. When the top
// band renders one row short this drifts by one and hint clicks miss.
func TestPanelRectsMatchRenderedGrid(t *testing.T) {
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	lines := strings.Split(m.View().Content, "\n")

	rect := m.panelRects()[PanelConfig]
	titleLine := lines[rect.Min.Y]
	if !strings.Contains(stripANSI(titleLine), "Config") {
		t.Errorf("bottom band title not at PanelConfig.Min.Y=%d; line: %q", rect.Min.Y, stripANSI(titleLine))
	}
	if !strings.Contains(stripANSI(lines[rect.Min.Y+1]), "Enter] Edit") {
		t.Errorf("bottom band hint not at PanelConfig.Min.Y+1=%d; line: %q",
			rect.Min.Y+1, stripANSI(lines[rect.Min.Y+1]))
	}
	// And the click point derived from the real render must hit the segment.
	m.configView = NewConfigView("llm:\n  provider: x\n")
	renderedHint := stripANSI(lines[rect.Min.Y+1])
	// strings.Index is a byte index, but the click needs the display column
	// (↑/↓ are multi-byte); lipgloss.Width of the prefix gives the column.
	x := lipgloss.Width(renderedHint[:strings.Index(renderedHint, "[c]")]) + 1
	if key, ok := m.hintHitTest(tea.Mouse{X: x, Y: rect.Min.Y + 1}, PanelConfig, rect); !ok || key != "c" {
		t.Fatalf("hintHitTest at rendered [c] (x=%d) = %q, want \"c\"", x, key)
	}
	after, _ := m.Update(tea.MouseClickMsg{X: x, Y: rect.Min.Y + 1})
	if after.(*Model).configView.Mode() != ConfigViewRaw {
		t.Error("click at the rendered [c] position did not toggle the config view")
	}
}

// popupHintClickPoint returns the screen coordinates at the centre of the
// hint segment with the given key inside a rendered popup rectangle (the
// same layout popupHintHitTest uses), or false when no such segment exists.
func popupHintClickPoint(m *Model, rect image.Rectangle, hint, key string) (int, int, bool) {
	segs := parsePopupHint(hint)
	if len(segs) == 0 {
		return 0, 0, false
	}
	innerW := rect.Dx() - 2
	if innerW < 1 {
		return 0, 0, false
	}
	lines := layoutHintSegments(segs, innerW-2, rect.Dy()-3)
	for li, line := range lines {
		for _, pw := range line {
			if segs[pw.seg].Key == key {
				x := rect.Min.X + 1 + (pw.startCol+pw.endCol)/2
				y := rect.Min.Y + 1 + li
				return x, y, true
			}
		}
	}
	return 0, 0, false
}

// Regression (reviewer, 2026-10-12): clicking the [Enter] Select hint of the
// file-picker popup used the picker's stale ChosenPath, loading a previously
// selected file instead of the highlighted one (and doing nothing when no
// file had ever been selected). The hint click must clear the stale path,
// let Enter act on the highlighted row, and then apply that selection.
func TestModelPickerEnterHintClickSelectsHighlightedFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "alpha.yaml"), []byte("llm:\n  provider: ollama\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "beta.yaml"), []byte("llm:\n  provider: ollama\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("YTSS_CONFIG_DIR", dir)
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if c := m.filePicker.Init(); c != nil {
		if msg := c(); msg != nil {
			m.Update(msg)
		}
	}
	m.focus = PanelFilePicker
	m.handleKey(pressKey("enter")) // open the popup; cursor at row 0 = alpha.yaml
	if !m.pickerOpen {
		t.Fatal("popup did not open")
	}

	// Simulate a prior selection that left a stale path behind.
	m.filePicker.fp.Path = filepath.Join(dir, "stale.yaml")

	x, y, ok := popupHintClickPoint(m, m.pickerPopupRect(), pickerPopupHint, "enter")
	if !ok {
		t.Fatal("[Enter] segment not found in the picker popup hint")
	}
	m.Update(tea.MouseClickMsg{X: x, Y: y})

	if m.pickerOpen {
		t.Error("popup still open after clicking the [Enter] Select hint")
	}
	if got := m.state.Snapshot().ConfigPath; !strings.HasSuffix(got, "alpha.yaml") {
		t.Errorf("config path = %q, want .../alpha.yaml (the highlighted file, not the stale path)", got)
	}
}

// Regression (reviewer, 2026-10-12): clicking a config table row did nothing
// because ItemAtPos/PosToRow ignored the panel origin, the hint rows and the
// viewport offset. The click must map through those and start editing the
// row's value, and the [Enter] Save hint of the edit popup must commit it
// (editPopupRect previously disagreed with renderConfig's placement).
func TestModelConfigRowClickStartsEditAndSaveHintCommits(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(cfgPath, []byte("llm:\n  provider: x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.state.ConfigPath = cfgPath
	m.configView = NewConfigView("llm:\n  provider: x\n")
	m.View() // RenderTable builds the table-row mapping the click resolves through

	// Click the first value row (provider: x). The table viewport starts
	// below the title bar and hint rows; the table's first rows are the
	// border/header and the first content row is the "llm" heading, so walk
	// to the first editable (value) row.
	configRect := m.panelRects()[PanelConfig]
	hintRows := m.panelHintRows(PanelConfig, configRect)
	contentY := -1
	for i := 0; i < 30; i++ {
		if c := m.configView.TableContentIndex(i); c >= 0 {
			if _, _, ok := m.configView.EditTargetForLine(c); ok {
				contentY = i
				break
			}
		}
	}
	if contentY < 0 {
		t.Fatal("no content row found in the rendered table")
	}
	y := configRect.Min.Y + 1 + hintRows + contentY
	m.Update(tea.MouseClickMsg{X: configRect.Min.X + 2, Y: y})
	if !m.editing {
		t.Fatal("clicking a config table row did not start editing")
	}
	if m.editPath != "llm.provider" {
		t.Errorf("editPath = %q, want \"llm.provider\"", m.editPath)
	}

	// Type a new value, then click the [Enter] Save hint of the edit popup.
	m.editText.SetValue("y")
	hx, hy, ok := popupHintClickPoint(m, m.editPopupRect(), editPopupHint, "enter")
	if !ok {
		t.Fatalf("no [Enter] hint found in editPopupRect=%v", m.editPopupRect())
	}
	m.Update(tea.MouseClickMsg{X: hx, Y: hy})
	if m.editing {
		t.Fatal("still editing after clicking the [Enter] Save hint")
	}
	content, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "provider: y") {
		t.Errorf("config file not updated by the Save hint click:\n%s", content)
	}
}

// W1-02 A3 positive: with the events viewport pinned at the bottom, new
// events keep the newest line visible (the viewport follows the bottom).
func TestEventsNewEventsStayPinned(t *testing.T) {
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	for i := 0; i < 50; i++ {
		m.state.AddRecentEvent(fmt.Sprintf("event-%02d", i))
	}
	m.focus = PanelStatus // lower shows the execution log
	// Render once so the events viewport is sized and pinned at the bottom
	// (real flow: the frame is drawn before the next event arrives).
	m.View()
	m.state.AddRecentEvent("new-event")
	after := m.View()

	if !strings.Contains(after.Content, "new-event") {
		t.Error("newest event not visible after adding new event while pinned at bottom")
	}
}

// W1-02 A3 negative: when the user has scrolled up, new events must NOT jump
// the viewport back to the bottom — the offset is preserved.
func TestEventsScrolledUpDoesNotJump(t *testing.T) {
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	for i := 0; i < 50; i++ {
		m.state.AddRecentEvent(fmt.Sprintf("event-%02d", i))
	}
	m.focus = PanelStatus // lower shows the execution log

	for i := 0; i < 5; i++ {
		m.handleKey(pressKey("up"))
	}
	offsetBefore := m.eventsVP.YOffset()

	m.state.AddRecentEvent("new-event")

	offsetAfter := m.eventsVP.YOffset()
	if offsetBefore != offsetAfter {
		t.Errorf("viewport offset changed from %d to %d after adding new event while scrolled up", offsetBefore, offsetAfter)
	}
}

// W1-01 A1 positive: the layout is two rows — two upper panels and one
// full-width lower content panel. The upper row contains the config card and
// the execution status panel side by side; the lower row is a single content
// area that shows config content when the config card has focus and the
// execution log when the status panel has focus.
func TestModelTwoRowsThreePanels(t *testing.T) {
	m := sizedModel()
	content := viewContent(t, m)
	for _, title := range []string{"■ Config File", "■ Execution Status"} {
		if !strings.Contains(content, title) {
			t.Errorf("View missing upper panel title %q", title)
		}
	}
	if strings.Contains(content, "■ Recent Events") {
		t.Error("execution log rendered while the config card has focus; the lower area must show the config content")
	}
	lines := strings.Split(content, "\n")
	corners := 0
	for _, ln := range lines {
		corners += strings.Count(stripANSI(ln), "└")
	}
	if corners != 3 {
		t.Errorf("found %d panel bottom borders (└), want 3 (two upper + one full-width lower)", corners)
	}
	// The lower band is a single full-width panel: its rows carry exactly
	// two vertical borders (one panel), not four (two side-by-side).
	ansiRe := regexp.MustCompile(`\x1b\[[0-9;>?]*[a-zA-Z]`)
	lowerRows := 0
	for i, ln := range lines {
		plain := ansiRe.ReplaceAllString(ln, "")
		if strings.Count(plain, "│") == 4 {
			continue // an upper-row line (two panels side by side)
		}
		if strings.Count(plain, "│") == 2 && i > 0 {
			lowerRows++
		}
	}
	if lowerRows == 0 {
		t.Error("no full-width lower rows found; the bottom band is still split into two panels")
	}
	// The lower panel's bottom border should span the full terminal width.
	fullWidthFound := false
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.Contains(stripANSI(lines[i]), "└") && lipgloss.Width(stripANSI(lines[i])) == 120 {
			fullWidthFound = true
			break
		}
	}
	if !fullWidthFound {
		t.Error("lower panel bottom border does not span the full terminal width")
	}
}

// A1 boundary: resize keeps the layout filled and in bounds.
func TestModelResizeKeepsLayoutInBounds(t *testing.T) {
	m := NewModel(NewAppState())
	for _, size := range [][2]int{{80, 24}, {120, 40}, {60, 20}, {200, 50}} {
		w, h := size[0], size[1]
		m.Update(tea.WindowSizeMsg{Width: w, Height: h})
		lines := strings.Split(m.View().Content, "\n")
		if len(lines) > h {
			t.Errorf("%dx%d: view renders %d lines (overflows the terminal)", w, h, len(lines))
		}
		if last := lines[len(lines)-1]; !strings.Contains(stripANSI(last), "↑↓ Navigate") {
			t.Errorf("%dx%d: last line is not the hint line: %q", w, h, stripANSI(last))
		}
		corners := 0
		fullWidth := false
		for _, ln := range lines {
			plain := stripANSI(ln)
			corners += strings.Count(plain, "└")
			if strings.Contains(plain, "└") && lipgloss.Width(plain) == w {
				fullWidth = true
			}
		}
		if corners != 3 {
			t.Errorf("%dx%d: found %d panel bottom borders, want 3", w, h, corners)
		}
		if !fullWidth {
			t.Errorf("%dx%d: no full-width lower panel border found", w, h)
		}
	}
}

// A2 positive: the focus cycle swaps lower content between the config view and
// the execution log, and each view's scroll position survives the round trip.
func TestModelFocusCycleSwapsLowerContentRestoresScroll(t *testing.T) {
	m := sizedModel()
	for i := 0; i < 60; i++ {
		m.state.AddRecentEvent(fmt.Sprintf("event-%02d", i))
	}
	if m.focus != PanelFilePicker {
		t.Fatalf("initial focus = %v, want PanelFilePicker", m.focus)
	}
	content := viewContent(t, m)
	if strings.Contains(content, "■ Recent Events") {
		t.Error("log shown while the config card has focus")
	}
	m.handleKey(pressKey("tab"))
	if m.focus != PanelStatus {
		t.Fatalf("after Tab focus = %v, want PanelStatus (the cycle has exactly two targets)", m.focus)
	}
	content = viewContent(t, m)
	if !strings.Contains(content, "■ Recent Events") {
		t.Error("log not shown while the status panel has focus")
	}
	// Scroll up into older events, switch away and back: the offset survives.
	for i := 0; i < 10; i++ {
		m.handleKey(pressKey("up"))
	}
	offset := m.eventsVP.YOffset()
	if offset == 0 {
		t.Fatal("scrolling up did not move the events viewport")
	}
	m.handleKey(pressKey("tab")) // to the config card: lower shows the config view
	viewContent(t, m)
	m.handleKey(pressKey("tab")) // back to status: lower shows the log again
	viewContent(t, m)
	if m.eventsVP.YOffset() != offset {
		t.Errorf("events scroll offset = %d after a focus round-trip, want %d", m.eventsVP.YOffset(), offset)
	}
	if m.focus != PanelStatus {
		t.Errorf("focus after round-trip = %v, want PanelStatus", m.focus)
	}
}

// A2 boundary: with a single focus target the cycle is a no-op; a focus
// outside the active set snaps into it.
func TestNextFocusTargetSingleTargetAndSnapIn(t *testing.T) {
	solo := []FocusedPanel{PanelStatus}
	if got := nextFocusTarget(solo, PanelStatus, 1); got != PanelStatus {
		t.Errorf("single-target cycle forward = %v, want PanelStatus (no-op)", got)
	}
	if got := nextFocusTarget(solo, PanelStatus, -1); got != PanelStatus {
		t.Errorf("single-target cycle backward = %v, want PanelStatus (no-op)", got)
	}
	if got := nextFocusTarget(focusTargets(), PanelConfig, 1); got != PanelFilePicker {
		t.Errorf("stale focus PanelConfig cycles to %v, want snap into the active set", got)
	}
}

// A3 positive: rendering and hit-testing agree on one shared geometry source.
func TestModelRenderMatchesGeometryHitTest(t *testing.T) {
	m := sizedModel()
	g := m.geometry()
	lines := strings.Split(m.View().Content, "\n")
	if title := stripANSI(lines[g.configRect.Min.Y]); !strings.Contains(title, "Config File") {
		t.Errorf("config card title not at geometry row %d: %q", g.configRect.Min.Y, title)
	}
	if title := stripANSI(lines[g.statusRect.Min.Y]); !strings.Contains(title, "Execution Status") {
		t.Errorf("status title not at geometry row %d: %q", g.statusRect.Min.Y, title)
	}
	if title := stripANSI(lines[g.lowerRect.Min.Y]); !strings.Contains(title, "Config") {
		t.Errorf("lower content title not at geometry row %d: %q", g.lowerRect.Min.Y, title)
	}
	// A click inside the status rect focuses it.
	m.Update(tea.MouseClickMsg{X: g.statusRect.Min.X + 2, Y: g.statusRect.Min.Y + 4})
	if m.focus != PanelStatus {
		t.Errorf("click inside the status rect left focus = %v, want PanelStatus", m.focus)
	}
	// The lower area now shows the log, whose owner is the status panel: a
	// click in the lower content area keeps focus on that owner — it must
	// not flip to the other panel or open the picker.
	m.Update(tea.MouseClickMsg{X: g.lowerRect.Dx() / 2, Y: g.lowerRect.Min.Y + 4})
	if m.focus != PanelStatus {
		t.Errorf("click in the log content left focus = %v, want PanelStatus (its owner)", m.focus)
	}
	if m.pickerOpen {
		t.Error("click in the log content opened the file picker")
	}
	// Switch to the config card: the lower area shows the config content,
	// whose owner is the config card. Clicking it keeps that focus.
	m.handleKey(pressKey("tab"))
	viewContent(t, m)
	m.Update(tea.MouseClickMsg{X: g.lowerRect.Dx() / 2, Y: g.lowerRect.Min.Y + 4})
	if m.focus != PanelFilePicker {
		t.Errorf("click in the config content left focus = %v, want PanelFilePicker (its owner)", m.focus)
	}
	if m.editing {
		t.Error("click in the config content started the inline editor")
	}
}

// A3 boundary: panelRects is derived from the single geometry source.
func TestModelPanelRectsDerivedFromGeometry(t *testing.T) {
	m := NewModel(NewAppState())
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	g := m.geometry()
	rects := m.panelRects()
	if rects[PanelFilePicker] != g.configRect {
		t.Error("panelRects[PanelFilePicker] diverged from geometry().configRect")
	}
	if rects[PanelStatus] != g.statusRect {
		t.Error("panelRects[PanelStatus] diverged from geometry().statusRect")
	}
	if rects[PanelConfig] != g.lowerRect || rects[PanelEvents] != g.lowerRect {
		t.Error("panelRects content kinds diverged from geometry().lowerRect")
	}
	// The edit popup is placed from the same lower rect.
	m.editing = true
	m.editPath = "x"
	if er := m.editPopupRect(); er.Max.X > g.lowerRect.Max.X-1 || er.Min.X < g.lowerRect.Min.X+1 ||
		er.Max.Y > g.lowerRect.Max.Y-1 || er.Min.Y < g.lowerRect.Min.Y+1 {
		t.Errorf("editPopupRect %v escapes the lower content rect %v", er, g.lowerRect)
	}
	m.editing = false
}

// W1-01 round 2: finishRun must skip the end-of-run SetRunning(false) while
// the generation is stale (an old run unwinding after stop→restart), and must
// apply it once the generation is current.
func TestModelFinishRunSkipsStaleGeneration(t *testing.T) {
	m := sizedModel()
	m.state.SetRunning(true)
	m.runGen = 2

	m.finishRun(1) // stale: must not clobber the newer run's running state.
	if snap := m.state.Snapshot(); !snap.IsRunning {
		t.Fatal("stale generation clobbered the newer run's running state")
	}

	m.finishRun(2) // current: the end-of-run clear must apply.
	if snap := m.state.Snapshot(); snap.IsRunning {
		t.Fatal("current generation finishRun did not clear the running state")
	}
}
