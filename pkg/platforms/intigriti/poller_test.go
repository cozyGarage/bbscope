package intigriti

import (
	"context"
	"testing"

	"github.com/cozyGarage/bbscope/v2/pkg/platforms"
)

func TestNewPoller(t *testing.T) {
	p := NewPoller()
	
	if p == nil {
		t.Fatal("NewPoller() returned nil")
	}
	
	if p.urlToID == nil {
		t.Error("urlToID map should be initialized")
	}
	
	if p.handleToURL == nil {
		t.Error("handleToURL map should be initialized")
	}
}

func TestPollerName(t *testing.T) {
	p := NewPoller()
	if got := p.Name(); got != "it" {
		t.Errorf("Name() = %v, want %v", got, "it")
	}
}

func TestPollerAuthenticate(t *testing.T) {
	p := NewPoller()
	ctx := context.Background()

	tests := []struct {
		name      string
		cfg       platforms.AuthConfig
		wantToken string
	}{
		{
			name: "with token",
			cfg: platforms.AuthConfig{
				Token: "test-bearer-token",
			},
			wantToken: "test-bearer-token",
		},
		{
			name:      "empty token keeps existing",
			cfg:       platforms.AuthConfig{Token: ""},
			wantToken: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p.token = "" // Reset
			err := p.Authenticate(ctx, tt.cfg)
			if err != nil {
				t.Errorf("Authenticate() error = %v", err)
			}
			if p.token != tt.wantToken {
				t.Errorf("Authenticate() token = %v, want %v", p.token, tt.wantToken)
			}
		})
	}
}

func TestPollerImplementsInterface(t *testing.T) {
	// Compile-time check that Poller implements PlatformPoller
	var _ platforms.PlatformPoller = (*Poller)(nil)
}

func TestPollerMapsInitialization(t *testing.T) {
	p := NewPoller()
	
	// Test that maps are writable
	p.urlToID["test-url"] = "test-id"
	if p.urlToID["test-url"] != "test-id" {
		t.Error("urlToID map should be writable")
	}
	
	p.handleToURL["test-handle"] = "test-url"
	if p.handleToURL["test-handle"] != "test-url" {
		t.Error("handleToURL map should be writable")
	}
}
