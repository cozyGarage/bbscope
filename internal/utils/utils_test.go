package utils

import (
	"strings"
	"testing"
)

func TestAreSlicesEqual(t *testing.T) {
	tests := []struct {
		name string
		a    []string
		b    []string
		want bool
	}{
		{"both empty", []string{}, []string{}, true},
		{"equal single", []string{"a"}, []string{"a"}, true},
		{"equal multiple", []string{"a", "b", "c"}, []string{"a", "b", "c"}, true},
		{"different length", []string{"a"}, []string{"a", "b"}, false},
		{"different content", []string{"a", "b"}, []string{"a", "c"}, false},
		{"same content different order", []string{"a", "b"}, []string{"b", "a"}, false},
		{"nil and empty", nil, []string{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AreSlicesEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("AreSlicesEqual(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestIsCIDR(t *testing.T) {
	tests := []struct {
		name string
		cidr string
		want bool
	}{
		{"valid /24", "192.168.1.0/24", true},
		{"valid /32", "10.0.0.1/32", true},
		{"valid /0", "0.0.0.0/0", true},
		{"valid /16", "172.16.0.0/16", true},
		{"IPv6 valid", "2001:db8::/32", true},
		{"no slash", "192.168.1.0", false},
		{"invalid prefix", "192.168.1.0/33", false},
		{"invalid IP", "256.1.1.1/24", false},
		{"empty", "", false},
		{"just slash", "/24", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsCIDR(tt.cidr)
			if got != tt.want {
				t.Errorf("IsCIDR(%q) = %v, want %v", tt.cidr, got, tt.want)
			}
		})
	}
}

func TestIsIP(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		want bool
	}{
		{"valid IPv4", "192.168.1.1", true},
		{"localhost", "127.0.0.1", true},
		{"zeros", "0.0.0.0", true},
		{"max IPv4", "255.255.255.255", true},
		{"valid IPv6", "2001:db8::1", true},
		{"IPv6 localhost", "::1", true},
		{"IPv6 full", "2001:0db8:85a3:0000:0000:8a2e:0370:7334", true},
		{"IPv6 with brackets", "[2001:db8::1]", true},
		{"invalid octet", "256.1.1.1", false},
		{"empty", "", false},
		{"domain", "example.com", false},
		{"with port", "192.168.1.1:8080", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsIP(tt.ip)
			if got != tt.want {
				t.Errorf("IsIP(%q) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestIsIPRange(t *testing.T) {
	tests := []struct {
		name    string
		ipRange string
		want    bool
	}{
		{"valid range", "192.168.1.1-192.168.1.255", true},
		{"single IP range", "10.0.0.1-10.0.0.1", true},
		{"with spaces", "192.168.1.1 - 192.168.1.255", true},
		{"no hyphen", "192.168.1.1", false},
		{"invalid start", "256.1.1.1-192.168.1.255", false},
		{"invalid end", "192.168.1.1-256.1.1.1", false},
		{"empty", "", false},
		{"just hyphen", "-", false},
		{"multiple hyphens", "192.168.1.1-192.168.1.100-192.168.1.255", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsIPRange(tt.ipRange)
			if got != tt.want {
				t.Errorf("IsIPRange(%q) = %v, want %v", tt.ipRange, got, tt.want)
			}
		})
	}
}

func TestRedactURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			"with password",
			"postgres://user:secret@localhost/db",
			"postgres://user:%2A%2A%2A%2A@localhost/db", // URL-encoded ****
		},
		{
			"without password",
			"postgres://user@localhost/db",
			"postgres://user@localhost/db",
		},
		{
			"no credentials",
			"postgres://localhost/db",
			"postgres://localhost/db",
		},
		{
			"with port and password",
			"postgres://admin:p@ssw0rd@localhost:5432/mydb?sslmode=disable",
			"postgres://admin:%2A%2A%2A%2A@localhost:5432/mydb?sslmode=disable", // URL-encoded ****
		},
		{
			"invalid URL",
			"not a valid url ::::",
			"[invalid URL]",
		},
		{
			"http URL",
			"http://user:pass@example.com/path",
			"http://user:%2A%2A%2A%2A@example.com/path", // URL-encoded ****
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RedactURL(tt.url)
			if got != tt.want {
				t.Errorf("RedactURL(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestSetLogLevel(t *testing.T) {
	tests := []struct {
		name  string
		level string
	}{
		{"debug", "debug"},
		{"info", "info"},
		{"warn", "warn"},
		{"warning", "warning"},
		{"error", "error"},
		{"uppercase", "DEBUG"},
		{"mixed case", "Info"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			SetLogLevel(tt.level)
		})
	}
}

// TestRedactURLRefusesNonURLConnectionStrings covers libpq keyword/value DSNs,
// which url.Parse accepts without populating User — previously causing the raw
// password to be echoed back to the caller for logging.
func TestRedactURLRefusesNonURLConnectionStrings(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"url with password", "postgres://u:s3cret@host/db", "postgres://u:%2A%2A%2A%2A@host/db"},
		{"keyword value dsn", "host=db user=u password=s3cret", "[redacted non-URL connection string]"},
		{"url without password", "postgres://u@host/db", "postgres://u@host/db"},
		{"host and port is not a URL", "127.0.0.1:5432", "[invalid URL]"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := RedactURL(tc.in)
			if got != tc.want {
				t.Errorf("RedactURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if strings.Contains(got, "s3cret") {
				t.Errorf("RedactURL(%q) leaked the password: %q", tc.in, got)
			}
		})
	}
}
