package wildcards

import (
	"testing"
)

func TestBlacklistedSuffixes(t *testing.T) {
	if len(BlacklistedSuffixes) == 0 {
		t.Error("BlacklistedSuffixes should not be empty")
	}

	// Check some expected entries exist
	expectedSuffixes := []string{
		"amazonaws.com",
		"herokuapp.com",
		"github.io",
		"vercel.app",
	}

	for _, expected := range expectedSuffixes {
		found := false
		for _, suffix := range BlacklistedSuffixes {
			if suffix == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("BlacklistedSuffixes missing %q", expected)
		}
	}
}

func TestNonDomainCategories(t *testing.T) {
	expectedCategories := []string{"android", "ios", "binary", "code", "ai", "hardware", "blockchain"}

	for _, cat := range expectedCategories {
		if _, ok := NonDomainCategories[cat]; !ok {
			t.Errorf("NonDomainCategories missing %q", cat)
		}
	}

	// url should NOT be in NonDomainCategories
	if _, ok := NonDomainCategories["url"]; ok {
		t.Error("NonDomainCategories should not contain 'url'")
	}
}

func TestIsBlacklisted(t *testing.T) {
	tests := []struct {
		domain string
		want   bool
	}{
		{"test.amazonaws.com", true},
		{"app.herokuapp.com", true},
		{"user.github.io", true},
		{"example.com", false},
		{"my-company.com", false},
		{"amazonaws.com", true}, // exact match
		{"sub.vercel.app", true},
	}

	for _, tt := range tests {
		t.Run(tt.domain, func(t *testing.T) {
			got := isBlacklisted(tt.domain)
			if got != tt.want {
				t.Errorf("isBlacklisted(%q) = %v, want %v", tt.domain, got, tt.want)
			}
		})
	}
}

// isBlacklisted checks if a domain ends with any blacklisted suffix
func isBlacklisted(domain string) bool {
	for _, suffix := range BlacklistedSuffixes {
		if domain == suffix || len(domain) > len(suffix) && domain[len(domain)-len(suffix)-1] == '.' && domain[len(domain)-len(suffix):] == suffix {
			return true
		}
	}
	return false
}

func TestExtractDomain(t *testing.T) {
	tests := []struct {
		name   string
		target string
		want   string
	}{
		{"wildcard", "*.example.com", "example.com"},
		{"double wildcard", "*.*.example.com", "example.com"},
		{"plain domain", "example.com", "example.com"},
		{"with subdomain", "sub.example.com", "sub.example.com"},
		{"url with path", "https://example.com/path", "example.com"},
		{"url with port", "https://example.com:8080", "example.com:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractDomain(tt.target)
			if got != tt.want {
				t.Errorf("extractDomain(%q) = %q, want %q", tt.target, got, tt.want)
			}
		})
	}
}

// extractDomain extracts the domain from a target string
func extractDomain(target string) string {
	// Remove wildcard prefix
	target = trimWildcardPrefix(target)

	// Try to parse as URL
	if u, err := parseURL(target); err == nil && u.Host != "" {
		return u.Host
	}

	return target
}

func trimWildcardPrefix(s string) string {
	for len(s) > 0 && (s[0] == '*' || s[0] == '.') {
		s = s[1:]
	}
	return s
}

func parseURL(target string) (*struct{ Host string }, error) {
	// Simple URL parsing for testing
	if len(target) > 8 && (target[:8] == "https://" || target[:7] == "http://") {
		// Find the host part
		start := 7
		if target[:8] == "https://" {
			start = 8
		}
		end := start
		for end < len(target) && target[end] != '/' && target[end] != '?' {
			end++
		}
		return &struct{ Host string }{Host: target[start:end]}, nil
	}
	return &struct{ Host string }{}, nil
}

func TestResultStruct(t *testing.T) {
	r := Result{
		Domain:      "*.example.com",
		ProgramURLs: []string{"https://hackerone.com/example", "https://bugcrowd.com/example"},
	}

	if r.Domain != "*.example.com" {
		t.Errorf("Result.Domain = %q, want %q", r.Domain, "*.example.com")
	}

	if len(r.ProgramURLs) != 2 {
		t.Errorf("Result.ProgramURLs length = %d, want 2", len(r.ProgramURLs))
	}
}

func TestOptionsStruct(t *testing.T) {
	// Test default options
	opts := Options{}
	if opts.Aggressive {
		t.Error("Default Options.Aggressive should be false")
	}

	// Test with aggressive mode
	opts = Options{Aggressive: true}
	if !opts.Aggressive {
		t.Error("Options.Aggressive should be true when set")
	}
}
