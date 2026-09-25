package storage

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// withSearchPath returns a DSN whose connections use schemaName as the search path.
func withSearchPath(dsn, schemaName string) string {
	if strings.Contains(dsn, "://") {
		u, err := url.Parse(dsn)
		if err != nil {
			return dsn
		}
		q := u.Query()
		q.Set("search_path", schemaName)
		u.RawQuery = q.Encode()
		return u.String()
	}
	return dsn + " search_path=" + schemaName
}

func TestIntegration_UpsertCollapsesCanonicalSpellings(t *testing.T) {
	db := openTestDB(t)
	platform := uniquePlatform(t)
	cleanupPlatform(t, db, platform)
	ctx := context.Background()
	programURL := "https://example.com/" + platform + "/spellings"

	first := mustBuildEntries(t, programURL, platform, "spell", []TargetItem{{
		URI:         "https://Example.com/a",
		Category:    "url",
		InScope:     true,
		Description: "first",
	}})
	if _, err := db.UpsertProgramEntries(ctx, programURL, platform, "spell", first); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	// Host case, the default HTTPS port, a trailing slash, and a category alias
	// ("website" -> "url") are the same identity as the row already stored.
	second := mustBuildEntries(t, programURL, platform, "spell", []TargetItem{{
		URI:         "https://example.com:443/a/",
		Category:    "website",
		InScope:     true,
		Description: "second",
	}})
	changes, err := db.UpsertProgramEntries(ctx, programURL, platform, "spell", second)
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if got := countByType(changes)["added"]; got != 0 {
		t.Fatalf("second spelling reported %d additions, want 0", got)
	}
	if got := countByType(changes)["updated"]; got != 1 {
		t.Fatalf("second spelling reported %d updates, want 1 (%v)", got, countByType(changes))
	}

	var n int
	var target, identity, category, description string
	if err := db.sql.QueryRowContext(ctx, `
		SELECT COUNT(*) OVER (), tr.target, tr.target_identity, tr.category, COALESCE(tr.description, '')
		FROM targets_raw tr
		JOIN programs p ON p.id = tr.program_id
		WHERE p.url = $1
	`, NormalizeProgramURL(programURL)).Scan(&n, &target, &identity, &category, &description); err != nil {
		t.Fatalf("read stored target: %v", err)
	}
	if n != 1 {
		t.Fatalf("stored rows = %d, want 1", n)
	}
	if target != "https://Example.com/a" {
		t.Fatalf("target = %q, want the first raw spelling", target)
	}
	if identity != "https://example.com/a" {
		t.Fatalf("target_identity = %q, want https://example.com/a", identity)
	}
	if category != "url" {
		t.Fatalf("category = %q, want url", category)
	}
	if description != "second" {
		t.Fatalf("description = %q, want second", description)
	}
}

func TestIntegration_AddCustomTargetCollapsesSpellings(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	progURL := "custom-ident-" + strconv.FormatInt(time.Now().UnixNano(), 36)

	t.Cleanup(func() {
		_, _ = db.sql.Exec(`DELETE FROM targets_raw WHERE program_id IN (SELECT id FROM programs WHERE url = $1)`, progURL)
		_, _ = db.sql.Exec(`DELETE FROM programs WHERE url = $1`, progURL)
	})

	added, err := db.AddCustomTarget(ctx, "https://Example.com/a", "URL", progURL)
	if err != nil {
		t.Fatalf("AddCustomTarget: %v", err)
	}
	if !added {
		t.Fatal("expected first spelling to be added")
	}
	added, err = db.AddCustomTarget(ctx, "https://example.com:443/a/", "url", progURL)
	if err != nil {
		t.Fatalf("AddCustomTarget second spelling: %v", err)
	}
	if added {
		t.Fatal("expected second spelling to match the existing target")
	}

	var n int
	var target, identity string
	if err := db.sql.QueryRowContext(ctx, `
		SELECT COUNT(*) OVER (), tr.target, tr.target_identity
		FROM targets_raw tr
		JOIN programs p ON p.id = tr.program_id
		WHERE p.url = $1
	`, progURL).Scan(&n, &target, &identity); err != nil {
		t.Fatalf("read custom target: %v", err)
	}
	if n != 1 || target != "https://Example.com/a" || identity != "https://example.com/a" {
		t.Fatalf("stored custom target n=%d target=%q identity=%q", n, target, identity)
	}
}

func TestIntegration_MigrateDuplicateTargetSpellings(t *testing.T) {
	dsn := os.Getenv("TEST_DB_URL")
	if dsn == "" {
		t.Skip("TEST_DB_URL not set; skipping PostgreSQL integration test")
	}
	schemaName := "it_ident_" + strconv.FormatInt(time.Now().UnixNano(), 36)
	quoted := pgx.Identifier{schemaName}.Sanitize()

	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open admin db: %v", err)
	}
	scoped, err := sql.Open("pgx", withSearchPath(dsn, schemaName))
	if err != nil {
		_ = admin.Close()
		t.Fatalf("open scoped db: %v", err)
	}
	t.Cleanup(func() {
		_ = scoped.Close()
		_, _ = admin.Exec(`DROP SCHEMA IF EXISTS ` + quoted + ` CASCADE`)
		_ = admin.Close()
	})

	if _, err := admin.Exec(`CREATE SCHEMA ` + quoted); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	var currentSchema string
	if err := scoped.QueryRow(`SELECT current_schema()`).Scan(&currentSchema); err != nil || currentSchema != schemaName {
		t.Fatalf("search_path schema = %q, want %s (%v)", currentSchema, schemaName, err)
	}
	if _, err := scoped.Exec(schema); err != nil {
		t.Fatalf("apply v1 schema: %v", err)
	}
	if _, err := scoped.Exec(`CREATE TABLE schema_migrations (
		version    INTEGER PRIMARY KEY,
		name       TEXT NOT NULL,
		applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		t.Fatalf("create schema_migrations: %v", err)
	}
	if _, err := scoped.Exec(`
		INSERT INTO schema_migrations(version, name) VALUES
			(1, 'initial_schema'),
			(2, 'canonicalize_program_urls_and_targets')
	`); err != nil {
		t.Fatalf("record v1 and v2: %v", err)
	}

	var programID int64
	if err := scoped.QueryRow(`
		INSERT INTO programs(platform, handle, url) VALUES ('custom', 'spell', 'https://example.com/mig')
		RETURNING id
	`).Scan(&programID); err != nil {
		t.Fatalf("insert program: %v", err)
	}
	var keeperID, loserID int64
	if err := scoped.QueryRow(`
		INSERT INTO targets_raw(program_id, target, category, in_scope, is_bbp)
		VALUES ($1, 'https://Example.com/a', 'URL', 1, 0)
		RETURNING id
	`, programID).Scan(&keeperID); err != nil {
		t.Fatalf("insert keeper: %v", err)
	}
	if err := scoped.QueryRow(`
		INSERT INTO targets_raw(program_id, target, category, in_scope, is_bbp)
		VALUES ($1, 'https://example.com:443/a/', 'url', 1, 0)
		RETURNING id
	`, programID).Scan(&loserID); err != nil {
		t.Fatalf("insert duplicate spelling: %v", err)
	}
	if _, err := scoped.Exec(`
		INSERT INTO targets_ai_enhanced(target_id, target_ai_normalized, category, in_scope)
		VALUES ($1, 'a.example.com', 'url', 1)
	`, loserID); err != nil {
		t.Fatalf("insert variant on duplicate: %v", err)
	}
	// A lone non-canonical spelling must survive v3 unchanged. v2 is already
	// recorded, so only v3 runs, and v3 must not rewrite target.
	if _, err := scoped.Exec(`
		INSERT INTO targets_raw(program_id, target, category, in_scope, is_bbp)
		VALUES ($1, 'https://Only.Example/b', 'url', 1, 0)
	`, programID); err != nil {
		t.Fatalf("insert singleton spelling: %v", err)
	}

	if err := applyMigrations(scoped); err != nil {
		t.Fatalf("apply v3: %v", err)
	}

	var n int
	if err := scoped.QueryRow(`SELECT COUNT(*) FROM targets_raw WHERE program_id = $1`, programID).Scan(&n); err != nil {
		t.Fatalf("count migrated targets: %v", err)
	}
	if n != 2 {
		t.Fatalf("migrated rows = %d, want 2", n)
	}
	var target, identity, category string
	var variantTarget int64
	if err := scoped.QueryRow(`
		SELECT tr.target, tr.target_identity, tr.category, v.target_id
		FROM targets_raw tr
		LEFT JOIN targets_ai_enhanced v ON v.target_id = tr.id
		WHERE tr.id = $1
	`, keeperID).Scan(&target, &identity, &category, &variantTarget); err != nil {
		t.Fatalf("read migrated target: %v", err)
	}
	if target != "https://Example.com/a" {
		t.Fatalf("target = %q, want the original keeper spelling", target)
	}
	if identity != "https://example.com/a" {
		t.Fatalf("target_identity = %q, want https://example.com/a", identity)
	}
	if category != "url" {
		t.Fatalf("category = %q, want url", category)
	}
	if variantTarget != keeperID {
		t.Fatalf("variant target_id = %d, want keeper %d", variantTarget, keeperID)
	}
	var singleTarget, singleIdentity string
	if err := scoped.QueryRow(`
		SELECT target, target_identity FROM targets_raw
		WHERE program_id = $1 AND target = 'https://Only.Example/b'
	`, programID).Scan(&singleTarget, &singleIdentity); err != nil {
		t.Fatalf("read singleton spelling: %v", err)
	}
	if singleTarget != "https://Only.Example/b" || singleIdentity != "https://only.example/b" {
		t.Fatalf("singleton target=%q identity=%q", singleTarget, singleIdentity)
	}

	_, err = scoped.Exec(`
		INSERT INTO targets_raw(program_id, target, category, target_identity, in_scope, is_bbp)
		VALUES ($1, 'https://EXAMPLE.com/a', 'url', 'https://example.com/a', 1, 0)
	`, programID)
	if err == nil {
		t.Fatal("unique constraint accepted a second spelling")
	}
	if !strings.Contains(err.Error(), "targets_raw_program_category_identity_key") {
		t.Fatalf("duplicate insert error = %v, want identity unique violation", err)
	}
}
