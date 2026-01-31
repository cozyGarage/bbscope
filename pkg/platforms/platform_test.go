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

// MockPoller implements PlatformPoller for testing
type MockPoller struct {
	name       string
	authCalled bool
	handles    []string
	authError  error
}

func NewMockPoller(name string) *MockPoller {
	return &MockPoller{
		name:    name,
		handles: []string{"program1", "program2"},
	}
}

func (m *MockPoller) Name() string {
	return m.name
}

func (m *MockPoller) Authenticate(ctx context.Context, cfg AuthConfig) error {
	m.authCalled = true
	return m.authError
}

func (m *MockPoller) ListProgramHandles(ctx context.Context, opts PollOptions) ([]string, error) {
	return m.handles, nil
}

func (m *MockPoller) FetchProgramScope(ctx context.Context, handle string, opts PollOptions) (interface{}, error) {
	return nil, nil
}

func TestMockPollerInterface(t *testing.T) {
	mock := NewMockPoller("test")
	
	// Test Name()
	if mock.Name() != "test" {
		t.Errorf("Name() = %v, want %v", mock.Name(), "test")
	}
	
	// Test Authenticate()
	ctx := context.Background()
	err := mock.Authenticate(ctx, AuthConfig{Username: "user", Token: "token"})
	if err != nil {
		t.Errorf("Authenticate() error = %v", err)
	}
	if !mock.authCalled {
		t.Error("Authenticate() was not called")
	}
	
	// Test ListProgramHandles()
	handles, err := mock.ListProgramHandles(ctx, PollOptions{})
	if err != nil {
		t.Errorf("ListProgramHandles() error = %v", err)
	}
	if len(handles) != 2 {
		t.Errorf("ListProgramHandles() returned %d handles, want 2", len(handles))
	}
}
