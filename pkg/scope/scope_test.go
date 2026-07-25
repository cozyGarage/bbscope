package scope

import (
	"testing"
)

func TestNormalizeCategory(t *testing.T) {
	tests := []struct {
		name     string
		category string
		want     string
	}{
		// Direct mappings
		{"wildcard", "wildcard", "wildcard"},
		{"url", "url", "url"},
		{"website to url", "website", "url"},
		{"web to url", "web", "url"},
		{"api to url", "api", "url"},
		{"ip_address to url", "ip_address", "url"},

		// CIDR
		{"cidr", "cidr", "cidr"},
		{"iprange to cidr", "iprange", "cidr"},

		// Mobile
		{"android", "android", "android"},
		{"google_play_app_id to android", "google_play_app_id", "android"},
		{"ios", "ios", "ios"},
		{"apple to ios", "apple", "ios"},
		{"apple_store_app_id to ios", "apple_store_app_id", "ios"},

		// Case insensitive
		{"uppercase WILDCARD", "WILDCARD", "wildcard"},
		{"mixed case Url", "Url", "url"},

		// Unknown categories
		{"unknown category", "unknown_thing", "unknown thing"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeCategory(tt.category)
			if got != tt.want {
				t.Errorf("NormalizeCategory(%q) = %q, want %q", tt.category, got, tt.want)
			}
		})
	}
}

func TestUnifiedCategories(t *testing.T) {
	categories := UnifiedCategories()

	if len(categories) == 0 {
		t.Error("UnifiedCategories() returned empty list")
	}

	// Check that some expected categories exist
	expectedCategories := []string{"wildcard", "url", "cidr", "android", "ios"}
	for _, expected := range expectedCategories {
		found := false
		for _, cat := range categories {
			if cat == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("UnifiedCategories() missing expected category %q", expected)
		}
	}

	// Check that list is sorted
	for i := 1; i < len(categories); i++ {
		if categories[i-1] > categories[i] {
			t.Errorf("UnifiedCategories() not sorted: %q > %q", categories[i-1], categories[i])
		}
	}
}

func TestIsUnifiedCategory(t *testing.T) {
	tests := []struct {
		name     string
		category string
		want     bool
	}{
		{"wildcard", "wildcard", true},
		{"url", "url", true},
		{"cidr", "cidr", true},
		{"android", "android", true},
		{"ios", "ios", true},
		{"uppercase", "WILDCARD", true},
		{"with spaces", " url ", true},
		{"website (not unified)", "website", false},
		{"unknown", "unknown", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsUnifiedCategory(tt.category)
			if got != tt.want {
				t.Errorf("IsUnifiedCategory(%q) = %v, want %v", tt.category, got, tt.want)
			}
		})
	}
}

func TestGetAllStringsForCategories(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantNil  bool
		wantLen  int
		contains []string
	}{
		{
			name:    "all returns nil",
			input:   "all",
			wantNil: true,
		},
		{
			name:    "empty returns nil",
			input:   "",
			wantNil: true,
		},
		{
			name:     "url category",
			input:    "url",
			wantNil:  false,
			contains: []string{"url", "website", "web", "api"},
		},
		{
			name:     "wildcard category",
			input:    "wildcard",
			wantNil:  false,
			wantLen:  1,
			contains: []string{"wildcard"},
		},
		{
			name:     "multiple categories",
			input:    "url,wildcard",
			wantNil:  false,
			contains: []string{"url", "website", "wildcard"},
		},
		{
			name:    "invalid category only",
			input:   "invalid",
			wantNil: true, // Falls back to all
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetAllStringsForCategories(tt.input)

			if tt.wantNil {
				if got != nil {
					t.Errorf("GetAllStringsForCategories(%q) = %v, want nil", tt.input, got)
				}
				return
			}

			if got == nil {
				t.Errorf("GetAllStringsForCategories(%q) = nil, want non-nil", tt.input)
				return
			}

			if tt.wantLen > 0 && len(got) != tt.wantLen {
				t.Errorf("GetAllStringsForCategories(%q) len = %d, want %d", tt.input, len(got), tt.wantLen)
			}

			for _, expected := range tt.contains {
				found := false
				for _, v := range got {
					if v == expected {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("GetAllStringsForCategories(%q) missing %q", tt.input, expected)
				}
			}
		})
	}
}

func TestCreateLine(t *testing.T) {
	element := ScopeElement{
		Target:      "*.example.com",
		Description: "Main wildcard",
		Category:    "wildcard",
	}
	url := "https://hackerone.com/example"

	tests := []struct {
		name      string
		flags     string
		delimiter string
		want      string
	}{
		{"target only", "t", ",", "*.example.com"},
		{"target and category", "tc", ",", "*.example.com,wildcard"},
		{"all fields", "tdcu", " | ", "*.example.com | Main wildcard | wildcard | https://hackerone.com/example"},
		{"url only", "u", ",", "https://hackerone.com/example"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := createLine(element, url, tt.flags, tt.delimiter)
			if got != tt.want {
				t.Errorf("createLine() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProgramData(t *testing.T) {
	// Test that ProgramData struct works correctly
	pd := ProgramData{
		Url: "https://hackerone.com/test",
		InScope: []ScopeElement{
			{Target: "*.test.com", Category: "wildcard"},
			{Target: "api.test.com", Category: "url"},
		},
		OutOfScope: []ScopeElement{
			{Target: "admin.test.com", Category: "url"},
		},
	}

	if len(pd.InScope) != 2 {
		t.Errorf("InScope length = %d, want 2", len(pd.InScope))
	}
	if len(pd.OutOfScope) != 1 {
		t.Errorf("OutOfScope length = %d, want 1", len(pd.OutOfScope))
	}
}
