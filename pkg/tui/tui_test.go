package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNewModel(t *testing.T) {
	// Test that NewModel creates a valid model
	model := NewModel(nil)

	if model.view != ViewDashboard {
		t.Errorf("Expected initial view to be Dashboard, got %v", model.view)
	}

	if model.quitting {
		t.Error("Expected quitting to be false initially")
	}

	if model.cancel == nil {
		t.Error("Expected cancel function to be initialized")
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"this is a very long string", 10, "this is..."},
		{"", 5, ""},
		{"abcdef", 0, ""},
		{"abcdef", 1, "a"},
		{"abcdef", 2, "ab"},
		{"abcdef", 3, "abc"},
	}

	for _, tc := range tests {
		result := truncate(tc.input, tc.maxLen)
		if result != tc.expected {
			t.Errorf("truncate(%q, %d) = %q, want %q",
				tc.input, tc.maxLen, result, tc.expected)
		}
	}
}

func TestRenderRecentChangesWaitsForLoad(t *testing.T) {
	m := NewModel(nil)
	got := m.renderRecentChanges()
	if !strings.Contains(got, "Loading recent changes") {
		t.Fatalf("unloaded dashboard should not claim there are no changes, got %q", got)
	}
	if strings.Contains(got, "No recent changes") {
		t.Fatalf("unloaded dashboard flashed the empty state: %q", got)
	}

	m.changesLoaded = true
	got = m.renderRecentChanges()
	if !strings.Contains(got, "No recent changes") {
		t.Fatalf("loaded empty dashboard should say no recent changes, got %q", got)
	}
}

func TestUpdateClearsErrorOnSuccess(t *testing.T) {
	m := NewModel(nil)
	m.err = errors.New("stale")
	next, _ := m.Update(changesMsg(nil))
	got := next.(Model)
	if !got.changesLoaded {
		t.Fatal("changesMsg should mark changes loaded")
	}
	if got.err != nil {
		t.Fatalf("successful load should clear m.err, got %v", got.err)
	}
}

func TestNewStyles(t *testing.T) {
	styles := newStyles()

	// Just verify that styles are created without panicking
	if styles.Title.GetBold() != true {
		t.Error("Expected Title style to be bold")
	}
}

func keyMsg(s string) tea.KeyMsg {
	if s == "ctrl+c" {
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestHandleKeyPress(t *testing.T) {
	m := NewModel(nil)

	next, cmd := m.Update(keyMsg("s"))
	got := next.(Model)
	if got.view != ViewSearch {
		t.Fatalf("s: view = %v, want search", got.view)
	}
	if cmd != nil {
		t.Fatal("s should not start a command")
	}

	next, _ = got.Update(keyMsg("d"))
	got = next.(Model)
	if got.view != ViewDashboard {
		t.Fatalf("d: view = %v, want dashboard", got.view)
	}

	next, _ = got.Update(keyMsg("?"))
	got = next.(Model)
	if got.view != ViewHelp {
		t.Fatalf("?: view = %v, want help", got.view)
	}

	next, cmd = got.Update(keyMsg("p"))
	got = next.(Model)
	if got.view != ViewPolling || !got.pollingActive {
		t.Fatalf("p: view=%v pollingActive=%v", got.view, got.pollingActive)
	}
	if cmd == nil {
		t.Fatal("first p should return startPollingCmd")
	}
	// Do not invoke cmd: startPollingCmd sleeps for 2s (placeholder polling).

	next, cmd = got.Update(keyMsg("p"))
	got = next.(Model)
	if cmd != nil {
		t.Fatal("second p while polling must not start another poll")
	}

	next, cmd = got.Update(keyMsg("q"))
	got = next.(Model)
	if !got.quitting {
		t.Fatal("q should set quitting")
	}
	if cmd == nil {
		t.Fatal("q should return tea.Quit")
	}
}

func TestUpdateWindowSizeAndError(t *testing.T) {
	m := NewModel(nil)

	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	got := next.(Model)
	if got.width != 120 || got.height != 40 {
		t.Fatalf("size = %dx%d", got.width, got.height)
	}

	next, _ = got.Update(errMsg(errors.New("boom")))
	got = next.(Model)
	if got.err == nil || got.err.Error() != "boom" {
		t.Fatalf("err = %v", got.err)
	}
	view := got.View()
	if !strings.Contains(view, "boom") {
		t.Fatalf("view missing error: %q", view)
	}

	next, _ = got.Update(statsMsg{ProgramCount: 3})
	got = next.(Model)
	if !got.statsLoaded || got.err != nil {
		t.Fatalf("statsMsg should load stats and clear err (loaded=%v err=%v)", got.statsLoaded, got.err)
	}
	if got.stats.ProgramCount != 3 {
		t.Fatalf("ProgramCount = %d", got.stats.ProgramCount)
	}
}

func TestViewQuitting(t *testing.T) {
	m := NewModel(nil)
	m.quitting = true
	if got := m.View(); got != "Goodbye!\n" {
		t.Fatalf("view = %q", got)
	}
}
