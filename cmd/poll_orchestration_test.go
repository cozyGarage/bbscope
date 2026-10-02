package cmd

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"

	"github.com/cozyGarage/bbscope/v2/pkg/platforms"
)

func newOrchestrationTestCmd() *cobra.Command {
	c := &cobra.Command{Use: "poll"}
	c.Flags().String("category", "all", "")
	c.Flags().Bool("db", false, "")
	c.Flags().Bool("ai", false, "")
	c.Flags().Int("concurrency", 2, "")
	c.Flags().Bool("bbp-only", false, "")
	c.Flags().Bool("private-only", false, "")
	c.Flags().String("output", "tu", "")
	c.Flags().String("delimiter", " ", "")
	c.Flags().Bool("oos", false, "")
	return c
}

func TestRunPollWithPollers_HappyPath(t *testing.T) {
	mock := platforms.NewMockPoller("mock")
	cmd := newOrchestrationTestCmd()

	if err := runPollWithPollers(cmd, []platforms.PlatformPoller{mock}); err != nil {
		t.Fatalf("runPollWithPollers: %v", err)
	}
	fetched := mock.FetchedHandles()
	if len(fetched) != len(mock.Handles) {
		t.Fatalf("expected %d fetches, got %d (%v)", len(mock.Handles), len(fetched), fetched)
	}
}

func TestRunPollWithPollers_PerProgramErrorContinues(t *testing.T) {
	mock := platforms.NewMockPoller("mock")
	mock.FailOn = map[string]bool{"program1": true}
	cmd := newOrchestrationTestCmd()

	// Partial fetch failures skip platform sync but still fail the run so
	// scripts can tell the poll was incomplete.
	if err := runPollWithPollers(cmd, []platforms.PlatformPoller{mock}); err == nil {
		t.Fatal("expected a per-program fetch failure to fail the run")
	}
	if got := len(mock.FetchedHandles()); got != len(mock.Handles) {
		t.Fatalf("expected remaining programs to still be fetched, got %d", got)
	}
}

func TestRunPollWithPollers_ListError(t *testing.T) {
	failing := platforms.NewMockPoller("down")
	failing.ListError = errors.New("list failed")
	ok := platforms.NewMockPoller("ok")
	cmd := newOrchestrationTestCmd()

	err := runPollWithPollers(cmd, []platforms.PlatformPoller{failing, ok})
	if err == nil {
		t.Fatal("expected a list error to fail the run")
	}
	if got := len(ok.FetchedHandles()); got != len(ok.Handles) {
		t.Fatalf("a list failure on one platform must not skip the others: fetched %d", got)
	}
	if got := len(failing.FetchedHandles()); got != 0 {
		t.Fatalf("the failing platform should not have been fetched, got %d", got)
	}
}

func TestEmptyListingWouldWipe(t *testing.T) {
	tests := []struct {
		listed, stored int
		want           bool
	}{
		{0, 0, false},
		{0, 1, true},
		{0, 11, true},
		{1, 1, false},
		{1, 0, false},
	}
	for _, tt := range tests {
		if got := emptyListingWouldWipe(tt.listed, tt.stored); got != tt.want {
			t.Errorf("emptyListingWouldWipe(%d, %d) = %v, want %v", tt.listed, tt.stored, got, tt.want)
		}
	}
}

func TestRunPollWithPollers_EmptyHandles(t *testing.T) {
	mock := platforms.NewMockPoller("mock")
	mock.Handles = nil
	cmd := newOrchestrationTestCmd()

	if err := runPollWithPollers(cmd, []platforms.PlatformPoller{mock}); err != nil {
		t.Fatalf("empty handle list should succeed: %v", err)
	}
}

// One platform's auth failure used to abort `bbscope poll` before any of the
// platforms that did authenticate were polled.
func TestPollAvailable_AuthFailureStillPollsOthers(t *testing.T) {
	mock := platforms.NewMockPoller("mock")
	authErr := errors.New("bugcrowd: 403")

	err := pollAvailable(newOrchestrationTestCmd(), []platforms.PlatformPoller{mock}, authErr)
	if !errors.Is(err, authErr) {
		t.Fatalf("err = %v, want the auth error kept for the exit code", err)
	}
	if got := len(mock.FetchedHandles()); got != 2 {
		t.Fatalf("fetched %d programs, want 2: authenticated platforms must still be polled", got)
	}
}
