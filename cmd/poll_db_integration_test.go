package cmd

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/spf13/viper"

	"github.com/cozyGarage/bbscope/v2/pkg/platforms"
	"github.com/cozyGarage/bbscope/v2/pkg/scope"
	"github.com/cozyGarage/bbscope/v2/pkg/storage"
)

// seedPollProgram stores one program with a url target carrying an AI variant
// plus a wildcard target, then returns a mock poller that serves only the url
// target (what --category url would return) and a raw target counter.
func seedPollProgram(t *testing.T) (*platforms.MockPoller, func() int, func() int) {
	t.Helper()
	db, raw := openRoundtripDB(t)
	ctx := context.Background()
	platform := "itest_poll_" + strconv.FormatInt(time.Now().UnixNano(), 36)
	programURL := "https://example.com/" + platform + "/a"

	cleanup := func() {
		_, _ = raw.ExecContext(ctx, `DELETE FROM targets_ai_enhanced WHERE target_id IN (
			SELECT tr.id FROM targets_raw tr JOIN programs p ON tr.program_id = p.id WHERE p.platform = $1)`, platform)
		_, _ = raw.ExecContext(ctx, `DELETE FROM targets_raw WHERE program_id IN (SELECT id FROM programs WHERE platform = $1)`, platform)
		_, _ = raw.ExecContext(ctx, `DELETE FROM scope_changes WHERE platform = $1`, platform)
		_, _ = raw.ExecContext(ctx, `DELETE FROM programs WHERE platform = $1`, platform)
	}
	cleanup()
	t.Cleanup(cleanup)

	built, err := storage.BuildEntries(programURL, platform, "a", []storage.TargetItem{
		{URI: "https://app.poll.example.com", Category: "url", InScope: true,
			Variants: []storage.TargetVariant{{Value: "app.poll.example.com", HasCategory: true, Category: "url"}}},
		{URI: "*.poll.example.com", Category: "wildcard", InScope: true},
	})
	if err != nil {
		t.Fatalf("BuildEntries: %v", err)
	}
	if _, err := db.UpsertProgramEntriesWithOptions(ctx, programURL, platform, "a", built,
		storage.UpsertOptions{SkipChangeLog: true}); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("db_url", os.Getenv("TEST_DB_URL"))

	mock := platforms.NewMockPoller(platform)
	mock.Handles = []string{"a"}
	mock.ScopeByHandle = map[string]scope.ProgramData{"a": {
		Url:     programURL,
		InScope: []scope.ScopeElement{{Target: "https://app.poll.example.com", Category: "url"}},
	}}

	count := func(query string) func() int {
		return func() int {
			var n int
			if err := raw.QueryRowContext(ctx, query, platform).Scan(&n); err != nil {
				t.Fatalf("count: %v", err)
			}
			return n
		}
	}
	targets := count(`SELECT count(*) FROM targets_raw tr JOIN programs p ON tr.program_id = p.id WHERE p.platform = $1`)
	variants := count(`SELECT count(*) FROM targets_ai_enhanced a JOIN targets_raw tr ON a.target_id = tr.id
		JOIN programs p ON tr.program_id = p.id WHERE p.platform = $1`)
	return mock, targets, variants
}

// A --category poll returns a subset of scope; storing it as the full scope
// deleted every filtered-out target.
func TestIntegration_FilteredPollKeepsFilteredOutScope(t *testing.T) {
	mock, targets, _ := seedPollProgram(t)
	cmd := newOrchestrationTestCmd()
	_ = cmd.Flags().Set("db", "true")
	_ = cmd.Flags().Set("category", "url")

	if err := runPollWithPollers(cmd, []platforms.PlatformPoller{mock}); err != nil {
		t.Fatalf("runPollWithPollers: %v", err)
	}
	if got := targets(); got != 2 {
		t.Fatalf("filtered poll left %d targets, want 2 (wildcard must survive)", got)
	}
}

// A plain --db poll (no --ai) carries no variants; it used to delete every
// stored AI variant for the targets it polled.
func TestIntegration_PollWithoutAIKeepsVariants(t *testing.T) {
	mock, _, variants := seedPollProgram(t)
	cmd := newOrchestrationTestCmd()
	_ = cmd.Flags().Set("db", "true")

	if err := runPollWithPollers(cmd, []platforms.PlatformPoller{mock}); err != nil {
		t.Fatalf("runPollWithPollers: %v", err)
	}
	if got := variants(); got != 1 {
		t.Fatalf("poll without --ai left %d AI variants, want 1", got)
	}
}

// A custom program (`db add -u`) that owns a platform's program URL used to
// fail that platform's poll, skipping its sync on every run.
func TestIntegration_PollSkipsProgramOwnedByCustom(t *testing.T) {
	mock, _, _ := seedPollProgram(t)
	db, raw := openRoundtripDB(t)
	ctx := context.Background()
	ownedURL := "https://example.com/" + mock.Name() + "/owned"
	t.Cleanup(func() {
		_, _ = raw.ExecContext(ctx, `DELETE FROM targets_raw WHERE program_id IN (SELECT id FROM programs WHERE url = $1)`, ownedURL)
		_, _ = raw.ExecContext(ctx, `DELETE FROM programs WHERE url = $1`, ownedURL)
	})
	if _, err := db.AddCustomTarget(ctx, "owned.example.com", "url", ownedURL); err != nil {
		t.Fatalf("AddCustomTarget: %v", err)
	}
	mock.Handles = append(mock.Handles, "owned")
	mock.ScopeByHandle["owned"] = scope.ProgramData{
		Url:     ownedURL,
		InScope: []scope.ScopeElement{{Target: "owned.example.com", Category: "url"}},
	}

	cmd := newOrchestrationTestCmd()
	_ = cmd.Flags().Set("db", "true")
	if err := runPollWithPollers(cmd, []platforms.PlatformPoller{mock}); err != nil {
		t.Fatalf("runPollWithPollers: %v (a custom-owned URL must not fail the platform)", err)
	}
}
