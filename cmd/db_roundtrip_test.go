package cmd

import (
	"context"
	"database/sql"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/cozyGarage/bbscope/v2/pkg/storage"
)

// openRoundtripDB connects to TEST_DB_URL, skipping when it is unset so
// `go test ./...` stays green without a database. It returns both the storage
// handle and a raw *sql.DB, since cleanup needs statements the storage API does
// not expose from outside its package.
func openRoundtripDB(t *testing.T) (*storage.DB, *sql.DB) {
	t.Helper()
	dsn := os.Getenv("TEST_DB_URL")
	if dsn == "" {
		t.Skip("TEST_DB_URL not set; skipping PostgreSQL round-trip test")
	}
	db, err := storage.Open(dsn)
	if err != nil {
		t.Fatalf("Open(TEST_DB_URL): %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	raw, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("sql.Open(TEST_DB_URL): %v", err)
	}
	t.Cleanup(func() { _ = raw.Close() })

	return db, raw
}

// TestIntegration_ExportImportRoundTrip is the check the previous implementation
// could not pass: `db export` wrote every platform's entries but `db import`
// skipped anything whose platform was not "custom", so a backup silently
// restored almost nothing.
func TestIntegration_ExportImportRoundTrip(t *testing.T) {
	db, raw := openRoundtripDB(t)
	ctx := context.Background()
	platform := "itest_roundtrip_" + strconv.FormatInt(time.Now().UnixNano(), 36)
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

	// Seed a program with in-scope, out-of-scope, and bounty variations, plus a
	// description containing a comma to exercise CSV quoting.
	items := []storage.TargetItem{
		{URI: "api.example.com", Category: "url", InScope: true, IsBBP: true, Description: "main API, v2"},
		{URI: "*.example.com", Category: "wildcard", InScope: true, IsBBP: false},
		{URI: "legacy.example.com", Category: "url", InScope: false, IsBBP: false},
	}
	built, err := storage.BuildEntries(programURL, platform, "a", items)
	if err != nil {
		t.Fatalf("BuildEntries: %v", err)
	}
	if _, err := db.UpsertProgramEntriesWithOptions(ctx, programURL, platform, "a", built,
		storage.UpsertOptions{SkipChangeLog: true}); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	exportOpts := storage.ListOptions{Platform: platform, IncludeOOS: true, IncludeDisabled: true}
	original, err := db.ListEntries(ctx, exportOpts)
	if err != nil {
		t.Fatalf("ListEntries: %v", err)
	}
	if len(original) != 3 {
		t.Fatalf("expected 3 seeded entries, got %d", len(original))
	}

	for _, format := range []string{"json", "csv"} {
		t.Run(format, func(t *testing.T) {
			// Export through the real command output path.
			var dump string
			switch format {
			case "json":
				dump = captureStdout(t, func() {
					if err := exportJSON(original); err != nil {
						t.Errorf("exportJSON: %v", err)
					}
				})
			case "csv":
				dump = captureStdout(t, func() {
					if err := exportCSV(original); err != nil {
						t.Errorf("exportCSV: %v", err)
					}
				})
			}

			// Wipe the platform, then restore from the dump.
			cleanup()
			if remaining, err := db.ListEntries(ctx, exportOpts); err != nil {
				t.Fatalf("ListEntries after wipe: %v", err)
			} else if len(remaining) != 0 {
				t.Fatalf("expected the platform to be empty after wipe, got %d", len(remaining))
			}

			var parsed []storage.Entry
			switch format {
			case "json":
				parsed, err = parseimportJSON(strings.NewReader(dump))
			case "csv":
				parsed, err = parseimportCSV(strings.NewReader(dump))
			}
			if err != nil {
				t.Fatalf("parsing %s dump: %v", format, err)
			}

			imported, failed := importEntries(ctx, db, parsed)
			if failed != 0 {
				t.Fatalf("%d entries failed to import", failed)
			}
			if imported != 3 {
				t.Fatalf("imported %d entries, want 3", imported)
			}

			// Compare what came back with what went in.
			restored, err := db.ListEntries(ctx, exportOpts)
			if err != nil {
				t.Fatalf("ListEntries after import: %v", err)
			}
			assertSameEntries(t, original, restored)
		})
	}
}

// assertSameEntries compares two entry sets by the fields a backup must
// preserve, keyed on the raw target so ordering does not matter.
func assertSameEntries(t *testing.T, want, got []storage.Entry) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("restored %d entries, want %d", len(got), len(want))
	}

	index := make(map[string]storage.Entry, len(got))
	for _, e := range got {
		index[e.TargetRaw] = e
	}

	for _, w := range want {
		g, ok := index[w.TargetRaw]
		if !ok {
			t.Errorf("target %q was not restored", w.TargetRaw)
			continue
		}
		if g.ProgramURL != w.ProgramURL || g.Platform != w.Platform || g.Handle != w.Handle {
			t.Errorf("%s: program identity changed: got %+v, want %+v", w.TargetRaw, g, w)
		}
		if g.Category != w.Category {
			t.Errorf("%s: category = %q, want %q", w.TargetRaw, g.Category, w.Category)
		}
		if g.InScope != w.InScope {
			t.Errorf("%s: in_scope = %v, want %v", w.TargetRaw, g.InScope, w.InScope)
		}
		if g.IsBBP != w.IsBBP {
			t.Errorf("%s: is_bbp = %v, want %v", w.TargetRaw, g.IsBBP, w.IsBBP)
		}
		if g.Description != w.Description {
			t.Errorf("%s: description = %q, want %q", w.TargetRaw, g.Description, w.Description)
		}
	}
}
