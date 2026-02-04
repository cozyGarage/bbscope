package tui

import (
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
	}

	for _, tc := range tests {
		result := truncate(tc.input, tc.maxLen)
		if result != tc.expected {
			t.Errorf("truncate(%q, %d) = %q, want %q",
				tc.input, tc.maxLen, result, tc.expected)
		}
	}
}

func TestNewStyles(t *testing.T) {
	styles := newStyles()
	
	// Just verify that styles are created without panicking
	if styles.Title.GetBold() != true {
		t.Error("Expected Title style to be bold")
	}
}
