package immunefi

import (
	"testing"
)

func TestPlatformURL(t *testing.T) {
	expected := "https://immunefi.com"
	if PLATFORM_URL != expected {
		t.Errorf("PLATFORM_URL = %v, want %v", PLATFORM_URL, expected)
	}
}

func TestGetCategories(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "web category",
			input:    "web",
			expected: []string{"websites_and_applications"},
		},
		{
			name:     "Web category uppercase",
			input:    "WEB",
			expected: []string{"websites_and_applications"},
		},
		{
			name:     "contracts category",
			input:    "contracts",
			expected: []string{"smart_contract"},
		},
		{
			name:     "CONTRACTS category uppercase",
			input:    "CONTRACTS",
			expected: []string{"smart_contract"},
		},
		{
			name:     "all category",
			input:    "all",
			expected: []string{"websites_and_applications", "smart_contract"},
		},
		{
			name:     "invalid category defaults to all",
			input:    "invalid",
			expected: []string{"websites_and_applications", "smart_contract"},
		},
		{
			name:     "empty string defaults to all",
			input:    "",
			expected: []string{"websites_and_applications", "smart_contract"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getCategories(tt.input)
			
			if len(got) != len(tt.expected) {
				t.Errorf("getCategories(%q) returned %d items, want %d", tt.input, len(got), len(tt.expected))
				return
			}
			
			for i, v := range got {
				if v != tt.expected[i] {
					t.Errorf("getCategories(%q)[%d] = %v, want %v", tt.input, i, v, tt.expected[i])
				}
			}
		})
	}
}

func TestGetCategoriesCaseInsensitive(t *testing.T) {
	// Test that the function is case-insensitive
	lowercaseResult := getCategories("web")
	uppercaseResult := getCategories("WEB")
	mixedCaseResult := getCategories("WeB")
	
	if len(lowercaseResult) != len(uppercaseResult) {
		t.Error("getCategories should be case-insensitive")
	}
	
	if len(lowercaseResult) != len(mixedCaseResult) {
		t.Error("getCategories should be case-insensitive for mixed case")
	}
}
