package storage

import (
	"context"
	"errors"
	"strconv"
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

func TestIntegration_ListRecentChanges(t *testing.T) {
	db := openTestDB(t)
	platform := uniquePlatform(t)
	cleanupPlatform(t, db, platform)
	ctx := context.Background()
	programURL := "https://example.com/" + platform + "/a"

	entries := mustBuildEntries(t, programURL, platform, "a", []TargetItem{
		{URI: "recent.example.com", Category: "url", InScope: true},
	})
	changes, err := db.UpsertProgramEntries(ctx, programURL, platform, "a", entries)
	if err != nil {
		t.Fatalf("UpsertProgramEntries: %v", err)
	}
	if err := db.LogChanges(ctx, changes); err != nil {
		t.Fatalf("LogChanges: %v", err)
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
