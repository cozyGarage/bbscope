package yeswehack

import (
	"context"
	"testing"

	"github.com/cozyGarage/bbscope/v2/pkg/platforms"
)

func TestNewPoller(t *testing.T) {
	tests := []struct {
		name      string
		token     string
		wantToken string
	}{
		{
			name:      "with token",
			token:     "test-token",
			wantToken: "test-token",
		},
		{
			name:      "empty token",
			token:     "",
			wantToken: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPoller(tt.token)
			if p == nil {
				t.Fatal("NewPoller() returned nil")
			}
			if p.token != tt.wantToken {
				t.Errorf("NewPoller() token = %v, want %v", p.token, tt.wantToken)
			}
		})
	}
}

func TestPollerName(t *testing.T) {
	p := NewPoller("")
	if got := p.Name(); got != "ywh" {
		t.Errorf("Name() = %v, want %v", got, "ywh")
	}
}

func TestPollerAuthenticate(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		cfg       platforms.AuthConfig
		wantToken string
		wantError bool
	}{
		{
			name: "with direct token",
			cfg: platforms.AuthConfig{
				Token: "direct-bearer-token",
			},
			wantToken: "direct-bearer-token",
			wantError: false,
		},
		{
			name:      "empty config keeps existing",
			cfg:       platforms.AuthConfig{},
			wantToken: "existing-token",
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPoller("existing-token")
			err := p.Authenticate(ctx, tt.cfg)

			if (err != nil) != tt.wantError {
				t.Errorf("Authenticate() error = %v, wantError %v", err, tt.wantError)
				return
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
