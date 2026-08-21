package cmd

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/cozyGarage/bbscope/v2/pkg/platforms"
	"github.com/cozyGarage/bbscope/v2/pkg/scope"
)

func newPollTestCmd() *cobra.Command {
	c := &cobra.Command{}
	c.Flags().String("output", "tu", "")
	c.Flags().String("delimiter", " ", "")
	c.Flags().Bool("oos", false, "")
	return c
}

func TestProcessProgramsConcurrently_AllProcessed(t *testing.T) {
	p := platforms.NewMockPoller("fake")
	p.Handles = []string{"a", "b", "c", "d", "e"}
	handles := p.Handles
	urls, err := processProgramsConcurrently(context.Background(), newPollTestCmd(), p, handles, platforms.PollOptions{}, false, nil, nil, true, 3, nil, time.Time{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(urls) != len(handles) {
		t.Fatalf("expected %d program URLs, got %d", len(handles), len(urls))
	}
	if len(p.FetchedHandles()) != len(handles) {
		t.Fatalf("expected all %d handles fetched, got %d", len(handles), len(p.FetchedHandles()))
	}
}

func TestProcessProgramsConcurrently_ErrorIsolation(t *testing.T) {
	p := platforms.NewMockPoller("fake")
	p.FailOn = map[string]bool{"b": true}
	handles := []string{"a", "b", "c"}
	urls, err := processProgramsConcurrently(context.Background(), newPollTestCmd(), p, handles, platforms.PollOptions{}, false, nil, nil, true, 2, nil, time.Time{})
	if err == nil {
		t.Fatal("expected an error to be surfaced when one handle fails")
	}
	if len(urls) != 2 {
		t.Fatalf("expected the 2 healthy handles to still succeed, got %d (%v)", len(urls), urls)
	}
}

func TestProcessProgramsConcurrently_ContextCanceled(t *testing.T) {
	p := platforms.NewMockPoller("fake")
	handles := []string{"a", "b", "c", "d"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before any work starts

	urls, err := processProgramsConcurrently(ctx, newPollTestCmd(), p, handles, platforms.PollOptions{}, false, nil, nil, true, 2, nil, time.Time{})
	if len(urls) != 0 {
		t.Fatalf("workers should bail out on a canceled context, but processed %d (%v)", len(urls), urls)
	}
	// The error matters as much as the empty list: the caller only skips
	// SyncPlatformPrograms when an error comes back.
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected a context.Canceled error, got %v", err)
	}
}

// cancelAfterFirstFetch cancels the poll once one handle has been fetched, which
// is the dangerous shape of cancellation: the result list is partial rather than
// empty.
type cancelAfterFirstFetch struct {
	*platforms.MockPoller
	cancel func()
	once   sync.Once
}

func (c *cancelAfterFirstFetch) FetchProgramScope(ctx context.Context, handle string, opts platforms.PollOptions) (scope.ProgramData, error) {
	pd, err := c.MockPoller.FetchProgramScope(ctx, handle, opts)
	c.once.Do(c.cancel)
	return pd, err
}

// TestProcessProgramsConcurrently_CancelMidFlightReturnsError covers a Ctrl-C
// partway through a poll. Workers used to return without recording an error, so
// a truncated URL list was handed back as success and the caller went on to sync
// it — disabling every program that had not been fetched yet.
func TestProcessProgramsConcurrently_CancelMidFlightReturnsError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	base := platforms.NewMockPoller("fake")
	p := &cancelAfterFirstFetch{MockPoller: base, cancel: cancel}
	handles := []string{"a", "b", "c", "d", "e", "f", "g", "h"}

	// Concurrency 1 so the cancellation lands before the remaining handles run.
	urls, err := processProgramsConcurrently(ctx, newPollTestCmd(), p, handles, platforms.PollOptions{}, false, nil, nil, true, 1, nil, time.Time{})
	if err == nil {
		t.Fatal("a cancelled poll must surface an error so the caller skips platform sync")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected a context.Canceled error, got %v", err)
	}
	if len(urls) == len(handles) {
		t.Fatalf("expected a partial result list, got all %d handles", len(urls))
	}
}
