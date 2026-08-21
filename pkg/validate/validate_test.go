package validate

import (
	"testing"
)

func TestDomain(t *testing.T) {
	tests := []struct {
		name    string
		domain  string
		wantErr bool
	}{
		{"valid domain", "example.com", false},
		{"valid subdomain", "sub.example.com", false},
		{"valid deep subdomain", "a.b.c.example.com", false},
		{"empty", "", true},
		{"no tld", "example", true},
		{"starts with hyphen", "-example.com", true},
		{"ends with hyphen", "example-.com", true},
		{"double dot", "example..com", true},
		{"spaces", "example .com", true},
		{"wildcard not allowed", "*.example.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Domain(tt.domain)
			if (err != nil) != tt.wantErr {
				t.Errorf("Domain(%q) error = %v, wantErr %v", tt.domain, err, tt.wantErr)
			}
		})
	}
}

func TestWildcard(t *testing.T) {
	tests := []struct {
		name     string
		wildcard string
		wantErr  bool
	}{
		{"valid wildcard", "*.example.com", false},
		{"valid deep wildcard", "*.sub.example.com", false},
		{"empty", "", true},
		{"no asterisk", "example.com", true},
		{"double asterisk", "**.example.com", true},
		{"asterisk in middle", "sub.*.example.com", true},
		{"just asterisk", "*", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Wildcard(tt.wildcard)
			if (err != nil) != tt.wantErr {
				t.Errorf("Wildcard(%q) error = %v, wantErr %v", tt.wildcard, err, tt.wantErr)
			}
		})
	}
}

func TestIPv4(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		wantErr bool
	}{
		{"valid ip", "192.168.1.1", false},
		{"localhost", "127.0.0.1", false},
		{"zeros", "0.0.0.0", false},
		{"max values", "255.255.255.255", false},
		{"empty", "", true},
		{"invalid octet", "256.1.1.1", true},
		{"too few octets", "192.168.1", true},
		{"too many octets", "192.168.1.1.1", true},
		{"letters", "192.168.1.a", true},
		{"negative", "-1.168.1.1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := IPv4(tt.ip)
			if (err != nil) != tt.wantErr {
				t.Errorf("IPv4(%q) error = %v, wantErr %v", tt.ip, err, tt.wantErr)
			}
		})
	}
}

func TestCIDR(t *testing.T) {
	tests := []struct {
		name    string
		cidr    string
		wantErr bool
	}{
		{"valid /24", "192.168.1.0/24", false},
		{"valid /32", "192.168.1.1/32", false},
		{"valid /0", "0.0.0.0/0", false},
		{"empty", "", true},
		{"no prefix", "192.168.1.0", true},
		{"invalid prefix", "192.168.1.0/33", true},
		{"negative prefix", "192.168.1.0/-1", true},
		{"invalid ip", "256.168.1.0/24", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CIDR(tt.cidr)
			if (err != nil) != tt.wantErr {
				t.Errorf("CIDR(%q) error = %v, wantErr %v", tt.cidr, err, tt.wantErr)
			}
		})
	}
}

func TestURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"valid http", "http://example.com", false},
		{"valid https", "https://example.com", false},
		{"with path", "https://example.com/path", false},
		{"with query", "https://example.com?foo=bar", false},
		{"with port", "https://example.com:8080", false},
		{"empty", "", true},
		{"no scheme", "example.com", true},
		{"no host", "http://", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := URL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("URL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestPlatform(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		wantErr  bool
	}{
		{"hackerone", "hackerone", false},
		{"h1 alias", "h1", false},
		{"bugcrowd", "bugcrowd", false},
		{"bc alias", "bc", false},
		{"intigriti", "intigriti", false},
		{"it alias", "it", false},
		{"yeswehack", "yeswehack", false},
		{"ywh alias", "ywh", false},
		{"immunefi", "immunefi", false},
		{"uppercase", "HACKERONE", false},
		{"mixed case", "HackerOne", false},
		{"invalid", "github", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Platform(tt.platform)
			if (err != nil) != tt.wantErr {
				t.Errorf("Platform(%q) error = %v, wantErr %v", tt.platform, err, tt.wantErr)
			}
		})
	}
}

func TestDatabaseURL(t *testing.T) {
	tests := []struct {
		name          string
		url           string
		wantErr       bool
		wantSanitized string
	}{
		{
			name:          "valid with password",
			url:           "postgres://user:secret@localhost/db",
			wantErr:       false,
			wantSanitized: "postgres://user:****@localhost/db",
		},
		{
			name:          "valid without password",
			url:           "postgres://localhost/db",
			wantErr:       false,
			wantSanitized: "postgres://localhost/db",
		},
		{
			name:    "invalid scheme",
			url:     "mysql://localhost/db",
			wantErr: true,
		},
		{
			name:    "empty",
			url:     "",
			wantErr: true,
		},
		{
			name:          "keyword/value DSN",
			url:           "host=localhost port=5432 user=bbscope password=secret dbname=bbscope",
			wantErr:       false,
			wantSanitized: "host=localhost port=5432 user=bbscope password=secret dbname=bbscope",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sanitized, err := DatabaseURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("DatabaseURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
			if !tt.wantErr && sanitized != tt.wantSanitized {
				t.Errorf("DatabaseURL(%q) sanitized = %q, want %q", tt.url, sanitized, tt.wantSanitized)
			}
		})
	}
}

func TestHandle(t *testing.T) {
	tests := []struct {
		name    string
		handle  string
		wantErr bool
	}{
		{"simple", "myprogram", false},
		{"with numbers", "program123", false},
		{"with hyphen", "my-program", false},
		{"with underscore", "my_program", false},
		{"empty", "", true},
		{"starts with hyphen", "-program", true},
		{"special chars", "my@program", true},
		{"spaces", "my program", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Handle(tt.handle)
			if (err != nil) != tt.wantErr {
				t.Errorf("Handle(%q) error = %v, wantErr %v", tt.handle, err, tt.wantErr)
			}
		})
	}
}
