package tui

import (
	"errors"
	"strings"
	"testing"
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
