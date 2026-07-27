package storage

import "testing"

func TestAggressiveTransform(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"subdomain to wildcard", "sub.example.com", "*.example.com"},
		{"deep subdomain to registrable wildcard", "a.b.example.co.uk", "*.example.co.uk"},
		{"url to wildcard", "https://portal.example.com/login", "*.example.com"},
		{"already wildcard unchanged", "*.example.com", "*.example.com"},
		{"ip unchanged", "203.0.113.5", "203.0.113.5"},
		{"non-domain unchanged", "localhost", "localhost"},
		{"empty stays empty", "", ""},
		{"whitespace trimmed", "  sub.example.com  ", "*.example.com"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := AggressiveTransform(tc.input); got != tc.want {
				t.Errorf("AggressiveTransform(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestExtractRootDomain(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantRoot string
		wantOK   bool
	}{
		{"plain domain", "example.com", "example.com", true},
		{"subdomain", "sub.example.com", "example.com", true},
		{"url with path", "http://sub.foo.example.co.uk/path", "example.co.uk", true},
		{"bare host with port", "sub.example.com:8443", "example.com", true},
		{"wildcard rejected", "*.example.com", "", false},
		{"ip rejected", "203.0.113.5", "", false},
		{"no dot rejected", "localhost", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root, ok := ExtractRootDomain(tc.input)
			if ok != tc.wantOK || root != tc.wantRoot {
				t.Errorf("ExtractRootDomain(%q) = (%q, %v), want (%q, %v)", tc.input, root, ok, tc.wantRoot, tc.wantOK)
			}
		})
	}
}
