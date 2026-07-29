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

	// Partial fetch failures are logged and skipped for platform sync; the
	// overall run still returns nil so other platforms can proceed.
	if err := runPollWithPollers(cmd, []platforms.PlatformPoller{mock}); err != nil {
		t.Fatalf("runPollWithPollers should not abort on per-program errors: %v", err)
	}
}

func TestRunPollWithPollers_ListError(t *testing.T) {
	mock := platforms.NewMockPoller("mock")
	mock.ListError = errors.New("list failed")
	cmd := newOrchestrationTestCmd()

	err := runPollWithPollers(cmd, []platforms.PlatformPoller{mock})
	if err == nil {
		t.Fatal("expected list error to abort the run")
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
