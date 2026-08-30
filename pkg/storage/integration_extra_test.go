package storage

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestIntegration_AIVariants(t *testing.T) {
	db := openTestDB(t)
	platform := uniquePlatform(t)
	cleanupPlatform(t, db, platform)
	ctx := context.Background()
	programURL := "https://example.com/" + platform + "/a"

	items := []TargetItem{
		{
			URI:      "https://*.messy.example.com/**",
			Category: "url",
			InScope:  true,
			Variants: []TargetVariant{
				{Value: "messy.example.com", HasCategory: true, Category: "wildcard"},
			},
		},
	}
	entries := mustBuildEntries(t, programURL, platform, "a", items)
	if _, err := db.UpsertProgramEntries(ctx, programURL, platform, "a", entries); err != nil {
		t.Fatalf("UpsertProgramEntries: %v", err)
	}

	enh, err := db.ListAIEnhancements(ctx, programURL)
	if err != nil {
		t.Fatalf("ListAIEnhancements: %v", err)
	}
	if len(enh) == 0 {
		t.Fatalf("expected at least one AI enhancement, got none")
	}
	found := false
	for _, variants := range enh {
		for _, v := range variants {
			if v.Value == "messy.example.com" && v.HasCategory && v.Category == "wildcard" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("expected a wildcard variant 'messy.example.com', got %#v", enh)
	}
}

func TestIntegration_IdenticalReupsertDoesNotPhantomUpdateVariants(t *testing.T) {
	db := openTestDB(t)
	platform := uniquePlatform(t)
	cleanupPlatform(t, db, platform)
	ctx := context.Background()
	programURL := "https://example.com/" + platform + "/phantom"

	items := []TargetItem{
		{
			URI: "api.example.com", Category: "url", InScope: true,
			Variants: []TargetVariant{
				{Value: "www.api.example.com"},
				{Value: "api.example.com", HasInScope: true, InScope: true, HasCategory: true, Category: "url"},
			},
		},
	}
	entries := mustBuildEntries(t, programURL, platform, "a", items)
	if _, err := db.UpsertProgramEntries(ctx, programURL, platform, "a", entries); err != nil {
		t.Fatalf("seed: %v", err)
	}

	changes, err := db.UpsertProgramEntries(ctx, programURL, platform, "a", entries)
	if err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	for _, c := range changes {
		if c.ChangeType == "updated" {
			t.Fatalf("identical re-upsert reported a phantom update: %+v", c)
		}
	}
}

func TestIntegration_AddCustomTarget(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	progURL := "custom-itest-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	target := "custom-" + strconv.FormatInt(time.Now().UnixNano(), 36) + ".example.com"

	t.Cleanup(func() {
		_, _ = db.sql.Exec(`DELETE FROM targets_raw WHERE program_id IN (SELECT id FROM programs WHERE url = $1)`, progURL)
		_, _ = db.sql.Exec(`DELETE FROM programs WHERE url = $1`, progURL)
	})

	added, err := db.AddCustomTarget(ctx, target, "wildcard", progURL)
	if err != nil {
		t.Fatalf("AddCustomTarget: %v", err)
	}
	if !added {
		t.Fatalf("expected first AddCustomTarget to report added=true")
	}

	// Adding the same target again should be a no-op (added=false).
	added2, err := db.AddCustomTarget(ctx, target, "wildcard", progURL)
	if err != nil {
		t.Fatalf("AddCustomTarget (repeat): %v", err)
	}
	if added2 {
		t.Fatalf("expected repeat AddCustomTarget to report added=false")
	}
}

func TestIntegration_GetStatsAndSearch(t *testing.T) {
	db := openTestDB(t)
	platform := uniquePlatform(t)
	cleanupPlatform(t, db, platform)
	ctx := context.Background()
	programURL := "https://example.com/" + platform + "/a"
	token := "srch" + strconv.FormatInt(time.Now().UnixNano(), 36)

	entries := mustBuildEntries(t, programURL, platform, "a", []TargetItem{
		{URI: token + ".example.com", Category: "url", InScope: true},
		{URI: "oos.example.com", Category: "url", InScope: false},
	})
	if _, err := db.UpsertProgramEntries(ctx, programURL, platform, "a", entries); err != nil {
		t.Fatalf("UpsertProgramEntries: %v", err)
	}

	stats, err := db.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	var mine *PlatformStats
	for i := range stats {
		if stats[i].Platform == platform {
			mine = &stats[i]
		}
	}
	if mine == nil {
		t.Fatalf("platform %q not present in stats", platform)
	}
	if mine.ProgramCount != 1 || mine.InScopeCount != 1 || mine.OutOfScopeCount != 1 {
		t.Fatalf("unexpected stats for %q: %+v", platform, *mine)
	}

	results, err := db.SearchTargets(ctx, token)
	if err != nil {
		t.Fatalf("SearchTargets: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected SearchTargets to find %q, got none", token)
	}
}

// TestIntegration_GetStatsAIVariantsCountOnce pins the AI-variant fix. The
// stats CTE used to LEFT JOIN targets_ai_enhanced without collapsing back to one
// row per raw target, so a target carrying N variants was counted N times.
func TestIntegration_GetStatsAIVariantsCountOnce(t *testing.T) {
	db := openTestDB(t)
	platform := uniquePlatform(t)
	cleanupPlatform(t, db, platform)
	ctx := context.Background()
	programURL := "https://example.com/" + platform + "/a"

	// One in-scope target with three AI variants and one out-of-scope target
	// with two. Correct stats are 1 and 1, not 3 and 2.
	entries := mustBuildEntries(t, programURL, platform, "a", []TargetItem{
		{
			URI:      "https://*.multi.example.com/**",
			Category: "url",
			InScope:  true,
			Variants: []TargetVariant{
				{Value: "multi.example.com", HasCategory: true, Category: "wildcard"},
				{Value: "a.multi.example.com", HasCategory: true, Category: "url"},
				{Value: "b.multi.example.com", HasCategory: true, Category: "url"},
			},
		},
		{
			URI:      "https://*.gone.example.com/**",
			Category: "url",
			InScope:  false,
			Variants: []TargetVariant{
				{Value: "gone.example.com", HasCategory: true, Category: "wildcard"},
				{Value: "x.gone.example.com", HasCategory: true, Category: "url"},
			},
		},
	})
	if _, err := db.UpsertProgramEntries(ctx, programURL, platform, "a", entries); err != nil {
		t.Fatalf("UpsertProgramEntries: %v", err)
	}

	mine := statsFor(t, db, ctx, platform)
	if mine.ProgramCount != 1 {
		t.Errorf("ProgramCount = %d, want 1", mine.ProgramCount)
	}
	if mine.InScopeCount != 1 {
		t.Errorf("InScopeCount = %d, want 1 (AI variants must not inflate the count)", mine.InScopeCount)
	}
	if mine.OutOfScopeCount != 1 {
		t.Errorf("OutOfScopeCount = %d, want 1 (AI variants must not inflate the count)", mine.OutOfScopeCount)
	}
}

// TestIntegration_GetStatsCountsProgramsWithoutTargets covers the inner-JOIN
// fix: a program that has no targets yet still belongs in the program count.
func TestIntegration_GetStatsCountsProgramsWithoutTargets(t *testing.T) {
	db := openTestDB(t)
	platform := uniquePlatform(t)
	cleanupPlatform(t, db, platform)
	ctx := context.Background()

	if _, err := db.sql.ExecContext(ctx, `
		INSERT INTO programs(url, platform, handle, first_seen_at, last_seen_at)
		VALUES($1, $2, $3, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, "https://example.com/"+platform+"/empty", platform, "empty"); err != nil {
		t.Fatalf("inserting target-less program: %v", err)
	}

	mine := statsFor(t, db, ctx, platform)
	if mine.ProgramCount != 1 {
		t.Errorf("ProgramCount = %d, want 1 (a program with no targets still counts)", mine.ProgramCount)
	}
	if mine.InScopeCount != 0 || mine.OutOfScopeCount != 0 {
		t.Errorf("target counts = %d/%d, want 0/0", mine.InScopeCount, mine.OutOfScopeCount)
	}
}

// TestIntegration_SearchTargetsEscapesWildcards confirms that LIKE
// metacharacters in a search term are matched literally rather than acting as
// wildcards that match every row.
func TestIntegration_SearchTargetsEscapesWildcards(t *testing.T) {
	db := openTestDB(t)
	platform := uniquePlatform(t)
	cleanupPlatform(t, db, platform)
	ctx := context.Background()
	programURL := "https://example.com/" + platform + "/a"

	entries := mustBuildEntries(t, programURL, platform, "a", []TargetItem{
		{URI: "alpha.example.com", Category: "url", InScope: true},
		{URI: "beta.example.com", Category: "url", InScope: true},
	})
	if _, err := db.UpsertProgramEntries(ctx, programURL, platform, "a", entries); err != nil {
		t.Fatalf("UpsertProgramEntries: %v", err)
	}

	// "%" previously matched every target in the database.
	results, err := db.SearchTargets(ctx, "%")
	if err != nil {
		t.Fatalf("SearchTargets: %v", err)
	}
	for _, r := range results {
		if r.Platform == platform {
			t.Fatalf("a literal %% must not match %q", r.TargetNormalized)
		}
	}
}

// TestIntegration_ListAIEnhancementsNonCanonicalURL covers the lookup fix:
// callers pass the raw poller URL, while programs are stored canonicalized.
func TestIntegration_ListAIEnhancementsNonCanonicalURL(t *testing.T) {
	db := openTestDB(t)
	platform := uniquePlatform(t)
	cleanupPlatform(t, db, platform)
	ctx := context.Background()
	programURL := "https://example.com/" + platform + "/a"

	entries := mustBuildEntries(t, programURL, platform, "a", []TargetItem{
		{
			URI:      "https://*.canon.example.com/**",
			Category: "url",
			InScope:  true,
			Variants: []TargetVariant{
				{Value: "canon.example.com", HasCategory: true, Category: "wildcard"},
			},
		},
	})
	if _, err := db.UpsertProgramEntries(ctx, programURL, platform, "a", entries); err != nil {
		t.Fatalf("UpsertProgramEntries: %v", err)
	}

	// Trailing slash and an uppercased host both canonicalize to the stored URL.
	for _, variant := range []string{
		programURL + "/",
		"https://EXAMPLE.com/" + platform + "/a",
	} {
		enh, err := db.ListAIEnhancements(ctx, variant)
		if err != nil {
			t.Fatalf("ListAIEnhancements(%q): %v", variant, err)
		}
		if len(enh) == 0 {
			t.Errorf("ListAIEnhancements(%q) found no enhancements; lookup is not canonicalizing", variant)
		}
	}
}

// TestIntegration_ListEntriesPlatformFilterIgnoresCase pins the fix for the
// asymmetry that turned CI red: splitPlatformList lowercases the user's filter,
// but the query compared it against the raw stored column, so any program whose
// platform contained uppercase characters was invisible to --platform.
func TestIntegration_ListEntriesPlatformFilterIgnoresCase(t *testing.T) {
	db := openTestDB(t)
	platform := uniquePlatform(t) + "_MixedCase"
	cleanupPlatform(t, db, platform)
	ctx := context.Background()
	programURL := "https://example.com/" + platform + "/a"

	entries := mustBuildEntries(t, programURL, platform, "a", []TargetItem{
		{URI: "case.example.com", Category: "url", InScope: true},
	})
	if _, err := db.UpsertProgramEntries(ctx, programURL, platform, "a", entries); err != nil {
		t.Fatalf("UpsertProgramEntries: %v", err)
	}

	for _, filter := range []string{platform, strings.ToLower(platform), strings.ToUpper(platform)} {
		listed, err := db.ListEntries(ctx, ListOptions{Platform: filter})
		if err != nil {
			t.Fatalf("ListEntries(%q): %v", filter, err)
		}
		if len(listed) != 1 {
			t.Errorf("ListEntries(platform=%q) returned %d entries, want 1", filter, len(listed))
		}
	}
}

// changeCountFor counts the audit rows recorded for one platform.
func changeCountFor(t *testing.T, db *DB, ctx context.Context, platform string) int {
	t.Helper()
	var n int
	if err := db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM scope_changes WHERE platform = $1`, platform).Scan(&n); err != nil {
		t.Fatalf("counting scope_changes: %v", err)
	}
	return n
}

// TestIntegration_UpsertLogsChangesInSameTransaction pins the atomicity fix:
// the audit rows must land as part of the upsert, not via a separate call the
// caller might never make (or that might fail after the scope already changed).
func TestIntegration_UpsertLogsChangesInSameTransaction(t *testing.T) {
	db := openTestDB(t)
	platform := uniquePlatform(t)
	cleanupPlatform(t, db, platform)
	ctx := context.Background()
	programURL := "https://example.com/" + platform + "/a"

	entries := mustBuildEntries(t, programURL, platform, "a", []TargetItem{
		{URI: "atomic-one.example.com", Category: "url", InScope: true},
		{URI: "atomic-two.example.com", Category: "url", InScope: true},
	})
	changes, err := db.UpsertProgramEntries(ctx, programURL, platform, "a", entries)
	if err != nil {
		t.Fatalf("UpsertProgramEntries: %v", err)
	}
	if len(changes) != 2 {
		t.Fatalf("expected 2 changes, got %d", len(changes))
	}

	if got := changeCountFor(t, db, ctx, platform); got != 2 {
		t.Errorf("scope_changes rows = %d, want 2 written by the upsert itself", got)
	}
}

// TestIntegration_UpsertSkipChangeLog covers the first-poll path, where logging
// every target as an addition would bury later real changes.
func TestIntegration_UpsertSkipChangeLog(t *testing.T) {
	db := openTestDB(t)
	platform := uniquePlatform(t)
	cleanupPlatform(t, db, platform)
	ctx := context.Background()
	programURL := "https://example.com/" + platform + "/a"

	entries := mustBuildEntries(t, programURL, platform, "a", []TargetItem{
		{URI: "first-run.example.com", Category: "url", InScope: true},
	})
	changes, err := db.UpsertProgramEntriesWithOptions(ctx, programURL, platform, "a", entries, UpsertOptions{SkipChangeLog: true})
	if err != nil {
		t.Fatalf("UpsertProgramEntriesWithOptions: %v", err)
	}
	// The caller still learns what changed; only the audit write is suppressed.
	if len(changes) != 1 {
		t.Errorf("expected the change to still be reported, got %d", len(changes))
	}
	if got := changeCountFor(t, db, ctx, platform); got != 0 {
		t.Errorf("scope_changes rows = %d, want 0 when SkipChangeLog is set", got)
	}
}

// TestIntegration_SyncLogsRemovalsInSameTransaction covers the sync half of the
// same guarantee, and confirms the disable and its audit row agree.
func TestIntegration_SyncLogsRemovalsInSameTransaction(t *testing.T) {
	db := openTestDB(t)
	platform := uniquePlatform(t)
	cleanupPlatform(t, db, platform)
	ctx := context.Background()

	// Four programs so that removing one stays under the partial-sync ratio.
	var urls []string
	for _, name := range []string{"a", "b", "c", "d"} {
		u := "https://example.com/" + platform + "/" + name
		urls = append(urls, u)
		entries := mustBuildEntries(t, u, platform, name, []TargetItem{
			{URI: name + ".example.com", Category: "url", InScope: true},
		})
		if _, err := db.UpsertProgramEntriesWithOptions(ctx, u, platform, name, entries, UpsertOptions{SkipChangeLog: true}); err != nil {
			t.Fatalf("seeding %s: %v", u, err)
		}
	}

	// Poll returns everything except the last program.
	removed, err := db.SyncPlatformPrograms(ctx, platform, urls[:3])
	if err != nil {
		t.Fatalf("SyncPlatformPrograms: %v", err)
	}
	if len(removed) != 1 {
		t.Fatalf("expected 1 removal, got %d", len(removed))
	}

	if got := changeCountFor(t, db, ctx, platform); got != 1 {
		t.Errorf("scope_changes rows = %d, want 1 written by the sync itself", got)
	}

	var disabled int
	if err := db.sql.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM programs WHERE platform = $1 AND disabled = 1`, platform).Scan(&disabled); err != nil {
		t.Fatalf("counting disabled programs: %v", err)
	}
	if disabled != 1 {
		t.Errorf("disabled programs = %d, want 1 (audit row and disable must agree)", disabled)
	}
}

// TestIntegration_SyncAbortWritesNothing confirms the partial-sync guard leaves
// no trace: no disables, and no audit rows describing disables that never
// happened.
func TestIntegration_SyncAbortWritesNothing(t *testing.T) {
	db := openTestDB(t)
	platform := uniquePlatform(t)
	cleanupPlatform(t, db, platform)
	ctx := context.Background()

	var urls []string
	for _, name := range []string{"a", "b", "c", "d"} {
		u := "https://example.com/" + platform + "/" + name
		urls = append(urls, u)
		entries := mustBuildEntries(t, u, platform, name, []TargetItem{
			{URI: name + ".example.com", Category: "url", InScope: true},
		})
		if _, err := db.UpsertProgramEntriesWithOptions(ctx, u, platform, name, entries, UpsertOptions{SkipChangeLog: true}); err != nil {
			t.Fatalf("seeding %s: %v", u, err)
		}
	}

	// A truncated poll returning only one of four programs trips the guard.
	if _, err := db.SyncPlatformPrograms(ctx, platform, urls[:1]); !errors.Is(err, ErrAbortingPartialSync) {
		t.Fatalf("expected ErrAbortingPartialSync, got %v", err)
	}

	var disabled int
	if err := db.sql.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM programs WHERE platform = $1 AND disabled = 1`, platform).Scan(&disabled); err != nil {
		t.Fatalf("counting disabled programs: %v", err)
	}
	if disabled != 0 {
		t.Errorf("disabled programs = %d, want 0 after an aborted sync", disabled)
	}
	if got := changeCountFor(t, db, ctx, platform); got != 0 {
		t.Errorf("scope_changes rows = %d, want 0 after an aborted sync", got)
	}
}

// statsFor returns the PlatformStats row for one platform, failing if absent.
func statsFor(t *testing.T, db *DB, ctx context.Context, platform string) PlatformStats {
	t.Helper()
	stats, err := db.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	for _, s := range stats {
		if s.Platform == platform {
			return s
		}
	}
	t.Fatalf("platform %q not present in stats", platform)
	return PlatformStats{}
}

func TestIntegration_ListRecentChanges(t *testing.T) {
	db := openTestDB(t)
	platform := uniquePlatform(t)
	cleanupPlatform(t, db, platform)
	ctx := context.Background()
	programURL := "https://example.com/" + platform + "/a"

	entries := mustBuildEntries(t, programURL, platform, "a", []TargetItem{
		{URI: "recent.example.com", Category: "url", InScope: true},
	})
	// No explicit LogChanges call: the upsert records its own audit rows inside
	// the transaction that performed the mutation.
	if _, err := db.UpsertProgramEntries(ctx, programURL, platform, "a", entries); err != nil {
		t.Fatalf("UpsertProgramEntries: %v", err)
	}

	recent, err := db.ListRecentChanges(ctx, 100)
	if err != nil {
		t.Fatalf("ListRecentChanges: %v", err)
	}
	found := false
	for _, c := range recent {
		if c.Platform == platform && c.TargetRaw == "recent.example.com" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected recent change for %q not found", platform)
	}
}

// TestUpsertAllBlankTargetsAbortsWipe covers a poller that starts returning
// blank targets — the shape of a platform markup change. The scope-wipe guard
// used to count raw entries, so a non-empty batch of unusable entries cleared it
// and then every existing target was collected for deletion.
func TestUpsertAllBlankTargetsAbortsWipe(t *testing.T) {
	db := openTestDB(t)
	platform := uniquePlatform(t)
	cleanupPlatform(t, db, platform)

	ctx := context.Background()
	programURL := "https://example.com/programs/" + platform
	handle := "acme"

	seed := mustBuildEntries(t, programURL, platform, handle, []TargetItem{
		{URI: "api.example.com", Category: "url", InScope: true},
		{URI: "*.example.com", Category: "wildcard", InScope: true},
	})
	if _, err := db.UpsertProgramEntries(ctx, programURL, platform, handle, seed); err != nil {
		t.Fatalf("seeding targets: %v", err)
	}

	// Every entry here normalizes to an empty identity key, so none of them can
	// be diffed against the seeded rows.
	blank := mustBuildEntries(t, programURL, platform, handle, []TargetItem{
		{URI: "   ", Category: "url", InScope: true},
		{URI: "", Category: "wildcard", InScope: true},
		{URI: "\t\n", Category: "url", InScope: true},
	})
	if len(blank) == 0 {
		t.Fatal("expected BuildEntries to return the blank entries so the guard is actually exercised")
	}

	_, err := db.UpsertProgramEntries(ctx, programURL, platform, handle, blank)
	if !errors.Is(err, ErrAbortingScopeWipe) {
		t.Fatalf("expected ErrAbortingScopeWipe, got %v", err)
	}

	var remaining int
	if err := db.sql.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM targets_raw tr
		JOIN programs p ON tr.program_id = p.id
		WHERE p.platform = $1
	`, platform).Scan(&remaining); err != nil {
		t.Fatalf("counting remaining targets: %v", err)
	}
	if remaining != 2 {
		t.Fatalf("expected both seeded targets retained, got %d", remaining)
	}
}

// TestUpsertMixedBlankTargetsRemovesOnlyAbsent confirms the corrected guard is
// not over-eager: a batch that carries at least one usable entry must still
// process normally, blank entries and all.
func TestUpsertMixedBlankTargetsRemovesOnlyAbsent(t *testing.T) {
	db := openTestDB(t)
	platform := uniquePlatform(t)
	cleanupPlatform(t, db, platform)

	ctx := context.Background()
	programURL := "https://example.com/programs/" + platform
	handle := "acme"

	seed := mustBuildEntries(t, programURL, platform, handle, []TargetItem{
		{URI: "api.example.com", Category: "url", InScope: true},
		{URI: "old.example.com", Category: "url", InScope: true},
	})
	if _, err := db.UpsertProgramEntries(ctx, programURL, platform, handle, seed); err != nil {
		t.Fatalf("seeding targets: %v", err)
	}

	next := mustBuildEntries(t, programURL, platform, handle, []TargetItem{
		{URI: "api.example.com", Category: "url", InScope: true},
		{URI: "  ", Category: "url", InScope: true},
	})
	changes, err := db.UpsertProgramEntries(ctx, programURL, platform, handle, next)
	if err != nil {
		t.Fatalf("UpsertProgramEntries: %v", err)
	}

	counts := countByType(changes)
	if counts["removed"] != 1 {
		t.Fatalf("expected exactly one removal (old.example.com), got %v", counts)
	}
	if counts["added"] != 0 {
		t.Fatalf("blank entries must not be added as targets, got %v", counts)
	}
}

func TestIntegration_SearchTargetsHidesIgnoredPrograms(t *testing.T) {
	db := openTestDB(t)
	platform := uniquePlatform(t)
	cleanupPlatform(t, db, platform)
	ctx := context.Background()
	programURL := "https://example.com/" + platform + "/a"
	target := "ignoreme-" + platform + ".example.com"

	entries := mustBuildEntries(t, programURL, platform, "a", []TargetItem{
		{URI: target, Category: "url", InScope: true},
	})
	if _, err := db.UpsertProgramEntries(ctx, programURL, platform, "a", entries); err != nil {
		t.Fatalf("UpsertProgramEntries: %v", err)
	}

	found, err := db.SearchTargets(ctx, target)
	if err != nil {
		t.Fatalf("SearchTargets: %v", err)
	}
	if len(found) == 0 {
		t.Fatal("expected the live target before ignore")
	}

	if err := db.SetProgramIgnoredStatus(ctx, programURL, true); err != nil {
		t.Fatalf("SetProgramIgnoredStatus: %v", err)
	}

	found, err = db.SearchTargets(ctx, target)
	if err != nil {
		t.Fatalf("SearchTargets after ignore: %v", err)
	}
	for _, r := range found {
		if r.Platform == platform {
			t.Fatalf("ignored program leaked via %s search: %+v", r.Source, r)
		}
	}
}

func TestIntegration_ListEntriesSinceUsesRawLastSeen(t *testing.T) {
	db := openTestDB(t)
	platform := uniquePlatform(t)
	cleanupPlatform(t, db, platform)
	ctx := context.Background()
	programURL := "https://example.com/" + platform + "/a"

	entries := mustBuildEntries(t, programURL, platform, "a", []TargetItem{
		{
			URI:      "since.example.com",
			Category: "url",
			InScope:  true,
			Variants: []TargetVariant{{Value: "since.example.com"}},
		},
	})
	if _, err := db.UpsertProgramEntries(ctx, programURL, platform, "a", entries); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	if _, err := db.sql.Exec(`
		UPDATE targets_ai_enhanced
		SET last_seen_at = NOW() - INTERVAL '2 days'
		WHERE target_id IN (
			SELECT t.id FROM targets_raw t
			JOIN programs p ON p.id = t.program_id
			WHERE p.platform = $1
		)
	`, platform); err != nil {
		t.Fatalf("staling AI last_seen_at: %v", err)
	}

	if _, err := db.UpsertProgramEntries(ctx, programURL, platform, "a", entries); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	got, err := db.ListEntries(ctx, ListOptions{Platform: platform, Since: time.Now().Add(-time.Minute)})
	if err != nil {
		t.Fatalf("ListEntries: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("AI-joined row was dropped because --since used the stale AI timestamp")
	}
}

func TestIntegration_UpsertMergeOnlyKeepsMissingTargets(t *testing.T) {
	db := openTestDB(t)
	platform := uniquePlatform(t)
	cleanupPlatform(t, db, platform)
	ctx := context.Background()
	programURL := "https://example.com/" + platform + "/merge"

	seed := mustBuildEntries(t, programURL, platform, "a", []TargetItem{
		{URI: "keep.example.com", Category: "url", InScope: true},
		{URI: "extra.example.com", Category: "url", InScope: true},
	})
	if _, err := db.UpsertProgramEntries(ctx, programURL, platform, "a", seed); err != nil {
		t.Fatalf("seed: %v", err)
	}

	partial := mustBuildEntries(t, programURL, platform, "a", []TargetItem{
		{URI: "keep.example.com", Category: "url", InScope: true},
	})
	changes, err := db.UpsertProgramEntriesWithOptions(ctx, programURL, platform, "a", partial, UpsertOptions{MergeOnly: true})
	if err != nil {
		t.Fatalf("merge-only upsert: %v", err)
	}
	for _, c := range changes {
		if c.ChangeType == "removed" {
			t.Fatalf("merge-only upsert removed %s", c.TargetRaw)
		}
	}

	got, err := db.ListEntries(ctx, ListOptions{Platform: platform, IncludeOOS: true})
	if err != nil {
		t.Fatalf("ListEntries: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("merge-only upsert dropped a live target: got %d entries", len(got))
	}
}

func TestIntegration_GetOrCreateProgramDoesNotStealURL(t *testing.T) {
	db := openTestDB(t)
	owner := uniquePlatform(t)
	thief := uniquePlatform(t)
	cleanupPlatform(t, db, owner)
	cleanupPlatform(t, db, thief)
	ctx := context.Background()
	programURL := "https://example.com/" + owner + "/shared"

	entries := mustBuildEntries(t, programURL, owner, "a", []TargetItem{
		{URI: "api.example.com", Category: "url", InScope: true},
	})
	if _, err := db.UpsertProgramEntries(ctx, programURL, owner, "a", entries); err != nil {
		t.Fatalf("owner upsert: %v", err)
	}

	stolen := mustBuildEntries(t, programURL, thief, "b", []TargetItem{
		{URI: "evil.example.com", Category: "url", InScope: true},
	})
	_, err := db.UpsertProgramEntries(ctx, programURL, thief, "b", stolen)
	if err == nil {
		t.Fatal("expected upsert to refuse a URL owned by another platform")
	}
	if !errors.Is(err, ErrProgramURLOwned) {
		t.Fatalf("error = %v, want ErrProgramURLOwned", err)
	}

	progs, err := db.ListPrograms(ctx)
	if err != nil {
		t.Fatalf("ListPrograms: %v", err)
	}
	for _, p := range progs {
		if p.URL == NormalizeProgramURL(programURL) && p.Platform != owner {
			t.Fatalf("program URL was stolen: %+v", p)
		}
	}
}
