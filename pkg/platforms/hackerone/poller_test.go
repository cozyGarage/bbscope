package hackerone

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/cozyGarage/bbscope/v2/pkg/platforms"
)

func TestNewPoller(t *testing.T) {
	tests := []struct {
		name     string
		username string
		token    string
		wantB64  string
	}{
		{
			name:     "basic credentials",
			username: "testuser",
			token:    "testtoken",
			wantB64:  base64.StdEncoding.EncodeToString([]byte("testuser:testtoken")),
		},
		{
			name:     "empty credentials",
			username: "",
			token:    "",
			wantB64:  base64.StdEncoding.EncodeToString([]byte(":")),
		},
		{
			name:     "special characters in token",
			username: "user@example.com",
			token:    "abc123!@#$%",
			wantB64:  base64.StdEncoding.EncodeToString([]byte("user@example.com:abc123!@#$%")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPoller(tt.username, tt.token)
			if p.authB64 != tt.wantB64 {
				t.Errorf("NewPoller() authB64 = %v, want %v", p.authB64, tt.wantB64)
			}
		})
	}
}

func TestPollerName(t *testing.T) {
	p := NewPoller("user", "token")
	if got := p.Name(); got != "h1" {
		t.Errorf("Name() = %v, want %v", got, "h1")
	}
}

func TestPollerAuthenticate(t *testing.T) {
	p := &Poller{}
	ctx := context.Background()

	tests := []struct {
		name    string
		cfg     platforms.AuthConfig
		wantB64 string
	}{
		{
			name: "with credentials",
			cfg: platforms.AuthConfig{
				Username: "newuser",
				Token:    "newtoken",
			},
			wantB64: base64.StdEncoding.EncodeToString([]byte("newuser:newtoken")),
		},
		{
			name: "empty username keeps existing",
			cfg: platforms.AuthConfig{
				Username: "",
				Token:    "token",
			},
			wantB64: "", // Should not update if username is empty
		},
		{
			name: "empty token keeps existing",
			cfg: platforms.AuthConfig{
				Username: "user",
				Token:    "",
			},
			wantB64: "", // Should not update if token is empty
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p.authB64 = "" // Reset
			err := p.Authenticate(ctx, tt.cfg)
			if err != nil {
				t.Errorf("Authenticate() error = %v", err)
			}
			if p.authB64 != tt.wantB64 {
				t.Errorf("Authenticate() authB64 = %v, want %v", p.authB64, tt.wantB64)
			}
		})
	}
}

func TestPollerImplementsInterface(t *testing.T) {
	// Compile-time check that Poller implements PlatformPoller
	var _ platforms.PlatformPoller = (*Poller)(nil)
}

func TestAuthB64Encoding(t *testing.T) {
	// Test that the base64 encoding is correct for HTTP Basic Auth
	username := "api_user"
	token := "secret_token_123"
	
	p := NewPoller(username, token)
	
	// Decode and verify
	decoded, err := base64.StdEncoding.DecodeString(p.authB64)
	if err != nil {
		t.Fatalf("Failed to decode authB64: %v", err)
	}
	
	expected := username + ":" + token
	if string(decoded) != expected {
		t.Errorf("Decoded authB64 = %v, want %v", string(decoded), expected)
	}
}
