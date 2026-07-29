package platforms

import (
	"context"
	"errors"
	"sync"

	"github.com/cozyGarage/bbscope/v2/pkg/scope"
)

// MockPoller implements PlatformPoller for tests without network I/O.
type MockPoller struct {
	name          string
	AuthCalled    bool
	Handles       []string
	AuthError     error
	ListError     error
	FailOn        map[string]bool
	mu            sync.Mutex
	Fetched       []string
	ScopeByHandle map[string]scope.ProgramData
}

// NewMockPoller returns a poller that lists two sample program handles.
func NewMockPoller(name string) *MockPoller {
	return &MockPoller{
		name:    name,
		Handles: []string{"program1", "program2"},
	}
}

func (m *MockPoller) Name() string { return m.name }

func (m *MockPoller) Authenticate(ctx context.Context, cfg AuthConfig) error {
	m.AuthCalled = true
	return m.AuthError
}

func (m *MockPoller) ListProgramHandles(ctx context.Context, opts PollOptions) ([]string, error) {
	if m.ListError != nil {
		return nil, m.ListError
	}
	return append([]string(nil), m.Handles...), nil
}

func (m *MockPoller) FetchProgramScope(ctx context.Context, handle string, opts PollOptions) (scope.ProgramData, error) {
	m.mu.Lock()
	m.Fetched = append(m.Fetched, handle)
	m.mu.Unlock()

	if m.FailOn[handle] {
		return scope.ProgramData{}, errors.New("boom")
	}
	if m.ScopeByHandle != nil {
		if pd, ok := m.ScopeByHandle[handle]; ok {
			return pd, nil
		}
	}
	return scope.ProgramData{
		Url:     "https://example.com/" + handle,
		InScope: []scope.ScopeElement{{Target: handle + ".example.com", Category: "url"}},
	}, nil
}

// FetchedHandles returns a snapshot of handles passed to FetchProgramScope.
func (m *MockPoller) FetchedHandles() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.Fetched...)
}

// Ensure MockPoller satisfies PlatformPoller at compile time.
var _ PlatformPoller = (*MockPoller)(nil)
