package dev

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
}

func TestPollerName(t *testing.T) {
	p := NewPoller()
	if got := p.Name(); got != "dev" {
		t.Errorf("Name() = %v, want %v", got, "dev")
	}
}

func TestPollerAuthenticate(t *testing.T) {
	p := NewPoller()
	ctx := context.Background()

	// Dev poller Authenticate should always return nil
	err := p.Authenticate(ctx, platforms.AuthConfig{
		Username: "any",
		Token:    "any",
	})

	if err != nil {
		t.Errorf("Authenticate() error = %v, want nil", err)
	}
}

func TestPollerListProgramHandles(t *testing.T) {
	p := NewPoller()
	ctx := context.Background()

	handles, err := p.ListProgramHandles(ctx, platforms.PollOptions{})
	if err != nil {
		t.Fatalf("ListProgramHandles() error = %v", err)
	}

	// Should return hardcoded test handles
	if len(handles) != 3 {
		t.Errorf("ListProgramHandles() returned %d handles, want 3", len(handles))
	}

	expectedHandles := []string{
		"https://example.com/program/a",
		"https://example.com/program/b",
		"https://example.com/program/c",
	}

	for i, expected := range expectedHandles {
		if handles[i] != expected {
			t.Errorf("ListProgramHandles()[%d] = %v, want %v", i, handles[i], expected)
		}
	}
}

func TestPollerFetchProgramScope(t *testing.T) {
	p := NewPoller()
	ctx := context.Background()

	tests := []struct {
		name            string
		handle          string
		wantInScopeLen  int
		wantOutScopeLen int
	}{
		{
			name:            "program a",
			handle:          "https://example.com/program/a",
			wantInScopeLen:  3,
			wantOutScopeLen: 2,
		},
		{
			name:            "program b",
			handle:          "https://example.com/program/b",
			wantInScopeLen:  1,
			wantOutScopeLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pd, err := p.FetchProgramScope(ctx, tt.handle, platforms.PollOptions{})
			if err != nil {
				t.Fatalf("FetchProgramScope() error = %v", err)
			}

			if pd.Url != tt.handle {
				t.Errorf("FetchProgramScope().Url = %v, want %v", pd.Url, tt.handle)
			}

			if len(pd.InScope) != tt.wantInScopeLen {
				t.Errorf("FetchProgramScope().InScope length = %d, want %d", len(pd.InScope), tt.wantInScopeLen)
			}

			if len(pd.OutOfScope) != tt.wantOutScopeLen {
				t.Errorf("FetchProgramScope().OutOfScope length = %d, want %d", len(pd.OutOfScope), tt.wantOutScopeLen)
			}
		})
	}
}

func TestPollerImplementsInterface(t *testing.T) {
	// Compile-time check that Poller implements PlatformPoller
	var _ platforms.PlatformPoller = (*Poller)(nil)
}
