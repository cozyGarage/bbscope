package cmd

import (
	"context"
	"testing"

	"github.com/spf13/cobra"

	"github.com/cozyGarage/bbscope/v2/pkg/platforms"
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
	urls, err := processProgramsConcurrently(context.Background(), newPollTestCmd(), p, handles, platforms.PollOptions{}, false, nil, nil, true, 3, nil)
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
	urls, err := processProgramsConcurrently(context.Background(), newPollTestCmd(), p, handles, platforms.PollOptions{}, false, nil, nil, true, 2, nil)
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

	urls, _ := processProgramsConcurrently(ctx, newPollTestCmd(), p, handles, platforms.PollOptions{}, false, nil, nil, true, 2, nil)
	if len(urls) != 0 {
		t.Fatalf("workers should bail out on a canceled context, but processed %d (%v)", len(urls), urls)
	}
}
