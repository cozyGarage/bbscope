package platforms

import (
	"context"
	"testing"
)

func TestPollOptionsDefaults(t *testing.T) {
	opts := PollOptions{}

	if opts.BountyOnly != false {
		t.Errorf("BountyOnly should default to false, got %v", opts.BountyOnly)
	}

	if opts.PrivateOnly != false {
		t.Errorf("PrivateOnly should default to false, got %v", opts.PrivateOnly)
	}

	if opts.Categories != "" {
		t.Errorf("Categories should default to empty string, got %v", opts.Categories)
	}
}

func TestAuthConfigDefaults(t *testing.T) {
	cfg := AuthConfig{}

	if cfg.Username != "" {
		t.Errorf("Username should default to empty string, got %v", cfg.Username)
	}

	if cfg.Email != "" {
		t.Errorf("Email should default to empty string, got %v", cfg.Email)
	}

	if cfg.Password != "" {
		t.Errorf("Password should default to empty string, got %v", cfg.Password)
	}

	if cfg.Token != "" {
		t.Errorf("Token should default to empty string, got %v", cfg.Token)
	}

	if cfg.OtpSecret != "" {
		t.Errorf("OtpSecret should default to empty string, got %v", cfg.OtpSecret)
	}

	if cfg.Proxy != "" {
		t.Errorf("Proxy should default to empty string, got %v", cfg.Proxy)
	}
}

func TestMockPollerInterface(t *testing.T) {
	mock := NewMockPoller("test")

	if mock.Name() != "test" {
		t.Errorf("Name() = %v, want %v", mock.Name(), "test")
	}

	ctx := context.Background()
	err := mock.Authenticate(ctx, AuthConfig{Username: "user", Token: "token"})
	if err != nil {
		t.Errorf("Authenticate() error = %v", err)
	}
	if !mock.AuthCalled {
		t.Error("Authenticate() was not called")
	}

	handles, err := mock.ListProgramHandles(ctx, PollOptions{})
	if err != nil {
		t.Errorf("ListProgramHandles() error = %v", err)
	}
	if len(handles) != 2 {
		t.Errorf("ListProgramHandles() returned %d handles, want 2", len(handles))
	}

	pd, err := mock.FetchProgramScope(ctx, "program1", PollOptions{})
	if err != nil {
		t.Fatalf("FetchProgramScope: %v", err)
	}
	if pd.Url != "https://example.com/program1" {
		t.Errorf("Url = %q", pd.Url)
	}
}
