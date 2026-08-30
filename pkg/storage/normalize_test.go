package storage

import "testing"

func TestNormalizeTarget(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"https://Example.COM/path/", "https://example.com/path"},
		{"HTTPS://EXAMPLE.COM:443/a", "https://example.com/a"},
		{"*.Example.COM.", "*.example.com"},
		{"Example.COM/", "example.com"},
		{"", ""},
	}
	for _, tc := range tests {
		if got := NormalizeTarget(tc.in); got != tc.want {
			t.Errorf("NormalizeTarget(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeProgramURL(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"https://HackerOne.com/foo/", "https://hackerone.com/foo"},
		{"https://hackerone.com/foo", "https://hackerone.com/foo"},
		{"https://hackerone.com/", "https://hackerone.com"},
		{"https://hackerone.com", "https://hackerone.com"},
		{"", ""},
	}
	for _, tc := range tests {
		if got := NormalizeProgramURL(tc.in); got != tc.want {
			t.Errorf("NormalizeProgramURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestIdentityKeyUsesNormalization(t *testing.T) {
	a := identityKey("https://Example.com/a/", "Website")
	b := identityKey("https://example.com/a", "url")
	if a == "" || a != b {
		t.Fatalf("identity keys should match after normalize: %q vs %q", a, b)
	}
	if identityKey("evil.com", "url") == a {
		t.Fatal("different hosts must not share identity keys")
	}
}
