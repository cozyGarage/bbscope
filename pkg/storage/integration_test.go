package storage

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// openTestDB connects to the database identified by TEST_DB_URL. Tests that
// require a live PostgreSQL are skipped cleanly when the variable is not set,
// so `go test ./...` stays green in environments without a database.
func openTestDB(t *testing.T) *DB {
	t.Helper()
	dsn := os.Getenv("TEST_DB_URL")
	if dsn == "" {
		t.Skip("TEST_DB_URL not set; skipping PostgreSQL integration test")
	}
	db, err := Open(dsn)
	if err != nil {
		t.Fatalf("Open(TEST_DB_URL): %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// uniquePlatform returns a platform identifier unique to this test invocation so
// repeated or parallel runs never observe each other's rows.
func uniquePlatform(t *testing.T) string {
	t.Helper()
	name := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	return "itest_" + name + "_" + strconv.FormatInt(time.Now().UnixNano(), 36)
}

// cleanupPlatform removes every row created under the given platform once the
// test finishes. It runs in the same package, so it can reach the unexported
// *sql.DB handle directly.
func cleanupPlatform(t *testing.T, db *DB, platform string) {
	t.Helper()
	t.Cleanup(func() {
		_, _ = db.sql.Exec(`DELETE FROM targets_ai_enhanced WHERE target_id IN (
			SELECT tr.id FROM targets_raw tr JOIN programs p ON tr.program_id = p.id WHERE p.platform = $1)`, platform)
		_, _ = db.sql.Exec(`DELETE FROM targets_raw WHERE program_id IN (SELECT id FROM programs WHERE platform = $1)`, platform)
		_, _ = db.sql.Exec(`DELETE FROM scope_changes WHERE platform = $1`, platform)
		_, _ = db.sql.Exec(`DELETE FROM programs WHERE platform = $1`, platform)
	})
}

func mustBuildEntries(t *testing.T, programURL, platform, handle string, items []TargetItem) []UpsertEntry {
	t.Helper()
	entries, err := BuildEntries(programURL, platform, handle, items)
	if err != nil {
		t.Fatalf("BuildEntries: %v", err)
	}
	return entries
}

func countByType(changes []Change) map[string]int {
	m := map[string]int{}
	for _, c := range changes {
		m[c.ChangeType]++
	}
	return m
}

func TestIntegration_UpsertAndList(t *testing.T) {
	db := openTestDB(t)
	platform := uniquePlatform(t)
	cleanupPlatform(t, db, platform)
	ctx := context.Background()
	programURL := "https://example.com/" + platform + "/a"

	items := []TargetItem{
		{URI: "*.example.com", Category: "wildcard", InScope: true},
		{URI: "https://api.example.com", Category: "url", InScope: true},
		{URI: "old.example.com", Category: "url", InScope: false},
	}
	entries := mustBuildEntries(t, programURL, platform, "a", items)

	changes, err := db.UpsertProgramEntries(ctx, programURL, platform, "a", entries)
	if err != nil {
		t.Fatalf("UpsertProgramEntries (initial): %v", err)
	}
	if got := countByType(changes)["added"]; got != len(items) {
		t.Fatalf("expected %d added changes on first upsert, got %d (%v)", len(items), got, countByType(changes))
	}

	listed, err := db.ListEntries(ctx, ListOptions{Platform: platform, IncludeOOS: true})
	if err != nil {
		t.Fatalf("ListEntries: %v", err)
	}
	if len(listed) != len(items) {
		t.Fatalf("expected %d entries, got %d", len(items), len(listed))
	}

	// Without IncludeOOS the out-of-scope target must be filtered out.
	inScopeOnly, err := db.ListEntries(ctx, ListOptions{Platform: platform})
	if err != nil {
		t.Fatalf("ListEntries (in-scope only): %v", err)
	}
	if len(inScopeOnly) != 2 {
		t.Fatalf("expected 2 in-scope entries, got %d", len(inScopeOnly))
	}
}

func TestIntegration_ChangeDetection(t *testing.T) {
	db := openTestDB(t)
	platform := uniquePlatform(t)
	cleanupPlatform(t, db, platform)
	ctx := context.Background()
	programURL := "https://example.com/" + platform + "/a"

	first := mustBuildEntries(t, programURL, platform, "a", []TargetItem{
		{URI: "keep.example.com", Category: "url", InScope: true},
		{URI: "remove.example.com", Category: "url", InScope: true},
		{URI: "change.example.com", Category: "url", InScope: true, Description: "before"},
	})
	if _, err := db.UpsertProgramEntries(ctx, programURL, platform, "a", first); err != nil {
		t.Fatalf("UpsertProgramEntries (initial): %v", err)
	}

	second := mustBuildEntries(t, programURL, platform, "a", []TargetItem{
		{URI: "keep.example.com", Category: "url", InScope: true},
		{URI: "change.example.com", Category: "url", InScope: true, Description: "after"},
		{URI: "new.example.com", Category: "url", InScope: true},
	})
	changes, err := db.UpsertProgramEntries(ctx, programURL, platform, "a", second)
	if err != nil {
		t.Fatalf("UpsertProgramEntries (update): %v", err)
	}

	byType := countByType(changes)
	if byType["added"] != 1 || byType["removed"] != 1 || byType["updated"] != 1 {
		t.Fatalf("expected 1 added / 1 removed / 1 updated, got %v", byType)
	}

	listed, err := db.ListEntries(ctx, ListOptions{Platform: platform, IncludeOOS: true})
	if err != nil {
		t.Fatalf("ListEntries: %v", err)
	}
	got := map[string]bool{}
	for _, e := range listed {
		got[e.TargetRaw] = true
	}
	if got["remove.example.com"] {
		t.Fatalf("removed target should be gone, got %v", got)
	}
	if !got["new.example.com"] || !got["keep.example.com"] || !got["change.example.com"] {
		t.Fatalf("expected keep/change/new targets present, got %v", got)
	}
}

func TestIntegration_WipeGuard(t *testing.T) {
	db := openTestDB(t)
	platform := uniquePlatform(t)
	cleanupPlatform(t, db, platform)
	ctx := context.Background()
	programURL := "https://example.com/" + platform + "/a"

	entries := mustBuildEntries(t, programURL, platform, "a", []TargetItem{
		{URI: "a.example.com", Category: "url", InScope: true},
		{URI: "b.example.com", Category: "url", InScope: true},
	})
	if _, err := db.UpsertProgramEntries(ctx, programURL, platform, "a", entries); err != nil {
		t.Fatalf("UpsertProgramEntries (initial): %v", err)
	}

	// An empty scope for an existing program must be refused, not wipe the data.
	_, err := db.UpsertProgramEntries(ctx, programURL, platform, "a", nil)
	if err == nil {
		t.Fatalf("expected ErrAbortingScopeWipe, got nil")
	}
	if err.Error() != ErrAbortingScopeWipe.Error() {
		t.Fatalf("expected ErrAbortingScopeWipe, got %v", err)
	}

	listed, err := db.ListEntries(ctx, ListOptions{Platform: platform, IncludeOOS: true})
	if err != nil {
		t.Fatalf("ListEntries: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("wipe guard should have preserved 2 entries, got %d", len(listed))
	}
}

func TestIntegration_SyncPlatformPrograms(t *testing.T) {
	db := openTestDB(t)
	platform := uniquePlatform(t)
	cleanupPlatform(t, db, platform)
	ctx := context.Background()
	progA := "https://example.com/" + platform + "/a"
	progB := "https://example.com/" + platform + "/b"

	for _, p := range []string{progA, progB} {
		entries := mustBuildEntries(t, p, platform, "h", []TargetItem{
			{URI: "t.example.com", Category: "url", InScope: true},
		})
		if _, err := db.UpsertProgramEntries(ctx, p, platform, "h", entries); err != nil {
			t.Fatalf("UpsertProgramEntries(%s): %v", p, err)
		}
	}

	if count, err := db.GetActiveProgramCount(ctx, platform); err != nil || count != 2 {
		t.Fatalf("expected 2 active programs, got %d (err=%v)", count, err)
	}

	// Only program A was seen in the latest poll; B should be disabled. Use a
	// trailing slash to verify sync comparisons use normalized program URLs.
	if _, err := db.SyncPlatformPrograms(ctx, platform, []string{progA + "/"}); err != nil {
		t.Fatalf("SyncPlatformPrograms: %v", err)
	}

	if count, err := db.GetActiveProgramCount(ctx, platform); err != nil || count != 1 {
		t.Fatalf("expected 1 active program after sync, got %d (err=%v)", count, err)
	}

	listed, err := db.ListEntries(ctx, ListOptions{Platform: platform})
	if err != nil {
		t.Fatalf("ListEntries after sync: %v", err)
	}
	if len(listed) != 1 || listed[0].ProgramURL != NormalizeProgramURL(progA) {
		t.Fatalf("expected only active program A to list after sync, got %#v", listed)
	}

	var retainedTargets int
	if err := db.sql.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM targets_raw tr
		JOIN programs p ON p.id = tr.program_id
		WHERE p.url = $1
	`, NormalizeProgramURL(progB)).Scan(&retainedTargets); err != nil {
		t.Fatalf("counting retained targets: %v", err)
	}
	if retainedTargets != 1 {
		t.Fatalf("soft-disabled program should retain 1 target, got %d", retainedTargets)
	}

	// Soft-disable retains targets, but default list excludes disabled programs.
	activeOnly, err := db.ListEntries(ctx, ListOptions{Platform: platform, IncludeOOS: true, IncludeIgnored: true})
	if err != nil {
		t.Fatalf("ListEntries: %v", err)
	}
	for _, e := range activeOnly {
		if e.ProgramURL == progB || e.ProgramURL == NormalizeProgramURL(progB) {
			t.Fatalf("default ListEntries should exclude soft-disabled program %s", progB)
		}
	}

	listedDisabled, err := db.ListEntries(ctx, ListOptions{Platform: platform, IncludeOOS: true, IncludeIgnored: true, IncludeDisabled: true})
	if err != nil {
		t.Fatalf("ListEntries(IncludeDisabled): %v", err)
	}
	foundB := false
	for _, e := range listedDisabled {
		if e.ProgramURL == progB || e.ProgramURL == NormalizeProgramURL(progB) {
			foundB = true
			break
		}
	}
	if !foundB {
		t.Fatalf("expected retained targets for soft-disabled program %s with IncludeDisabled, got %#v", progB, listedDisabled)
	}
}

func TestIntegration_GetChangesBetween(t *testing.T) {
	db := openTestDB(t)
	platform := uniquePlatform(t)
	cleanupPlatform(t, db, platform)
	ctx := context.Background()
	programURL := "https://example.com/" + platform + "/a"

	entries := mustBuildEntries(t, programURL, platform, "a", []TargetItem{
		{URI: "logged.example.com", Category: "url", InScope: true},
	})
	changes, err := db.UpsertProgramEntries(ctx, programURL, platform, "a", entries)
	if err != nil {
		t.Fatalf("UpsertProgramEntries: %v", err)
	}
	if err := db.LogChanges(ctx, changes); err != nil {
		t.Fatalf("LogChanges: %v", err)
	}

	from := time.Now().Add(-1 * time.Hour)
	to := time.Now().Add(1 * time.Hour)
	got, err := db.GetChangesBetween(ctx, from, to, programURL)
	if err != nil {
		t.Fatalf("GetChangesBetween: %v", err)
	}
	if len(got) == 0 {
		t.Fatalf("expected at least one logged change, got 0")
	}
	for _, c := range got {
		if c.Platform != platform {
			t.Fatalf("unexpected platform in change: %q", c.Platform)
		}
	}
}

// TestIntegrationDatabaseIsConfiguredInCI fails when the integration suite would
// silently skip in CI. The storage integration tests are gated on TEST_DB_URL,
// and for a long time no CI job set it: they skipped on every push while the
// suite still reported green, hiding the fact that the package owning every
// destructive database operation was effectively untested.
//
// Locally, where TEST_DB_URL is genuinely optional, this test skips like the rest.
func TestIntegrationDatabaseIsConfiguredInCI(t *testing.T) {
	if os.Getenv("CI") == "" {
		t.Skip("not running in CI; TEST_DB_URL is optional for local runs")
	}
	if os.Getenv("TEST_DB_URL") == "" {
		t.Fatal("TEST_DB_URL is unset in CI: the storage integration tests would skip. " +
			"Check the postgres service and TEST_DB_URL in .github/workflows/ci.yml.")
	}
}
