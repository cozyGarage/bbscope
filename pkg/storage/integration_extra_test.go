package storage

import (
	"context"
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
