package cmd

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/spf13/cobra"

	"github.com/cozyGarage/bbscope/v2/pkg/platforms"
	"github.com/cozyGarage/bbscope/v2/pkg/scope"
)

// fakePoller is a minimal PlatformPoller for exercising the poll worker pool
// without any network or database access.
type fakePoller struct {
	mu     sync.Mutex
	failOn map[string]bool
	seen   []string
}

func (f *fakePoller) Name() string { return "fake" }

func (f *fakePoller) Authenticate(ctx context.Context, cfg platforms.AuthConfig) error { return nil }

func (f *fakePoller) ListProgramHandles(ctx context.Context, opts platforms.PollOptions) ([]string, error) {
	return nil, nil
}

func (f *fakePoller) FetchProgramScope(ctx context.Context, handle string, opts platforms.PollOptions) (scope.ProgramData, error) {
	f.mu.Lock()
	f.seen = append(f.seen, handle)
	f.mu.Unlock()
	if f.failOn[handle] {
		return scope.ProgramData{}, errors.New("boom")
	}
	return scope.ProgramData{
		Url:     "https://fake/" + handle,
		InScope: []scope.ScopeElement{{Target: handle + ".example.com", Category: "url"}},
	}, nil
}

func newPollTestCmd() *cobra.Command {
	c := &cobra.Command{}
	c.Flags().String("output", "tu", "")
	c.Flags().String("delimiter", " ", "")
	c.Flags().Bool("oos", false, "")
	return c
}

func TestProcessProgramsConcurrently_AllProcessed(t *testing.T) {
	p := &fakePoller{}
	handles := []string{"a", "b", "c", "d", "e"}
	urls, err := processProgramsConcurrently(context.Background(), newPollTestCmd(), p, handles, platforms.PollOptions{}, false, nil, nil, true, 3, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(urls) != len(handles) {
		t.Fatalf("expected %d program URLs, got %d", len(handles), len(urls))
	}
	if len(p.seen) != len(handles) {
		t.Fatalf("expected all %d handles fetched, got %d", len(handles), len(p.seen))
	}
}

func TestProcessProgramsConcurrently_ErrorIsolation(t *testing.T) {
	p := &fakePoller{failOn: map[string]bool{"b": true}}
	handles := []string{"a", "b", "c"}
	urls, err := processProgramsConcurrently(context.Background(), newPollTestCmd(), p, handles, platforms.PollOptions{}, false, nil, nil, true, 2, nil)
	if err == nil {
		t.Fatal("expected an error to be surfaced when one handle fails")
	}
	if len(urls) != 2 {
		t.Fatalf("expected the 2 healthy handles to still succeed, got %d (%v)", len(urls), urls)
	}
}

func TestProcessProgramsConcurrently_ContextCanceled(t *testing.T) {
	p := &fakePoller{}
	handles := []string{"a", "b", "c", "d"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before any work starts

	urls, _ := processProgramsConcurrently(ctx, newPollTestCmd(), p, handles, platforms.PollOptions{}, false, nil, nil, true, 2, nil)
	if len(urls) != 0 {
		t.Fatalf("workers should bail out on a canceled context, but processed %d (%v)", len(urls), urls)
	}
}
