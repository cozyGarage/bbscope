package immunefi

import (
	"testing"

	"github.com/tidwall/gjson"
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
			expected: []string{"websites_and_applications", "smart_contract", "blockchain_dlt"},
		},
		{
			name:     "blockchain category",
			input:    "blockchain",
			expected: []string{"blockchain_dlt"},
		},
		{
			name:     "invalid category defaults to all",
			input:    "invalid",
			expected: []string{"websites_and_applications", "smart_contract", "blockchain_dlt"},
		},
		{
			name:     "empty string defaults to all",
			input:    "",
			expected: []string{"websites_and_applications", "smart_contract", "blockchain_dlt"},
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

func TestImmunefiProgramSlug(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{`{"slug":"acme","id":"ignored"}`, "acme"},
		{`{"url":"/bug-bounty/beta/information/"}`, "beta"},
		{`{"url":"https://immunefi.com/bug-bounty/gamma/information/"}`, "gamma"},
		{`{"id":"delta"}`, "delta"},
		{`{"inviteOnly":false}`, ""},
	}
	for _, tc := range tests {
		if got := immunefiProgramSlug(gjson.Parse(tc.raw)); got != tc.want {
			t.Errorf("immunefiProgramSlug(%s) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}
